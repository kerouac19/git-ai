package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresForwardedHeadersWhenUntrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.Header.Set("X-Real-Ip", "9.9.9.9")

	got := ClientIP(req, false)
	if got != "10.0.0.1" {
		t.Fatalf("ClientIP(untrusted) = %q, want %q", got, "10.0.0.1")
	}
}

func TestClientIPPrefersXFFWhenTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.Header.Set("X-Real-Ip", "9.9.9.9")

	got := ClientIP(req, true)
	if got != "1.2.3.4" {
		t.Fatalf("ClientIP(trusted) = %q, want %q", got, "1.2.3.4")
	}
}

func TestClientIPFallsBackToXRealIPWhenTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	req.Header.Set("X-Real-Ip", "9.9.9.9")

	got := ClientIP(req, true)
	if got != "9.9.9.9" {
		t.Fatalf("ClientIP(trusted, no XFF) = %q, want %q", got, "9.9.9.9")
	}
}

func TestClientIPUsesRemoteAddrWhenNoHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.5:443"

	if got := ClientIP(req, true); got != "192.168.1.5" {
		t.Fatalf("ClientIP(trusted, no headers) = %q, want %q", got, "192.168.1.5")
	}
	if got := ClientIP(req, false); got != "192.168.1.5" {
		t.Fatalf("ClientIP(untrusted) = %q, want %q", got, "192.168.1.5")
	}
}

func TestForwardedProtoOrDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "HTTPS, http")

	if got := ForwardedProtoOrDefault(req, true, "http"); got != "https" {
		t.Fatalf("trusted = %q, want https", got)
	}
	if got := ForwardedProtoOrDefault(req, false, "http"); got != "http" {
		t.Fatalf("untrusted = %q, want http", got)
	}

	req.Header.Del("X-Forwarded-Proto")
	if got := ForwardedProtoOrDefault(req, true, "http"); got != "http" {
		t.Fatalf("trusted, no header = %q, want http", got)
	}
}
