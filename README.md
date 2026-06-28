# Deye monitor - local real-time monitoring in Go

`deye-monitor` is a Go utility for direct polling of a **Deye** inverter
through a Wi-Fi logger using **Solarman V5** (Modbus over TCP, port `8899`).
No cloud dependency.

The project has three parts:

- **`deye/`** - reusable data package (client, retries, register map,
  decoding). It is independent of CLI/TUI and imported as `deye-monitor/deye`
  (see [below](#reusable-deye-package)).
- **`cmd/tui/`** - thin CLI/TUI layer on top of the `deye` package.
- **`cmd/gui/`** - desktop/mobile GUI entry point based on Fyne.
  The visual components and polling controller live in **`gui/`**.

## Build and run (TUI)

```bash
go build -o deye-monitor ./cmd/tui

# Only the logger IP is required - via flag or env:
./deye-monitor -ip 192.168.50.171   # logger serial will be auto-detected
export DEYE_IP=192.168.50.171       # ... or via env

./deye-monitor               # interactive TUI (termui), refresh every 5s
./deye-monitor -interval 2s  # refresh every 2s (Go Duration: 2s, 500ms, 1m)
./deye-monitor -plain        # looped plain-text dashboard (no termui)
./deye-monitor -once         # single snapshot and exit
./deye-monitor -json         # single snapshot as JSON (for cron/Grafana)
```

Without building: `go run ./cmd/tui -ip 192.168.50.171 -once`.

By default, the tool starts **termui TUI** (ASCII-style dashboard). If stdout
is not a terminal (pipe/file output) or `-plain` is set, it automatically
switches to plain-text dashboard mode because termui requires a real TTY.

## Desktop and mobile GUI (`cmd/gui`)

`cmd/gui` is a cross-platform graphical app powered by
[`fyne.io/fyne/v2`](https://github.com/fyne-io/fyne). It uses the same
`deye-monitor/deye` data layer and polls the inverter over the local network.

Features:

- **Power Flow tab** - Deye-cloud-style diagram with inverter/PV/grid/battery/load
  nodes, directional arrows, and color-coded flows. The battery node also shows
  the estimated time until full while charging (e.g. `80% · full 1h23m`).
- **Details tab** - live metrics cards (status, PV, battery, grid, load,
  temperatures, daily/total energy). The Battery card adds a `Time to full` row
  with the estimated time remaining while charging.
- **History tab** - rolling line charts for PV, load, grid, and battery power.
- **Settings dialog** - logger IP, serial, port, interval, web credentials,
  and model override. If serial is empty, it is auto-discovered from the
  logger web UI and persisted for the next launch.

Run on desktop:

```bash
go run ./cmd/gui
```

The app stores settings in Fyne Preferences (application ID `com.deye.monitor`),
so once configured, it reconnects automatically on next start.

Package with `make` targets:

```bash
make gui          # host OS package (artifact in cmd/gui/)
make gui-darwin   # macOS .app
make gui-linux    # Linux package (run on Linux)
make gui-windows  # Windows package (run on Windows)
make gui-ios      # iOS package (requires Xcode on macOS)
```

Android APK:

```bash
make gui-android
```

`cmd/gui/build-android.sh` builds a sideload-ready APK with a modern
target SDK and re-signs it with `apksigner` (v1+v2+v3), which is required on
recent Android versions.

## Flags and environment variables

| Flag | Env | Default |
|---|---|---|
| `-ip` | `DEYE_IP` | - (**required**) |
| `-serial` | `DEYE_SERIAL` | auto from logger web UI |
| `-port` | `DEYE_PORT` | `8899` |
| `-interval` | `DEYE_INTERVAL` | `5s` (Go Duration format) |
| `-model` | `DEYE_MODEL` | auto from logger (override) |
| - | `DEYE_HTTP_USER` / `DEYE_HTTP_PASS` | `admin` / `admin` (for SN autodiscovery) |
| `-plain` | - | plain-text dashboard instead of TUI |
| `-once` / `-json` | - | single snapshot (text / JSON) |

> Important: `-interval` expects a **Go Duration** (`2s`, `500ms`, `1m`), not
> a plain number. `-interval 2` is invalid; use `-interval 2s`.

### Where model and serial numbers come from

You only need to set the **logger IP** manually. Everything else is detected:

- **Logger serial** (required by Solarman V5 for addressing) is pulled from the
  logger's built-in web UI (`http://<ip>/status.html`, `cover_mid` field,
  HTTP Basic Auth `admin`/`admin`). If credentials were changed, set
  `DEYE_HTTP_USER` / `DEYE_HTTP_PASS`. If the web UI is unavailable, provide
  `-serial` / `DEYE_SERIAL` manually (printed on the stick and in its Wi-Fi AP
  name `AP_<serial>`).
- **Inverter serial** (header value like `SN 2508064166`) is read from the
  inverter via Modbus (registers 3-7).
- **Model** (for example `Deye 12kW 3P`) is derived from `Device Rated Power`
  (0x14-0x15) and `Device Phases` (0x16). Exact SKU string
  (`SUN-12K-SG05LP3`) is not available in these registers. If you need an exact
  SKU label, set `-model` / `DEYE_MODEL`.

## TUI (termui)

Interactive dashboard built with
[`github.com/gizak/termui/v3`](https://github.com/gizak/termui):

- **Battery charge gauge** - SOC bar with level-based color (green >=50%,
  yellow >=20%, red below), plus charge/discharge power and temperature. While
  charging, the gauge label also shows a `full in 1h23m` estimate of the time
  remaining until the battery is full (also shown on the plain/`-plain` battery
  line).
- **Power sparklines** - history (up to 120 points) for PV, load, grid, and
  battery. Values are absolute; sign (import/export, charge/discharge) is shown
  in labels.
- **Flow diagram** - web-UI-like flow chart in terminal graphics: inverter
  (`INV`) in the center, PV / GRID / BAT / LOAD nodes in corners, connected by
  lines. Arrows and color indicate flow direction (import/export,
  charge/discharge). Adapts to panel size; on narrow terminals it collapses to a
  text list.
- **Details table** - PV strings (V/I/W), per-phase grid/load voltages and
  frequency, temperatures (DC/AC/BAT), daily and total energy counters.

### Keys

| Key | Action |
|---|---|
| `q` / `Ctrl-C` | quit |
| `p` | pause/resume polling |
| `+` / `-` | increase / decrease refresh interval |

Polling runs in a separate goroutine, so keyboard input remains responsive even
during register reads.

## Reusable `deye` package

The data layer is split into a dedicated package `deye-monitor/deye`, which can
be imported by other projects independently of the CLI/TUI:

```go
package main

import (
	"fmt"
	"log"

	"deye-monitor/deye"
)

func main() {
	c := deye.New(deye.Config{
		IP:     "192.168.50.171",
		Serial: 3566613625,
		// Port, Timeout, Attempts are optional (defaults: 8899, 8s, 2)
	})
	defer c.Close()

	r, err := c.Snapshot() // read live blocks and decode them
	if err != nil {
		log.Fatal(err)
	}

	soc, _ := r.Get("bat_soc")
	fmt.Printf("%s · SOC %.0f%%, PV %.0f W, mode %q\n",
		r.Model, soc, r.PVTotal(), r.State("work_mode"))
}
```

Public API surface: `deye.New(Config)`, `Client.Snapshot()`,
`Client.Device()` (serial, power, phases, model), `Client.Identity()`,
`Client.Heartbeats()`, `Client.Close()`; `deye.DiscoverSerial(ip, user, pass)`
(logger serial from web UI); types `Reading` (`Get`/`State`/`PVTotal`, fields
`Serial`/`Model`) and `DeviceInfo`; `Metrics` map and `DeviceState`/`WorkMode`
dictionaries.

> To consume this package from a **different repository** via `go get`, change
> the module path to a real VCS URL (for example `github.com/n0madic/deye`) and
> publish it. Inside this module, imports already work as-is.

## Heartbeat indicator ♥

The LSW-5 logger periodically sends service **heartbeat** frames (Solarman V5
control code `0x4710`). Since the connection is persistent, these frames can
appear in the stream between a request and its Modbus response.

The `snowirbis/solarman` library (starting from the fix in
[PR #1](https://github.com/snowirbis/solarman/pull/1)) matches responses to
requests by sequence number and automatically skips heartbeat/stale frames,
passing them into `OnUnsolicited(controlCode, frame)`. The `deye` package hooks
into this callback and counts heartbeats natively, without parsing error text
(as older implementations did).

- **Heartbeat as a feature.** On heartbeat, the dashboard header shows a pulsing
  **♥** (for that cycle). Between heartbeats, it shows dim **♡** with counter
  and time since last one: `♡ ×7 63s ago`. Same behavior in TUI header, in
  `-json` as `"heartbeats"`, and in API via `Client.Heartbeats()`.
- **Failure resilience.** Real network errors (timeout/disconnect) trigger
  socket close and reconnect on the next attempt (`Attempts`). If all attempts
  are exhausted, control falls back to the main loop backoff.
