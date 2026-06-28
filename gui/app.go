// Package gui is a cross-platform (desktop + mobile) Fyne front-end for the
// reusable deye-monitor/deye data layer. It renders a Deye-Cloud-style power-flow
// diagram, a detail grid and live history charts from a Deye SUN-12K-SG05LP3
// inverter polled over the Solarman V5 protocol on the local network.
package gui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/n0madic/deye-monitor/deye"
)

// appID is the cross-platform application identifier; it scopes Preferences.
const appID = "com.deye.monitor"

// Main is the GUI entry point, invoked from cmd/gui. It builds the window
// and runs the Fyne event loop until the window closes.
func Main() {
	a := app.NewWithID(appID)
	w := a.NewWindow("Deye Monitor")
	newAppUI(a, w).run()
}

// appUI wires the views, the toolbar and the polling controller together. All
// fields except the controller are touched only on the UI thread; the controller
// pointer is guarded because background reconnect goroutines swap it.
type appUI struct {
	app   fyne.App
	win   fyne.Window
	prefs prefStore

	header  *headerView
	flow    *powerFlow
	details *detailsView
	history *historyView

	settings      settings // UI-thread only
	modelOverride string   // UI-thread only

	connectMu sync.Mutex  // serialises reconnects
	ctrlMu    sync.Mutex  // guards ctrl
	ctrl      *controller // current poller; nil until first connect

	pauseBtn *widget.Button
}

func newAppUI(a fyne.App, w fyne.Window) *appUI {
	u := &appUI{app: a, win: w, prefs: a.Preferences()}
	u.settings = loadSettings(u.prefs)
	u.modelOverride = u.settings.ModelOverride

	u.header = newHeaderView()
	u.flow = newPowerFlow()
	u.details = newDetailsView()
	u.history = newHistoryView()

	tabs := container.NewAppTabs(
		container.NewTabItem("Power Flow", u.flow),
		container.NewTabItem("Details", u.details.root),
		container.NewTabItem("History", u.history.root),
	)
	if fyne.CurrentDevice().IsMobile() {
		tabs.SetTabLocation(container.TabLocationBottom)
	}

	top := container.NewVBox(u.header.root, u.buildToolbar(), widget.NewSeparator())
	u.win.SetContent(container.NewBorder(top, nil, nil, nil, tabs))
	u.win.SetOnClosed(func() {
		if c := u.getCtrl(); c != nil {
			go c.close() // fire-and-forget; the process is exiting
		}
	})
	return u
}

func (u *appUI) getCtrl() *controller {
	u.ctrlMu.Lock()
	defer u.ctrlMu.Unlock()
	return u.ctrl
}

func (u *appUI) setCtrl(c *controller) {
	u.ctrlMu.Lock()
	u.ctrl = c
	u.ctrlMu.Unlock()
}

func (u *appUI) buildToolbar() fyne.CanvasObject {
	u.pauseBtn = widget.NewButtonWithIcon("", theme.MediaPauseIcon(), u.togglePause)
	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		if c := u.getCtrl(); c != nil {
			c.pollNow()
		}
	})
	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), u.openSettings)
	// The last-update time sits right of the refresh button (owned by the header,
	// which keeps it current); it is vertically centred against the buttons.
	updated := container.NewCenter(u.header.updated)
	return container.NewHBox(u.pauseBtn, refreshBtn, updated, layout.NewSpacer(), settingsBtn)
}

func (u *appUI) run() {
	if !fyne.CurrentDevice().IsMobile() {
		u.win.Resize(fyne.NewSize(540, 760))
	}
	if u.settings.valid() {
		go u.connect(u.settings)
	} else {
		// First run with no IP: open settings once the event loop is live.
		go fyne.Do(u.openSettings)
	}
	u.win.ShowAndRun()
}

func (u *appUI) togglePause() {
	c := u.getCtrl()
	if c == nil {
		return
	}
	paused := !c.isPaused()
	c.setPaused(paused)
	if paused {
		u.pauseBtn.SetIcon(theme.MediaPlayIcon())
	} else {
		u.pauseBtn.SetIcon(theme.MediaPauseIcon())
	}
}

// onUpdate is the controller callback, bound to a specific controller c. It runs
// on the UI thread (the controller dispatches via fyne.Do). Updates from a
// controller that has since been replaced are ignored. On error it flags stale
// data but keeps the last good reading on screen.
func (u *appUI) onUpdate(c *controller, r *deye.Reading, err error) {
	if u.getCtrl() != c {
		return // stale callback from a replaced controller
	}
	if err != nil {
		u.header.setStale(true, err)
		return
	}
	eta := chargeETAText(c, r)
	u.header.update(r, u.modelOverride)
	u.flow.setReading(r)
	u.flow.setChargeETA(eta)
	u.details.update(r)
	u.details.setChargeETA(eta)
	u.history.refresh(c)
}

