// Command deye-monitor reads real-time statistics from a Deye SUN-12K-SG05LP3
// inverter through its LSW-5 WiFi data logger using the Solarman V5 protocol
// (Modbus over TCP, port 8899). No cloud servers involved.
//
// The data layer lives in the reusable deye-monitor/deye package; this command
// is a thin CLI/TUI on top of it.
//
//	go run ./cmd/tui                 # interactive termui dashboard, refresh every 5s
//	go run ./cmd/tui -interval 2s    # refresh every 2s
//	go run ./cmd/tui -plain          # text dashboard loop instead of the TUI
//	go run ./cmd/tui -once           # single text snapshot then exit
//	go run ./cmd/tui -json           # single snapshot as JSON (for cron/pipes)
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/n0madic/deye-monitor/deye"
)

// Non-personal defaults. The logger IP and serial have no defaults — they
// identify a specific installation and must be provided via flag or env.
const (
	defaultPort     = 8899            // standard Solarman V5 port
	defaultInterval = 5 * time.Second // refresh cadence
	fallbackModel   = "Deye"          // used when no override is set and the device label is unknown
)

func main() {
	ip := flag.String("ip", os.Getenv("DEYE_IP"), "logger IP address (env DEYE_IP) [required]")
	serial := flag.Uint("serial", uint(envUintOr("DEYE_SERIAL", 0)), "logger serial; auto-discovered from the logger web UI if unset (env DEYE_SERIAL)")
	port := flag.Int("port", int(envUintOr("DEYE_PORT", defaultPort)), "logger TCP port (env DEYE_PORT)")
	interval := flag.Duration("interval", envDurationOr("DEYE_INTERVAL", defaultInterval), "refresh interval, e.g. 2s, 5s (env DEYE_INTERVAL)")
	model := flag.String("model", os.Getenv("DEYE_MODEL"), "model label override; auto-derived from the logger if unset (env DEYE_MODEL)")
	once := flag.Bool("once", false, "single snapshot then exit")
	asJSON := flag.Bool("json", false, "single snapshot as JSON")
	plainMode := flag.Bool("plain", false, "force the text dashboard instead of the termui TUI")
	flag.Parse()

	if *ip == "" {
		fatalUsage("missing logger IP: set -ip or DEYE_IP")
	}

	serialN := uint32(*serial)
	if serialN == 0 {
		// Auto-discover the logger serial from its built-in web UI.
		s, err := deye.DiscoverSerial(*ip, os.Getenv("DEYE_HTTP_USER"), os.Getenv("DEYE_HTTP_PASS"))
		if err != nil {
			fatalUsage(fmt.Sprintf("missing logger serial and auto-discovery failed: %v\n"+
				"  set -serial / DEYE_SERIAL (printed on the stick and in its AP SSID AP_<serial>),\n"+
				"  or set web-UI credentials via DEYE_HTTP_USER / DEYE_HTTP_PASS (default admin/admin)", err))
		}
		serialN = s
		fmt.Fprintf(os.Stderr, "discovered logger serial %d from http://%s\n", serialN, *ip)
	}

	client := deye.New(deye.Config{
		IP:     *ip,
		Serial: serialN,
		Port:   *port,
	})
	defer client.Close()

	switch {
	case *asJSON:
		runJSON(client, *model)
	case *once:
		runOnce(client, *ip, *model)
	case *plainMode || !isatty(os.Stdout):
		// termui needs a real terminal; fall back to the text loop otherwise.
		runPlainLoop(client, *ip, *model, *interval)
	default:
		runTUI(client, *ip, *model, *interval)
	}
}

// fatalUsage prints an error and the flag usage, then exits with code 2.
func fatalUsage(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	flag.Usage()
	os.Exit(2)
}

// displayModel resolves the model label to show: the CLI/env override wins,
// otherwise the label derived from the logger, otherwise a generic fallback.
func displayModel(override string, r *deye.Reading) string {
	if override != "" {
		return override
	}
	if r != nil && r.Model != "" {
		return r.Model
	}
	return fallbackModel
}

func envUintOr(key string, def uint64) uint64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envDurationOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func isatty(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
