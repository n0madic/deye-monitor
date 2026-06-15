package gui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"deye-monitor/deye"
)

// num formats a float with the minimal decimal representation (1319, 54.33, 12.2).
func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// metricLabels maps each metric key to its human label, derived once from the
// canonical register map so the Details screen and the data layer never drift.
var metricLabels = func() map[string]string {
	m := make(map[string]string, len(deye.Metrics))
	for _, mt := range deye.Metrics {
		m[mt.Key] = mt.Label
	}
	return m
}()

func metricLabel(key string) string {
	if l, ok := metricLabels[key]; ok {
		return l
	}
	return key
}

// detailsView is the Details tab: a scrollable set of cards whose value labels
// are refreshed in place from each reading.
type detailsView struct {
	root     fyne.CanvasObject
	updaters []func(*deye.Reading)
}

func newDetailsView() *detailsView {
	d := &detailsView{}
	cards := container.NewVBox(
		widget.NewCard("Status", "", d.form(
			d.stateRow("device_state"),
			d.stateRow("work_mode"),
		)),
		widget.NewCard("Solar", "", d.form(
			d.metricRow("pv1_v", "V"), d.metricRow("pv1_i", "A"), d.metricRow("pv1_p", "W"),
			d.metricRow("pv2_v", "V"), d.metricRow("pv2_i", "A"), d.metricRow("pv2_p", "W"),
			d.metricRow("pv3_p", "W"), d.metricRow("pv4_p", "W"),
			d.computedRow("PV total", "W", func(r *deye.Reading) float64 { return r.PVTotal() }),
		)),
		widget.NewCard("Battery", "", d.form(
			d.metricRow("bat_soc", "%"), d.metricRow("bat_v", "V"),
			d.metricRow("bat_power", "W"), d.metricRow("bat_temp", "°C"),
		)),
		widget.NewCard("Grid", "", d.form(
			d.metricRow("grid_l1_v", "V"), d.metricRow("grid_l2_v", "V"), d.metricRow("grid_l3_v", "V"),
			d.metricRow("grid_freq", "Hz"), d.metricRow("grid_power", "W"),
			d.metricRow("grid_l1_p", "W"), d.metricRow("grid_l2_p", "W"), d.metricRow("grid_l3_p", "W"),
		)),
		widget.NewCard("Load", "", d.form(
			d.metricRow("load_l1_v", "V"), d.metricRow("load_l2_v", "V"), d.metricRow("load_l3_v", "V"),
			d.metricRow("load_freq", "Hz"), d.metricRow("load_power", "W"),
			d.metricRow("load_l1_p", "W"), d.metricRow("load_l2_p", "W"), d.metricRow("load_l3_p", "W"),
		)),
		widget.NewCard("Temperatures", "", d.form(
			d.metricRow("temp_dc", "°C"), d.metricRow("temp_ac", "°C"), d.metricRow("bat_temp", "°C"),
		)),
		widget.NewCard("Energy today", "", d.form(
			d.metricRow("e_today_pv", "kWh"), d.metricRow("e_today_load", "kWh"),
			d.metricRow("e_today_chg", "kWh"), d.metricRow("e_today_dis", "kWh"),
			d.metricRow("e_today_imp", "kWh"), d.metricRow("e_today_exp", "kWh"),
		)),
		widget.NewCard("Energy total", "", d.form(
			d.metricRow("e_total_pv", "kWh"), d.metricRow("e_total_load", "kWh"),
			d.metricRow("e_total_chg", "kWh"), d.metricRow("e_total_dis", "kWh"),
			d.metricRow("e_total_imp", "kWh"), d.metricRow("e_total_exp", "kWh"),
		)),
	)
	d.root = container.NewVScroll(cards)
	return d
}

func (d *detailsView) form(items ...*widget.FormItem) *widget.Form {
	return &widget.Form{Items: items}
}

// metricRow makes a value row bound to a metric key; the value is shown with its
// unit, or "n/a" when the register was missing from the reading.
func (d *detailsView) metricRow(key, unit string) *widget.FormItem {
	lbl := widget.NewLabel("—")
	lbl.Alignment = fyne.TextAlignTrailing
	d.updaters = append(d.updaters, func(r *deye.Reading) {
		if v, ok := r.Get(key); ok {
			lbl.SetText(num(v) + " " + unit)
		} else {
			lbl.SetText("n/a")
		}
	})
	return widget.NewFormItem(metricLabel(key), lbl)
}

// computedRow makes a value row from a function of the whole reading.
func (d *detailsView) computedRow(label, unit string, fn func(*deye.Reading) float64) *widget.FormItem {
	lbl := widget.NewLabel("—")
	lbl.Alignment = fyne.TextAlignTrailing
	d.updaters = append(d.updaters, func(r *deye.Reading) {
		lbl.SetText(num(fn(r)) + " " + unit)
	})
	return widget.NewFormItem(label, lbl)
}

// stateRow makes a row bound to a decoded enum state (device_state, work_mode).
func (d *detailsView) stateRow(key string) *widget.FormItem {
	lbl := widget.NewLabel("—")
	lbl.Alignment = fyne.TextAlignTrailing
	d.updaters = append(d.updaters, func(r *deye.Reading) {
		if s := r.State(key); s != "" {
			lbl.SetText(s)
		} else {
			lbl.SetText("n/a")
		}
	})
	return widget.NewFormItem(metricLabel(key), lbl)
}

// update refreshes every bound value from r.
func (d *detailsView) update(r *deye.Reading) {
	if r == nil {
		return
	}
	for _, u := range d.updaters {
		u(r)
	}
}
