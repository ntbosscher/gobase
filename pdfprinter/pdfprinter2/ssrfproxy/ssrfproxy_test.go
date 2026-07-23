package ssrfproxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := map[string]bool{
		"169.254.169.254": true,  // cloud metadata (link-local)
		"127.0.0.1":       true,  // loopback
		"10.0.0.5":        true,  // RFC1918
		"192.168.1.1":     true,  // RFC1918
		"172.16.0.1":      true,  // RFC1918
		"100.64.0.1":      true,  // carrier-grade NAT
		"::1":             true,  // IPv6 loopback
		"fd00::1":         true,  // IPv6 ULA
		"fe80::1":         true,  // IPv6 link-local
		"0.0.0.0":         true,  // unspecified
		"224.0.0.1":       true,  // multicast
		"93.184.216.34":   false, // public
		"1.1.1.1":         false, // public
		// IPv4-mapped IPv6 forms of blocked addresses must also be blocked.
		"::ffff:127.0.0.1":       true,
		"::ffff:169.254.169.254": true,
	}

	for in, want := range cases {
		ip := net.ParseIP(in)
		if ip == nil {
			t.Fatalf("bad test ip %q", in)
		}
		if got := IsBlockedIP(ip); got != want {
			t.Errorf("IsBlockedIP(%q) = %v, want %v", in, got, want)
		}
	}

	if !IsBlockedIP(nil) {
		t.Error("IsBlockedIP(nil) = false, want true (fail closed)")
	}
}

// proxyClient returns an http.Client that routes every request through p.
func proxyClient(p *Proxy) *http.Client {
	u, _ := url.Parse(p.URL())
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(u)},
	}
}

func TestProxyBlocksLiteralPrivateIP(t *testing.T) {
	p, err := Start(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// A plain-HTTP request to a private IP literal must be refused by the proxy
	// before any connection is made to the target.
	resp, err := proxyClient(p).Get("http://169.254.169.254/latest/meta-data/")
	if err != nil {
		t.Fatalf("request errored instead of returning a proxy response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d (forbidden)", resp.StatusCode, http.StatusForbidden)
	}
}

func TestProxyForwardsAllowedHTTP(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello from origin")
	}))
	defer origin.Close()

	// The test origin lives on loopback, which the real policy blocks. Override
	// IsBlockedIP so we can exercise the forwarding path itself.
	p, err := Start(Options{IsBlockedIP: func(net.IP) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	resp, err := proxyClient(p).Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != "hello from origin" {
		t.Errorf("body = %q, want %q", body, "hello from origin")
	}
}

func TestProxyBlocksResolvedLoopbackHost(t *testing.T) {
	p, err := Start(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// "localhost" resolves (via /etc/hosts, no network) to a loopback address;
	// the proxy must refuse after resolution, before connecting. This exercises
	// the resolve-then-vet path, not just the IP-literal path.
	resp, err := proxyClient(p).Get("http://localhost/")
	if err != nil {
		t.Fatalf("request errored instead of returning a proxy response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d (forbidden)", resp.StatusCode, http.StatusForbidden)
	}
}