// chargeETAText returns the formatted time-to-full label for the battery node,
// or "" when the battery is not charging or no estimate is available yet.
func chargeETAText(c *controller, r *deye.Reading) string {
	if r.Values["bat_power"] >= 0 {
		return "" // only meaningful while charging
	}
	if d, ok := c.chargeETA(); ok {
		return deye.FormatETA(d)
	}
	return ""
}

// connect builds the data source (auto-discovering the serial if needed), tears
// down any previous controller, and starts polling. It runs on a background
// goroutine so the blocking discovery and teardown never freeze the UI; reconnects
// are serialised so a new source fully replaces the old one.
func (u *appUI) connect(s settings) {
	u.connectMu.Lock()
	defer u.connectMu.Unlock()

	client, serial, err := buildClient(s)
	if err != nil {
		fyne.Do(func() { u.showConnError(err) })
		return
	}

	if old := u.getCtrl(); old != nil {
		_ = old.close() // blocks until the old poller exits and its source closes
	}

	c := newController(client, s.interval())
	c.onUpdate = func(r *deye.Reading, e error) { u.onUpdate(c, r, e) }
	u.setCtrl(c)
	c.start()

	fyne.Do(func() { u.persistDiscoveredSerial(serial) })
}

// applySettings persists new settings and reconnects with them.
func (u *appUI) applySettings(s settings) {
	s = s.normalized()
	u.settings = s
	u.modelOverride = s.ModelOverride
	s.save(u.prefs)
	go u.connect(s)
}

// persistDiscoveredSerial saves a serial learned via auto-discovery so later runs
// skip the web-UI lookup. UI-thread only.
func (u *appUI) persistDiscoveredSerial(serial uint32) {
	if serial != 0 && serial != u.settings.Serial {
		u.settings.Serial = serial
		u.settings.save(u.prefs)
	}
}

func (u *appUI) showConnError(err error) {
	u.header.setStale(true, err)
	dialog.ShowError(fmt.Errorf("%w\n\nCheck the logger IP on the same Wi-Fi, "+
		"or enter the logger serial manually in Settings", err), u.win)
}

// openSettings shows the connection settings form, pre-filled from the current
// configuration.
func (u *appUI) openSettings() {
	s := u.settings

	ipE := widget.NewEntry()
	ipE.SetText(s.IP)
	ipE.SetPlaceHolder("192.168.x.x  (required)")
	serialE := widget.NewEntry()
	if s.Serial != 0 {
		serialE.SetText(strconv.FormatUint(uint64(s.Serial), 10))
	}
	serialE.SetPlaceHolder("auto-discover if empty")
	portE := widget.NewEntry()
	portE.SetText(strconv.Itoa(s.Port))
	intervalE := widget.NewEntry()
	intervalE.SetText(strconv.Itoa(s.IntervalSec))
	userE := widget.NewEntry()
	userE.SetText(s.HTTPUser)
	userE.SetPlaceHolder("admin")
	passE := widget.NewPasswordEntry()
	passE.SetText(s.HTTPPass)
	modelE := widget.NewEntry()
	modelE.SetText(s.ModelOverride)
	modelE.SetPlaceHolder("auto from device")

	items := []*widget.FormItem{
		widget.NewFormItem("Logger IP", ipE),
		widget.NewFormItem("Logger serial", serialE),
		widget.NewFormItem("Port", portE),
		widget.NewFormItem("Interval (s)", intervalE),
		widget.NewFormItem("Web user", userE),
		widget.NewFormItem("Web password", passE),
		widget.NewFormItem("Model override", modelE),
	}

	d := dialog.NewForm("Settings", "Save", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		ns := settings{
			IP:            strings.TrimSpace(ipE.Text),
			Port:          atoiOr(portE.Text, defaultPort),
			IntervalSec:   atoiOr(intervalE.Text, defaultIntervalSec),
			HTTPUser:      strings.TrimSpace(userE.Text),
			HTTPPass:      passE.Text,
			ModelOverride: strings.TrimSpace(modelE.Text),
		}
		if v := strings.TrimSpace(serialE.Text); v != "" {
			if n, perr := strconv.ParseUint(v, 10, 32); perr == nil {
				ns.Serial = uint32(n)
			}
		}
		if !ns.valid() {
			dialog.ShowError(errors.New("Logger IP is required"), u.win)
			return
		}
		u.applySettings(ns)
	}, u.win)
	d.Resize(fyne.NewSize(440, 400))
	d.Show()
}

// buildClient constructs a Deye client from settings, auto-discovering the logger
// serial from its web UI when one is not configured. It returns the serial in
// effect so the caller can persist a discovered value.
func buildClient(s settings) (*deye.Client, uint32, error) {
	serial := s.Serial
	if serial == 0 {
		d, err := deye.DiscoverSerial(s.IP, s.HTTPUser, s.HTTPPass)
		if err != nil {
			return nil, 0, fmt.Errorf("serial auto-discovery failed: %w", err)
		}
		serial = d
	}
	c := deye.New(deye.Config{IP: s.IP, Serial: serial, Port: s.Port})
	return c, serial, nil
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n
	}
	return def
}
