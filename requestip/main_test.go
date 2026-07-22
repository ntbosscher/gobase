package requestip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// run builds a request with the given RemoteAddr + headers, runs it through the
// middleware configured with opts, and returns the resolved IP(ctx).
func run(remoteAddr string, headers map[string]string, opt Option, opts ...Option) string {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	var got string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IP(r.Context())
	})

	Middleware(opt, opts...)(next).ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestNoProxiesIgnoresSpoofableHeaders(t *testing.T) {
	got := run("203.0.113.7:5555", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
		"X-Real-IP":       "5.6.7.8",
	}, NoProxies())

	if got != "203.0.113.7" {
		t.Fatalf("NoProxies must trust RemoteAddr only, got %q", got)
	}
}

func TestTrustedProxyResolvesClient(t *testing.T) {
	// direct peer is our LB (10.0.0.1); it appended the real client to XFF
	got := run("10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "203.0.113.9",
	}, TrustProxies("10.0.0.0/8"))

	if got != "203.0.113.9" {
		t.Fatalf("expected client behind trusted proxy, got %q", got)
	}
}

func TestSpoofedPrefixIgnoredBehindTrustedProxy(t *testing.T) {
	// attacker sent a fake XFF; our trusted LB appended their real IP (198.51.100.5)
	got := run("10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "1.2.3.4, 198.51.100.5",
	}, TrustProxies("10.0.0.0/8"))

	if got != "198.51.100.5" {
		t.Fatalf("expected the real (rightmost non-trusted) IP, got %q", got)
	}
}

func TestUntrustedPeerHeadersIgnored(t *testing.T) {
	// client connected directly (not via a trusted proxy) but sent headers
	got := run("203.0.113.7:5555", map[string]string{
		"X-Forwarded-For": "1.2.3.4",
	}, TrustProxies("10.0.0.0/8"))

	if got != "203.0.113.7" {
		t.Fatalf("headers from an untrusted peer must be ignored, got %q", got)
	}
}

func TestChainOfTrustedProxies(t *testing.T) {
	// client -> proxy1(10.0.0.2) -> proxy2(10.0.0.1) -> app; both proxies trusted
	got := run("10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "203.0.113.9, 10.0.0.2",
	}, TrustProxies("10.0.0.0/8"))

	if got != "203.0.113.9" {
		t.Fatalf("expected leftmost client through a chain of trusted proxies, got %q", got)
	}
}

func TestTrustProxyHops(t *testing.T) {
	// one hop in front: take the entry one from the right of (XFF..., RemoteAddr)
	got := run("10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "1.2.3.4, 203.0.113.9",
	}, TrustProxyHops(1))

	if got != "203.0.113.9" {
		t.Fatalf("expected the IP one hop from the peer, got %q", got)
	}
}

func TestTrustProxyHopsBeyondChain(t *testing.T) {
	// more hops claimed than exist -> clamp to the leftmost entry
	got := run("10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "203.0.113.9",
	}, TrustProxyHops(5))

	if got != "203.0.113.9" {
		t.Fatalf("expected clamp to leftmost, got %q", got)
	}
}

func TestGarbageHeaderDropped(t *testing.T) {
	got := run("10.0.0.1:443", map[string]string{
		"X-Forwarded-For": "not-an-ip",
	}, TrustProxies("10.0.0.0/8"))

	if got != "10.0.0.1" {
		t.Fatalf("garbage XFF must be dropped, falling back to peer, got %q", got)
	}
}

func TestXRealIPFallbackBehindTrustedProxy(t *testing.T) {
	got := run("10.0.0.1:443", map[string]string{
		"X-Real-IP": "203.0.113.9",
	}, TrustProxies("10.0.0.0/8"))

	if got != "203.0.113.9" {
		t.Fatalf("expected X-Real-IP used behind trusted proxy, got %q", got)
	}
}

func TestIPv6RemoteAddr(t *testing.T) {
	got := run("[2001:db8::1]:5555", nil, NoProxies())

	if got != "2001:db8::1" {
		t.Fatalf("expected IPv6 host without port, got %q", got)
	}
}

func TestInvalidTrustedProxyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid trusted proxy CIDR")
		}
	}()

	TrustProxies("not-a-cidr")(&config{})
}
