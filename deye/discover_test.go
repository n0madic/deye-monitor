package deye

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// statusPage is a trimmed status.html with the JS vars the real stick serves.
const statusPage = `var webdata_sn = "2508064166      ";
var cover_mid = "3566613625";
var cover_ver = "LSW5_32_5406_SS_04_00.00.00.0D";
var cover_ap_ssid = "AP_3566613625";`

func TestDiscoverSerial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "admin" {
			w.Header().Set("WWW-Authenticate", `Basic realm="logger"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/status.html" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(statusPage))
	}))
	defer srv.Close()

	ip := strings.TrimPrefix(srv.URL, "http://") // host:port

	got, err := DiscoverSerial(ip, "", "") // empty creds -> admin/admin
	if err != nil {
		t.Fatalf("DiscoverSerial: %v", err)
	}
	if got != 3566613625 {
		t.Fatalf("DiscoverSerial = %d, want 3566613625", got)
	}
}

func TestDiscoverSerialBadAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := DiscoverSerial(strings.TrimPrefix(srv.URL, "http://"), "x", "y"); err == nil {
		t.Fatal("DiscoverSerial should fail on 401")
	}
}

func TestDiscoverSerialMissingVar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("var webdata_sn = \"123\";"))
	}))
	defer srv.Close()

	if _, err := DiscoverSerial(strings.TrimPrefix(srv.URL, "http://"), "", ""); err == nil {
		t.Fatal("DiscoverSerial should fail when cover_mid is absent")
	}
}
