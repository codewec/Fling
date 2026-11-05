package config

import (
	"net/netip"
	"testing"
)

func TestIsTrusted(t *testing.T) {
	c := &Config{
		TrustedNetworks: []netip.Prefix{
			netip.MustParsePrefix("192.168.0.0/16"),
			netip.MustParsePrefix("fd00::/8"),
		},
		TrustedHosts: []string{"lan.internal"},
	}

	cases := []struct {
		remote, host string
		want         bool
	}{
		{"192.168.1.50:1234", "example.com", true},
		{"192.168.1.50", "example.com", true},
		{"[fd00::1]:80", "example.com", true},
		{"203.0.113.5:443", "lan.internal", true},     // trusted host
		{"203.0.113.5:443", "lan.internal:8080", true}, // host with port
		{"203.0.113.5:443", "example.com", false},
		{"10.1.2.3:80", "example.com", false},
		{"garbage", "example.com", false},
	}
	for _, tc := range cases {
		if got := c.IsTrusted(tc.remote, tc.host); got != tc.want {
			t.Errorf("IsTrusted(%q, %q) = %v, want %v", tc.remote, tc.host, got, tc.want)
		}
	}
}

func TestParseCIDRsSkipsInvalid(t *testing.T) {
	got := parseCIDRs("10.0.0.0/8, bogus, 192.168.1.0/24")
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
