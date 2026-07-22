package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAllowOrigin(t *testing.T) {
	cases := []struct {
		name    string
		allow   []string
		origin  string
		allowed bool
	}{
		// exact matching
		{"exact match", []string{"https://example.com"}, "https://example.com", true},
		{"exact match with port", []string{"http://localhost:3000"}, "http://localhost:3000", true},
		{"scheme mismatch", []string{"https://example.com"}, "http://example.com", false},
		{"port mismatch", []string{"http://localhost:3000"}, "http://localhost:4000", false},

		// the prefix-bypass class this fix closes
		{"suffix-appended host", []string{"https://example.com"}, "https://example.com.evil.com", false},
		{"prefix-extended host", []string{"https://example.com"}, "https://example.comattacker.net", false},
		{"different host entirely", []string{"https://example.com"}, "https://evil.com", false},

		// subdomain wildcard
		{"wildcard subdomain", []string{"https://*.example.com"}, "https://app.example.com", true},
		{"wildcard deep subdomain", []string{"https://*.example.com"}, "https://a.b.example.com", true},
		{"wildcard rejects apex", []string{"https://*.example.com"}, "https://example.com", false},
		{"wildcard rejects lookalike", []string{"https://*.example.com"}, "https://app.example.com.evil.com", false},
		{"wildcard rejects sibling", []string{"https://*.example.com"}, "https://evilexample.com", false},
		{"wildcard scheme enforced", []string{"https://*.example.com"}, "http://app.example.com", false},
		{"schemeless wildcard any scheme", []string{"*.example.com"}, "http://app.example.com", true},

		// junk / fail-closed
		{"empty origin", []string{"https://example.com"}, "", false},
		{"bare hostname entry never matches", []string{"example.com"}, "https://example.com", false},
		{"no allowlist", nil, "https://example.com", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mw := Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), WrapOpts{AllowOrigins: c.allow}).(*corsMiddleware)
			if got := mw.allowOrigin(c.origin); got != c.allowed {
				t.Fatalf("allowOrigin(%q) with allowlist %v = %v, want %v", c.origin, c.allow, got, c.allowed)
			}
		})
	}
}

func TestServeHTTPHeaders(t *testing.T) {
	mw := Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), WrapOpts{
		AllowOrigins: []string{"https://example.com"},
	})

	// Allowed origin gets ACAO + credentials + Vary.
	req := httptest.NewRequest("OPTIONS", "https://api.local/x", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("ACAO = %q, want the allowed origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("ACAC = %q, want true", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}

	// Disallowed origin must not receive ACAO, but Vary is still set.
	req2 := httptest.NewRequest("GET", "https://api.local/x", nil)
	req2.Header.Set("Origin", "https://example.com.evil.com")
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req2)

	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO = %q for disallowed origin, want empty", got)
	}
	if got := rec2.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin even when origin is rejected", got)
	}
}

// Credentials header is omitted entirely when credentials are disabled.
func TestCredentialsDisabled(t *testing.T) {
	mw := Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), WrapOpts{
		AllowOrigins:     []string{"https://example.com"},
		AllowCredentials: "false",
	})

	req := httptest.NewRequest("GET", "https://api.local/x", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if _, ok := rec.Header()["Access-Control-Allow-Credentials"]; ok {
		t.Fatalf("Access-Control-Allow-Credentials should be absent when disabled")
	}
}
