package cors

import (
	"net/http"
	"net/url"
	"strings"
)

type corsMiddleware struct {
	allowedOrigins   []originRule
	next             http.Handler
	allowHeaders     []string
	allowCredentials string
	allowMethods     string
}

// originRule is a precompiled entry from WrapOpts.AllowOrigins.
type originRule struct {
	// exact is the full, lowercased origin (scheme://host[:port]) for a
	// non-wildcard entry. Empty when the rule is a wildcard.
	exact string

	// wildcardSuffix is set for "*.example.com" style entries. It holds the
	// host suffix beginning with a dot (".example.com") so that only proper
	// subdomains match — never the apex ("example.com") and never a
	// look-alike like "example.com.evil.com".
	wildcardSuffix string

	// wildcardScheme constrains a wildcard rule to a single scheme (e.g.
	// "https"). Empty means any scheme is accepted.
	wildcardScheme string
}

// matches reports whether an incoming origin (already split into scheme, bare
// host without port, and the normalized scheme://host[:port] form) satisfies
// this rule.
func (r originRule) matches(scheme, host, normalized string) bool {
	if r.exact != "" {
		return r.exact == normalized
	}

	if r.wildcardScheme != "" && r.wildcardScheme != scheme {
		return false
	}

	// HasSuffix on ".example.com" matches "foo.example.com" but not
	// "example.com", "evilexample.com", or "example.com.evil.com".
	return strings.HasSuffix(host, r.wildcardSuffix)
}

func (c *corsMiddleware) allowOrigin(origin string) bool {
	if origin == "" {
		return false
	}

	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	scheme := u.Scheme
	host := u.Hostname()                  // host without port, for wildcard matching
	normalized := scheme + "://" + u.Host // preserves port, for exact matching

	for _, r := range c.allowedOrigins {
		if r.matches(scheme, host, normalized) {
			return true
		}
	}

	return false
}

func (c *corsMiddleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	origin := strings.ToLower(request.Header.Get("Origin"))

	// The response's Access-Control-Allow-Origin depends on the request's
	// Origin, so caches must key on it. Without this a shared/CDN cache could
	// serve a response allowing one origin to a request from another.
	writer.Header().Add("Vary", "Origin")

	if c.allowOrigin(origin) {
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		writer.Header().Set("Access-Control-Expose-Headers", "*")
		writer.Header().Set("Access-Control-Allow-Methods", c.allowMethods)
		writer.Header().Set("Access-Control-Allow-Headers", strings.Join(c.allowHeaders, ", "))

		// Only emit the credentials header when it's actually enabled. Sending
		// it together with a reflected origin is exactly the combination that
		// grants credentialed cross-origin reads, so it must never be paired
		// with a loose origin match.
		if c.allowCredentials == "true" {
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if request.Method == "OPTIONS" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
	}

	c.next.ServeHTTP(writer, request)
}

type WrapOpts struct {
	// AllowOrigins is the list of permitted request origins. Each entry must be
	// one of:
	//   - a full origin, matched exactly (case-insensitive), e.g.
	//     "https://app.example.com" or "http://localhost:3000"
	//   - a subdomain wildcard, e.g. "https://*.example.com" or "*.example.com",
	//     which matches any proper subdomain of example.com (but not the apex).
	//
	// Matching is exact/boundary-based — a bare prefix like "https://example.com"
	// will NOT match "https://example.com.evil.com". Bare hostnames without a
	// scheme (e.g. "example.com") never match a real browser origin and are
	// effectively ignored; use a full origin or a "*." wildcard instead.
	AllowOrigins []string

	// AllowHeaders response value
	// default: Authorization, Accept, Origin, X-Apiversion, X-Browserwindowid, X-Timezonename, X-Timezoneoffsetmins, Cookie, Content-Type, Cache-Control
	AllowHeaders []string

	// AllowCredentials response value
	// default: true
	AllowCredentials string

	// AllowMethods response value
	// default: *
	AllowMethods string
}

func Wrap(input http.Handler, opts WrapOpts) http.Handler {

	if opts.AllowMethods == "" {
		opts.AllowMethods = "*"
	}

	if opts.AllowCredentials == "" {
		opts.AllowCredentials = "true"
	}

	if opts.AllowHeaders == nil {
		opts.AllowHeaders = []string{
			"Authorization",
			"Accept",
			"Origin",
			"X-Apiversion",
			"X-Browserwindowid",
			"X-Timezonename",
			"X-Timezoneoffsetmins",
			"Cookie",
			"Content-Type",
			"Cache-Control",
		}
	}

	var rules []originRule
	for _, o := range opts.AllowOrigins {
		if r := compileOrigin(o); r.exact != "" || r.wildcardSuffix != "" {
			rules = append(rules, r)
		}
	}

	return &corsMiddleware{
		allowedOrigins:   rules,
		allowHeaders:     opts.AllowHeaders,
		allowCredentials: strings.ToLower(opts.AllowCredentials),
		allowMethods:     opts.AllowMethods,
		next:             input,
	}
}

// compileOrigin converts an AllowOrigins entry into a precompiled originRule.
// Entries that can't be interpreted as a full origin or a "*." wildcard yield
// an empty rule (fail closed — they match nothing) and are dropped by Wrap.
func compileOrigin(entry string) originRule {
	entry = strings.ToLower(strings.TrimSpace(entry))
	if entry == "" {
		return originRule{}
	}

	// Wildcard form: "*.example.com" or "https://*.example.com".
	if strings.Contains(entry, "*") {
		scheme := ""
		host := entry
		if idx := strings.Index(host, "://"); idx >= 0 {
			scheme = host[:idx]
			host = host[idx+3:]
		}

		// Trim any trailing path/port noise; we only match on the host suffix.
		if idx := strings.IndexAny(host, "/:"); idx >= 0 {
			host = host[:idx]
		}

		// Normalize "*.example.com" (and a bare "*example.com") to the suffix
		// ".example.com".
		suffix := strings.TrimPrefix(host, "*")
		if !strings.HasPrefix(suffix, ".") {
			suffix = "." + strings.TrimPrefix(suffix, ".")
		}
		if suffix == "." { // guard against a lone "*"
			return originRule{}
		}

		return originRule{wildcardSuffix: suffix, wildcardScheme: scheme}
	}

	// Exact form: normalize to scheme://host[:port].
	if u, err := url.Parse(entry); err == nil && u.Scheme != "" && u.Host != "" {
		return originRule{exact: u.Scheme + "://" + u.Host}
	}

	// Unrecognized entry (e.g. a bare hostname) — fail closed.
	return originRule{}
}
