package gui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/n0madic/deye-monitor/deye"
)

// headerView is the always-visible status strip: model, serial, heartbeat,
// device/work state, last-update time and a stale indicator.
type headerView struct {
	root    fyne.CanvasObject
	model   *widget.Label
	ident   *widget.Label
	heart   *widget.Label
	state   *widget.Label
	updated *widget.Label
	stale   *canvas.Text
}

func newHeaderView() *headerView {
	h := &headerView{
		model:   widget.NewLabelWithStyle("Deye", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		ident:   widget.NewLabel(""),
		heart:   widget.NewLabel(""),
		state:   widget.NewLabel("connecting…"),
		updated: widget.NewLabel(""),
		stale:   canvas.NewText("", colRed),
	}
	h.stale.TextStyle = fyne.TextStyle{Bold: true}
	h.state.Truncation = fyne.TextTruncateEllipsis // long work-mode text stays on one line

	// Line 1: model + serial on the left, heartbeat pushed to the right.
	line1 := container.NewHBox(h.model, h.ident, layout.NewSpacer(), h.heart)
	// Line 2: state/work-mode fills the row and truncates; the stale flag sits on the
	// left. The update time is placed next to the refresh button in the toolbar.
	line2 := container.NewBorder(nil, nil, h.stale, nil, h.state)
	h.root = container.NewVBox(line1, line2)
	return h
}

// update fills the header from a fresh reading and clears any stale flag.
func (h *headerView) update(r *deye.Reading, override string) {
	if r == nil {
		return
	}
	h.model.SetText(displayModelGUI(override, r))
	h.ident.SetText("SN " + r.Serial)
	h.heart.SetText(heartbeatText(r))
	h.state.SetText(fmt.Sprintf("State: %s · Mode: %s",
		dash(r.State("device_state")), dash(r.State("work_mode"))))
	h.updated.SetText("updated " + r.Time.Format("15:04:05"))
	h.setStale(false, nil)
}

// setStale shows or clears the stale-data warning without disturbing the last
// good reading already shown in the rest of the header.
func (h *headerView) setStale(stale bool, err error) {
	if stale {
		msg := "stale"
		if err != nil {
			msg = "stale: " + err.Error()
		}
		h.stale.Text = "⚠ " + msg
	} else {
		h.stale.Text = ""
	}
	h.stale.Refresh()
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// heartbeatText renders the logger heartbeat indicator: a filled ♥ on the cycle a
// heartbeat lands, otherwise a hollow ♡, with the running count and age.
func heartbeatText(r *deye.Reading) string {
	if r.Heartbeats == 0 {
		return ""
	}
	heart := "♡"
	if r.HeartbeatNow {
		heart = "♥"
	}
	ago := ""
	if !r.LastHeartbeat.IsZero() {
		ago = fmt.Sprintf(" %ds", int(time.Since(r.LastHeartbeat).Seconds()))
	}
	return fmt.Sprintf("%s ×%d%s", heart, r.Heartbeats, ago)
}

// displayModelGUI resolves the model label: the user override wins, otherwise the
// label derived from the logger, otherwise a generic fallback.
func displayModelGUI(override string, r *deye.Reading) string {
	if override != "" {
		return override
	}
	if r != nil && r.Model != "" {
		return r.Model
	}
	return "Deye"
}
