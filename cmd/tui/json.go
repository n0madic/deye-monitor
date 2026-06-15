package main

import (
	"encoding/json"
	"fmt"
	"os"

	"deye-monitor/deye"
)

// runJSON prints a single snapshot as indented JSON (for cron/Grafana/pipes).
func runJSON(c *deye.Client, model string) {
	r, err := c.Snapshot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	b, err := toJSON(r, model)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func toJSON(r *deye.Reading, model string) ([]byte, error) {
	m := map[string]any{
		"timestamp":  r.Time.Format("2006-01-02 15:04:05"),
		"serial":     r.Serial,
		"model":      displayModel(model, r),
		"heartbeats": r.Heartbeats,
	}
	metricsOut := make(map[string]any, len(r.Values)+len(r.States)+1)
	for k, v := range r.Values {
		metricsOut[k] = v
	}
	for k, v := range r.States {
		metricsOut[k] = v
	}
	metricsOut["pv_total_p"] = r.PVTotal()
	m["metrics"] = metricsOut
	return json.MarshalIndent(m, "", "  ")
}
