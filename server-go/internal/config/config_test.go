package config

import "testing"

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in       string
		fallback int64
		want     int64
	}{
		{"", 2 * 1024 * 1024, 2 * 1024 * 1024},
		{"  ", 2 * 1024 * 1024, 2 * 1024 * 1024},
		{"garbage", 2 * 1024 * 1024, 2 * 1024 * 1024},
		{"0", 2 * 1024 * 1024, 2 * 1024 * 1024},
		{"-5mb", 2 * 1024 * 1024, 2 * 1024 * 1024},

		{"2mb", 0, 2 * 1024 * 1024},
		{"2MB", 0, 2 * 1024 * 1024},
		{"500kb", 0, 500 * 1024},
		{"1gb", 0, 1024 * 1024 * 1024},
		{"1024", 0, 1024},
		{"2097152b", 0, 2 * 1024 * 1024},
	}
	for _, tc := range cases {
		got := parseBytes(tc.in, tc.fallback)
		if got != tc.want {
			t.Errorf("parseBytes(%q, %d) = %d, want %d", tc.in, tc.fallback, got, tc.want)
		}
	}
}

func TestTrustProxyEnabled(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"true", true},
		{"yes", true},
		{"on", true},
		{"1", true},
		{"0", false}, // Atoi("0") = 0; TrustProxyEnabled treats >0 as trust
		{"loopback", true},
	}
	for _, tc := range cases {
		c := &Config{RawTrustProxy: tc.raw}
		if got := c.TrustProxyEnabled(); got != tc.want {
			t.Errorf("TrustProxyEnabled(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
