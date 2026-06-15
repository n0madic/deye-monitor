package deye

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// coverMidPattern matches the logger serial (module id) in the stick's web UI
// status page: var cover_mid = "3566613625";
var coverMidPattern = regexp.MustCompile(`cover_mid\s*=\s*"(\d+)"`)

// DiscoverSerial fetches the logger's own serial from its built-in web UI
// (http://<ip>/status.html, HTTP Basic Auth). user/pass default to admin/admin
// when empty.
//
// This is the LSW-5 stick serial that New needs for Solarman V5 framing — the
// one otherwise only printed on the stick and used in its AP SSID
// (AP_<serial>). It is distinct from the inverter serial in Reading.Serial.
func DiscoverSerial(ip, user, pass string) (uint32, error) {
	if user == "" {
		user = "admin"
	}
	if pass == "" {
		pass = "admin"
	}
	url := "http://" + ip + "/status.html"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(user, pass)

	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", url, err)
	}
	m := coverMidPattern.FindSubmatch(body)
	if m == nil {
		return 0, fmt.Errorf("logger serial (cover_mid) not found at %s", url)
	}
	n, err := strconv.ParseUint(string(m[1]), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse logger serial %q: %w", m[1], err)
	}
	return uint32(n), nil
}
