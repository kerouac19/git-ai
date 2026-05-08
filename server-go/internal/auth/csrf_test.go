package auth

import (
	"strings"
	"testing"
)

func TestGenerateCSRFTokenIsRandom(t *testing.T) {
	a, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken err: %v", err)
	}
	b, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken err: %v", err)
	}
	if a == "" || b == "" {
		t.Fatalf("token should not be empty")
	}
	if a == b {
		t.Fatalf("two tokens should differ; got %q twice", a)
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %d", len(a))
	}
}

func TestSerializeCSRFCookieAttributes(t *testing.T) {
	got := SerializeCSRFCookie("abc123", 3600, false)
	for _, must := range []string{"csrf_token=abc123", "Path=/", "SameSite=Lax", "Max-Age=3600"} {
		if !strings.Contains(got, must) {
			t.Fatalf("missing %q in cookie %q", must, got)
		}
	}
	if strings.Contains(got, "HttpOnly") {
		t.Fatalf("CSRF cookie must NOT be HttpOnly: %q", got)
	}
	if strings.Contains(got, "Secure") {
		t.Fatalf("non-production cookie must not have Secure: %q", got)
	}

	prod := SerializeCSRFCookie("abc123", 3600, true)
	if !strings.Contains(prod, "Secure") {
		t.Fatalf("production cookie must have Secure: %q", prod)
	}
}

func TestClearCSRFCookie(t *testing.T) {
	got := ClearCSRFCookie(false)
	if !strings.Contains(got, "csrf_token=") {
		t.Fatalf("clear cookie missing name: %q", got)
	}
	if !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("clear cookie must have Max-Age=0: %q", got)
	}
	prod := ClearCSRFCookie(true)
	if !strings.Contains(prod, "Secure") {
		t.Fatalf("production cleared cookie must have Secure: %q", prod)
	}
}

func TestExtractCSRFTokenFromCookie(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"csrf_token=abc", "abc"},
		{"git_ai_session=xx; csrf_token=abc; other=y", "abc"},
		{"", ""},
		{"git_ai_session=xx", ""},
	}
	for _, tc := range cases {
		if got := ExtractCSRFTokenFromCookie(tc.header); got != tc.want {
			t.Errorf("Extract(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}
