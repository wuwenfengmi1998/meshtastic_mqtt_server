package web

import (
	"context"
	"net/netip"
	"testing"
)

func TestIsPublicAddr(t *testing.T) {
	cases := []struct {
		ip    string
		allow bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2001:4860:4860::8888", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"10.0.0.1", false},
		{"172.16.5.4", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false}, // AWS metadata
		{"100.64.0.1", false},      // CGNAT
		{"100.127.255.254", false}, // CGNAT
		{"224.0.0.1", false},       // multicast
		{"0.0.0.0", false},
		{"fc00::1", false}, // ULA
		{"fe80::1", false}, // link-local IPv6
		{"", false},
	}
	for _, tc := range cases {
		addr, err := netip.ParseAddr(tc.ip)
		got := false
		if err == nil {
			got = isPublicAddr(addr)
		}
		if got != tc.allow {
			t.Errorf("isPublicAddr(%q)=%v, want %v", tc.ip, got, tc.allow)
		}
	}
}

func TestIsMapTileDialTargetAllowed(t *testing.T) {
	cases := []struct {
		network string
		addr    string
		allow   bool
	}{
		{"tcp", "8.8.8.8:443", true},
		{"tcp4", "1.1.1.1:80", true},
		{"tcp", "127.0.0.1:80", false},
		{"tcp", "10.0.0.1:80", false},
		{"tcp", "169.254.169.254:80", false},
		{"tcp", "100.64.1.1:80", false},
		{"tcp6", "[::1]:1883", false},
		{"unix", "/tmp/x.sock", false},
		{"tcp", "no-such-host.invalid:80", false},
		{"tcp", "localhost:80", false}, // 解析到 127.0.0.1
	}
	for _, tc := range cases {
		if got := isMapTileDialTargetAllowed(context.Background(), tc.network, tc.addr); got != tc.allow {
			t.Errorf("isMapTileDialTargetAllowed(%q, %q)=%v, want %v", tc.network, tc.addr, got, tc.allow)
		}
	}
}

func TestMapTileContentType(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if got := mapTileContentType(png); got != "image/png" {
		t.Errorf("png: got %q", got)
	}
	html := []byte("<html><script>alert(1)</script></html>")
	if got := mapTileContentType(html); got != "application/octet-stream" {
		t.Errorf("html must be blocked, got %q", got)
	}
	svg := []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")
	if got := mapTileContentType(svg); got != "application/octet-stream" {
		t.Errorf("svg must be blocked, got %q", got)
	}
}

func TestIsLoopbackHost(t *testing.T) {
}
