package pdfprinter2

import "testing"

func TestDefaultAllowRequestURL(t *testing.T) {
	cases := map[string]bool{
		// SSRF targets — must be blocked
		"http://169.254.169.254/latest/meta-data/": false, // cloud metadata (link-local)
		"http://127.0.0.1/":                        false, // loopback
		"http://localhost/":                        false, // resolves to loopback
		"http://10.0.0.5/internal":                 false, // RFC1918
		"http://192.168.1.1/":                      false, // RFC1918
		"http://172.16.0.1/":                       false, // RFC1918
		"http://100.64.0.1/":                       false, // carrier-grade NAT
		"http://[::1]/":                            false, // IPv6 loopback
		"http://[fd00::1]/":                        false, // IPv6 ULA
		"http://[fe80::1]/":                        false, // IPv6 link-local
		"http://0.0.0.0/":                          false, // unspecified

		// Non-http network / unknown schemes — blocked
		"file:///etc/passwd": false,
		"ftp://internal/x":   false,
		"ws://10.0.0.1/":     false,

		// Public internet — allowed (remote images/fonts still work)
		"http://93.184.216.34/img.png": true,
		"https://93.184.216.34/x.css":  true,

		// Inlined / in-memory — allowed
		"data:image/png;base64,iVBORw0KGgo=": true,
		"about:blank":                        true,
		"blob:https://x/abc":                 true,

		// Unparseable — fail closed
		"://nonsense": false,
	}

	for in, want := range cases {
		if got := defaultAllowRequestURL(in); got != want {
			t.Errorf("defaultAllowRequestURL(%q) = %v, want %v", in, got, want)
		}
	}
}
