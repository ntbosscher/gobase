# Security Scan — `gobase`

**Date:** 2026-07-22
**Scope:** Static security review of every top-level package in `github.com/ntbosscher/gobase`, one agent per package, plus a dependency/`govulncheck` audit of `go.mod`/`go.sum`.
**Method:** Manual source review per package. This is a shared framework library, so many findings are *latent* — safe as the library is used internally today, but dangerous if untrusted input ever reaches the sink. Severity reflects worst-case exploitability for a consumer of the library.

> **Caveat:** These are findings from an automated fan-out review and have **not been independently re-verified or exploited**. Treat the High/Critical items as leads to confirm, not confirmed incidents. Line numbers were accurate at scan time.

---

## Executive summary

| # | Severity | Package | Issue |
|---|----------|---------|-------|
| 1 | ~~**Critical**~~ ✅ **Resolved** | cors | Prefix-based origin matching (`strings.HasPrefix`) + credentials-true-by-default → full CORS bypass |
| 2 | ~~High~~ ✅ **Resolved** | integrations | Timing-unsafe HMAC (`!=`) on GitHub webhook signature → forgery → deploy RCE path |
| 3 | ~~High~~ ✅ **Resolved** | auth | JWT signing key written `0777` (world-readable secret → token forgery) |
| 4 | **High** | auth | Access-token cookie not `HttpOnly`; access token replayable at refresh endpoint indefinitely |
| 5 | **High** | requestip | Blindly trusts spoofable `X-Forwarded-For`/`X-Real-IP` → rate-limit / allowlist / audit bypass |
| 6 | ~~High~~ ⊘ **Dismissed** | randomish | Predictable `math/rand` generator — package name signals non-secret use; won't fix |
| 7 | **High** | res | Stack traces + raw error messages returned to clients on every panic (framework-wide) |
| 8 | ~~High~~ → **Low** (SSRF ✅, DoS open) | pdfprinter | ~~LFI~~ + ~~`--no-sandbox`~~ (delegated to pdfprinter2) + ~~SSRF~~ (egress guard) fixed; remaining: render-timeout/concurrency DoS + lifecycle bugs |
| 9 | ~~High~~ ✅ **Resolved** | currency | Negative amounts mis-parsed from untrusted JSON (wrong magnitude; sign lost for -1<x<0) |
| 10 | ~~High~~ ✅ **Resolved** | paginate | Negative `pageSize` bypasses `MaxPageSize` via `uint64` wrap → unbounded query DoS |
| 11 | ~~High~~ ✅ **Resolved** | worker | Send on closed channel after `Stop()` → unrecovered panic crashes process |
| 12 | **High** (partial ✅) | deps | ~~`html/template` XSS (GO-2026-4980/4982) — fixed via toolchain~~; `pgx/v4` SQL-injection (GO-2026-5004) still open |
| — | Medium/Low | many | See per-package detail below (log-file perms, error/stack disclosure, DoS, header injection, etc.) |

**Cross-cutting themes worth a framework-level fix:**
- **Error/stack-trace disclosure** — `er.HandleErrors` packages `debug.Stack()` + raw error text into a struct that `res`, `bgtaskutil`, `pqworkqueue`, and `parallelize` all serialize toward callers/clients. Fixing this once in `er`/`res` closes several findings at once (#7 and the Medium disclosure items).
- **World-writable files (`os.ModePerm` = 0777)** — recurs in `auth` (the JWT key!), `pkglog`, `pdfprinter`, `imgutil`. Use `0600`/`0700`.
- **Spoofable/insecure defaults** — public-route-by-default (`res`, `sample`), credentials-on-by-default (`cors`), trust-the-header (`requestip`), `UNIT_TEST` env bypass (`env`).
- **Missing size caps → DoS** — unbounded JSON body & multipart (`res`), no per-file/total cap (`remotezip`), unbounded goroutines (`parallelize`).

---

## Critical

### 1. cors — Origin bypass via `strings.HasPrefix` + credentials on by default — ✅ RESOLVED (2026-07-22)
`cors/main.go:20-22, 25-29, 62-64`

> **Status: Fixed.** `cors/main.go` now uses exact, boundary-safe origin matching (precompiled `originRule`s) with explicit `*.`-subdomain wildcard support; unparseable/bare-hostname entries fail closed. `Vary: Origin` is set unconditionally (closes the related cache-poisoning gap) and `Access-Control-Allow-Credentials` is emitted only when credentials are enabled. Regression test added in `cors/main_test.go` asserting the bypass origins (`example.com.evil.com`, `example.comattacker.net`, wildcard apex/look-alikes) are rejected. **Follow-up left open by design:** `AllowCredentials` still defaults to `"true"` (flipping it to opt-in is a breaking change deferred to a called-out release). Details of the original finding below for the record.

Origins are validated with a prefix match and the matched origin is reflected into `Access-Control-Allow-Origin` together with `Access-Control-Allow-Credentials: true` (which defaults to `"true"`).

```go
match := jv.First(c.allowedOrigins, func(input string) bool {
    return strings.HasPrefix(origin, input)   // "https://example.com.evil.com" matches "https://example.com"
})
...
writer.Header().Set("Access-Control-Allow-Origin", origin)
writer.Header().Set("Access-Control-Allow-Credentials", c.allowCredentials) // defaults "true"
```

An allowlist entry `https://example.com` also matches attacker origin `https://example.com.evil.com`. Combined with reflected origin + credentials, an attacker page gains credentialed cross-origin read of authenticated responses — account/data compromise.

**Fix:** exact, lowercased origin match (or parsed-host suffix match for subdomains); default `AllowCredentials` to `false` (opt-in); add `Vary: Origin` unconditionally; replace the `*` defaults for methods/expose-headers with explicit lists.

---

## High

### 2. integrations — Timing-unsafe webhook HMAC comparison → deploy RCE — ✅ RESOLVED (2026-07-22)
`integrations/github/githubcd/githubcdutil/sig.go:48`

> **Status: Fixed.** `Verify` now hex-decodes the client signature and compares with `hmac.Equal` over raw HMAC bytes (constant-time; also safe on length mismatch), via a new `hmacSum` helper. The remaining `integrations` items (cleartext forwarding, hardcoded test secret, nil-deref, S3 filename) are still open. Original finding below.

```go
value, err := calcSignature(secret, body)
if value != sig {                     // non-constant-time string compare
    return nil, errors.New("invalid hash")
}
```

`!=` leaks timing, allowing byte-by-byte signature recovery. This `Verify` guards `githubcd/main.go` (runs `git pull -f` + an arbitrary `postPullScript`) and `exec/distributor` — a forged signature reaches a code-execution/deploy path.

**Fix:** `hmac.Equal` on raw HMAC bytes; hex-decode and length-check `sig` first.

Also in `integrations`:
- **Low** — hardcoded secret literal `QyzKtDRXrEH3` committed in `sig_test.go:6` with `t.Error(str)` printing it; rotate if ever real.
- **Medium** — signed webhook payload forwarded over cleartext `http://` in `exec/distributor/main.go:75,100`; captured HMAC header is replayable (no timestamp/nonce). Use HTTPS + replay protection.
- **Low** — nil-deref on `resp.StatusCode` when `Do` errors, `distributor/main.go:84-90`.
- **Low** — unsanitized filename in `Content-Disposition` in `s3fs/main.go:117,239` (only strips commas).

### 3. auth — JWT signing key written world-readable (`0777`) — ✅ RESOLVED (2026-07-22)
`auth/httpauth/jwtgen/main.go:25`

> **Status: Fixed.** `.jwtkey` is now written `0o600` (via `os.WriteFile`) with a follow-up `os.Chmod` to tighten permissions even if the file already existed with looser modes. The other `auth` items (#4 access-cookie `HttpOnly`/token-typing, alg pinning, CSRF/`SameSite`, etc.) remain open. Original finding below.

```go
ioutil.WriteFile(file, buf, os.ModePerm)   // 0777 — the HMAC secret signing all tokens
```

Any local user/process can read `./.jwtkey` and forge tokens for any user/role, or overwrite it. **Fix:** `os.OpenFile(..., O_CREATE|O_EXCL|O_WRONLY, 0o600)`.

### 4. auth — Access-token cookie not `HttpOnly`; token replay at refresh
`auth/httpauth/main.go:428-436` (access cookie, no `HttpOnly`) vs `:440` (refresh cookie sets it); `:373-381` refresh handler.

Access-token JWT is readable from `document.cookie` (XSS-exfiltratable). Because `createAccessToken`/`createRefreshToken` produce structurally identical tokens (same key/claims, see below) and `refreshHandler` accepts any valid JWT, a stolen short-lived access token can be POSTed to `/api/auth/refresh` to mint fresh tokens indefinitely.

**Fix:** set `HttpOnly` on the access cookie; add a `"typ":"access|refresh"` claim and enforce it at each endpoint.

Other `auth` findings:
- **Medium** — JWT parse doesn't pin the algorithm (`main.go:318-333`); add `jwt.WithValidMethods([]string{"HS256"})` (defense-in-depth; not a live bypass given symmetric key).
- **Medium** — refresh & access tokens cryptographically indistinguishable (`main.go:496-527`).
- **Medium** — cookie auth with `SameSite` unset by default and no CSRF token (`main.go:42-44`); default to `Lax`/`Strict`.
- **Low** — modulo bias in OAuth session-secret generation (`oauthgen/main.go:22-24`).
- **Low** — `PublicRoutePrefixes` uses raw `HasPrefix` → `/api/public` also whitelists `/api/public-admin` (`main.go:231-236`).
- **Low** — raw JWT/parse error strings returned to clients (`main.go:289,354,364`).
- *Good:* token/secret RNG uses `crypto/rand` correctly; no hardcoded secrets.

### 5. requestip — Trusts spoofable forwarding headers
`requestip/main.go:28-41`

```go
sources := []string{ r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-IP"), r.RemoteAddr }
// first non-empty wins; RemoteAddr only used if both headers empty
```

Any client sets `X-Forwarded-For: 1.2.3.4` and controls `IP(ctx)`. Defeats anything built on it: rate limiting (rotate XFF for unlimited quota), IP allowlists, audit logging. Also the multi-hop XFF list is never parsed (`net.SplitHostPort` fails on the list, leaving the raw string). **Fix:** only honor forwarding headers when `RemoteAddr` is a configured trusted proxy; parse XFF right-to-left, discarding trusted hops; validate with `net.ParseIP`.

### 6. randomish — Predictable RNG with a secret-looking API — ⊘ DISMISSED (won't fix)
`randomish/main.go:12,15-29`

> **Status: Dismissed by maintainer.** The package name `randomish` is itself the signal that its output is not cryptographically secure; using it for secrets/tokens is a caller error, not a library defect. In-repo callers are all benign (retry jitter, log IDs, random array pick). No change planned. Original analysis retained below for reference.

```go
var random = rand.New(rand.NewSource(time.Now().UnixNano()))   // math/rand
func GetAlphaNumericChars(length int) string { ... }
```

`math/rand` output is fully predictable from the seed. The convenient name invites use for tokens/IDs. In-repo callers are currently benign (retry jitter, log IDs, random array pick), so this is **latent** — but the API is a token-generation footgun. **Fix:** add a clearly-named `crypto/rand` variant and change the package/function docs from "random enough" to a hard "do NOT use for secrets" warning. Also: `Int` bypasses the mutex/uses the global RNG and panics on `max<=min` (`main.go:31-33`).

### 7. res — Stack traces & internal errors returned to clients
`res/router.go:137` (and `:152`), serialized via `errorData(...)` → `responder.go:356`

```go
defer er.HandleErrors(func(input *er.HandlerInput) {
    res := &responder{ status: input.SuggestedHttpCode,
        data: errorData(input.Message, input.StackTrace, "", input.Details) }
    res.Respond(wr, req)
})
```

`input.StackTrace` = `debug.Stack()` and `input.Message` = raw underlying error (e.g. SQL text, file paths). Every panic routed through the standard `er.Check` idiom returns these in the HTTP 500 body to any caller. **Fix:** log stack/message server-side; return a generic message; gate detail behind a dev-only flag. (This is the `res` half of the cross-cutting `er` disclosure theme.)

Other `res` findings:
- **Medium** — unbounded request body in `ParseJSON` (`router.go:301-308`); wrap with `http.MaxBytesReader`.
- **Medium** — multipart total size unbounded; `ParseMultipartForm` arg is only the in-memory threshold, overflow spills to disk (`router.go:168,225-235`).
- **Medium** — `Display` serves inline with sniffed content-type and no `X-Content-Type-Options: nosniff` (`responder.go:197-208`) → stored XSS from uploaded "images". Add `nosniff`.
- **Medium** — Content-Disposition filename param injection; `SanitizeDispositionName` only strips commas (`responder.go:181,193-195`). RFC 6266-encode.
- **Low** — open-redirect helper `Redirect` passes raw input to `http.Redirect` (`responder.go:283-289`).
- **Low** — source-map token compared with `==` (non-constant-time) and allows access when token is empty (`react.go:131-148`).
- **Low/Info** — routes public by default (`r/main.go:59-95`); WS upgrader origin check not configurable; `rqutil` interpolates `table`/`orderCol` identifiers (developer-controlled today).

### 8. pdfprinter — LFI, SSRF, and disabled sandbox on untrusted HTML — 🟡 PARTIALLY RESOLVED (2026-07-22)
`pdfprinter/main.go:53,74` and `pdfprinter2/playwright.go:242`

> **Status: LFI + sandbox fixed; SSRF still open.** `pdfprinter.Print` no longer shells out to Chrome — it now delegates to `pdfprinter2.Print`, which renders via Playwright using `SetContent` (an `about:blank` origin, so no `file://` subresource loads → **LFI resolved**) and does not pass `--no-sandbox` (**sandbox-escape amplification resolved**). The old `--no-sandbox` + `file://` code path and its `0777` temp files are gone.
> **⚠️ SSRF remains** for both `pdfprinter` and `pdfprinter2`: neither restricts the browser's network egress, so untrusted HTML with `<img src="http://169.254.169.254/...">` / `<iframe src="http://internal/...">` can still reach cloud metadata / internal services. Needs egress restriction (network namespace or a deny-RFC1918/link-local/loopback proxy, or Playwright `route`/`Route.Abort` of non-`data:` requests). The `sample.go:84` public print endpoint should still be put behind auth. Original finding below.
>
> **Re-analysis of `pdfprinter2` (2026-07-22)** — a fresh full read of `pdfprinter2/main.go` + `playwright.go` surfaced more than the carried-over SSRF. Open items in that package:
> - **SSRF (High) — `playwright.go:242`** (`pg.SetContent`). Untrusted HTML renders in Chromium with full network access; *and page JavaScript executes*, so it's dynamic SSRF (`<script>fetch('http://169.254.169.254/...')...</script>`) that can read internal responses and paint them into the DOM before the PDF snapshot → reflected exfiltration, not just blind. **✅ MITIGATED (2026-07-22):** `applyNetworkGuard` now installs a `BrowserContext.Route("**/*")` interceptor (before any page loads) that aborts requests to loopback/private/link-local/CGN/unspecified hosts (blocks `169.254.169.254`, RFC1918, `::1`, ULA, etc.); allows `data:`/`blob:`/`about:` and public http(s); blocks `file:`/`ftp:`/`ws:`/unknown schemes; fails closed on parse/DNS errors. Overridable per-call via the new `PrintOptInput.AllowRequestURL` hook (e.g. `func(string) bool { return false }` for a full lockdown or a host allow-list). Policy unit-tested in `pdfprinter2/playwright_test.go`. **Residual:** best-effort against DNS-rebinding (the browser does its own resolution) — for high-assurance isolation still run the renderer in a network namespace with no internal route.
> - **No render timeout → hang/DoS (Medium) — `:242, :258-267`.** `SetContent` uses the default `waitUntil:"load"` (waits on subresources) with no `Timeout`; the `ctx.Done()` checks at `:220/:231/:248` are non-blocking peeks *between* steps only — `SetContent`/`EmulateMedia`/`PDF` get no ctx. A `<img src="http://attacker/slowloris">` stalls each render to the 30s default; the fixed `<-time.After(3*time.Second)` at `:263` wastes ≥3s every call. No cap on `input.HTML` size.
> - **Unbounded concurrency → resource exhaustion (Medium) — `:143-197, :210`.** One `BrowserContext`+`Page` per `Print` in a single shared Chromium, no concurrency cap (the `active` counter only drives auto-shutdown). Flooding the (public) endpoint spawns unlimited pages.
> - **30-min max-active shutdown kills in-flight renders (Medium) — `:88-101 → :106-141`.** `Print` renders without holding `p.mu`; `maxActiveShutdown` takes the mutex and `chrome.Close()`s the browser out from under any in-flight render.
> - **Startup race: `Print` before `Init` → nil-channel send hangs forever (Medium/Low) — `main.go:13-16` vs `playwright.go:144`.** `Init` runs in a goroutine off package `init()` and is what allocates `activeIncrementC`; a `Print` that wins the race sends on a nil channel and blocks permanently.
> - **Auto-shutdown drain stalls new renders (Low) — `:91-96`.** During `maxActiveShutdown` the loop services only `activeDecrementC`, so a concurrent `Print` blocked on `activeIncrementC` (`:144`) can't proceed.
> - **Hooks expose raw `Page` (Info) — `:204-207, :237, :254`.** `OnSetupContent`/`OnRender` allow `pg.Goto(userURL)` — an explicit SSRF sink if a caller passes untrusted URLs.
> - **Runtime browser download (Info) — `:118-129`** (`playwright.Install`) fetches Chromium at runtime; integrity depends on playwright-go pinning.
> - *Clean:* `file://` LFI mitigated by Chromium's default cross-scheme policy (no `--allow-file-access-from-files`) — relies on the default, not an explicit control here; sandbox on; no command injection.

- **High LFI:** HTML written to `input.html` and opened as a `file://` origin, so `<iframe src="file:///etc/passwd">` renders local files into the PDF (`main.go:31,53,74`).
- **High SSRF:** no network isolation; `<img src="http://169.254.169.254/...">` hits cloud metadata / internal services and can reflect into the PDF (`main.go:69-74`, `playwright.go:242`).
- **Medium:** `--no-sandbox` (`main.go:74`) means a renderer exploit escalates to host RCE. (`pdfprinter2` does not pass it.)
- **Low:** temp files/dirs created `0777` (`main.go:38,43,48,53`).
- *Clean:* no command injection / path traversal (fixed binary path, arg slice, fixed filenames under a random `TempDir`).

**Fix:** don't load untrusted HTML from `file://` (use `data:`/`SetContent`/DevTools); restrict browser egress (proxy denying RFC1918/link-local/loopback, or a network namespace); drop `--no-sandbox` (run non-root). Audit all callers to confirm HTML is built with `html/template`. Note `sample.go:84` exposes exactly this as a **public** endpoint (see Appendix).

### 9. currency — Negative amounts corrupted when parsed from untrusted JSON — ✅ RESOLVED (2026-07-22)
`currency/main.go:116-139`

> **Status: Fixed.** `Parse` now extracts a single leading sign and parses the magnitude as non-negative (so `-2.50` → -250 and `-0.50` → -50), rejects embedded signs / stray characters / empty parts via an `isAllDigits` guard, and does the arithmetic in `int64` with a pre-multiply bound + final range check so an over-large value errors instead of wrapping (fixes the related Medium overflow and Low sign-in-cents items too). Regression tests added in `currency/main_test.go`. **Still open (separate items):** fractional cents beyond 2 places are still *truncated* not rounded, and `Cents` remains a platform-dependent `int` (finding #5). Original finding below.

```go
dollars, _ := strconv.Atoi(parts[0])   // "-2" -> -2
result := Cents(dollars * 100)          // -200
cents, _ := strconv.Atoi(parts[1])      // "50" -> 50
result += Cents(cents)                  // -200 + 50 = -150   (should be -250)
```

`Parse("-2.50")` → -150; `Parse("-0.50")` → **+50** (sign lost entirely). Reachable from untrusted JSON via `CentsWithJsonEncoding.UnmarshalJSON`. **Fix:** extract sign once, parse absolute values, reapply sign.

Also: **Medium** integer overflow in `dollars*100` (`:121`) reachable from JSON; **Medium** silent truncation of fractional cents (`:124-127`); **Low** `Atoi` accepts a sign inside the cents field (`"2.-5"`); **Low** `Cents` is platform-dependent `int` (use `int64`).

### 10. paginate — Negative `pageSize` bypasses `MaxPageSize` — ✅ RESOLVED (2026-07-22)
`paginate/main.go:275-288`

> **Status: Fixed.** `applyPager` now clamps the low end first (`pageSize <= 0` → default, then cap at `MaxPageSize`) and clamps a negative `page` to 0, so neither can wrap `uint64` into a huge LIMIT/OFFSET. Original finding below.

```go
if pageSize > MaxPageSize { pageSize = MaxPageSize } // doesn't catch negatives
if pageSize == 0 { pageSize = DefaultPageSize }      // only catches exactly 0
return query.Limit(uint64(pageSize)).Offset(uint64(cfg.page * pageSize))
```

`pageSize` comes from `rq.GetQueryInt("pageSize")`. `?pageSize=-1` → `uint64(-1)` = a huge LIMIT, defeating the cap → the DB materializes the whole table (memory/bandwidth DoS). **Fix:** `if pageSize <= 0 { pageSize = DefaultPageSize }; if pageSize > MaxPageSize { pageSize = MaxPageSize }`. Same wrap issue for negative `page` (`:287`, Low). *Note:* all SQL injection vectors here (order-by column/direction, filter column/operator, search values) are properly whitelisted/parameterized — good.

### 11. worker — Send on closed channel panics the process — ✅ RESOLVED (2026-07-22)
`worker/main.go:94` (`Stop` closes `w.signal`) racing `:101/:110` (`Trigger`/`TriggerWithInput` send on it)

> **Status: Fixed.** `Stop()` now closes a dedicated `done` channel guarded by `sync.Once` (never the `signal` channel that `Trigger` sends on), so post-stop triggers and double-`Stop()` are safe no-ops. The loop and `TriggerWithInput` select on `done`, and the 10s error backoff is now interruptible by `Stop()` (closes the related Low shutdown-delay item). Regression tests added (`TestStopIsSafe`, `TestStopConcurrentWithTrigger`). **Still open:** unchecked type assertion in `Current()` (`worker` Low). Original finding below.

```go
func (w *Worker) Stop() { close(w.signal) }
func (w *Worker) Trigger() { select { case w.signal <- 0: default: } } // panics if closed
```

These run in the caller's goroutine (outside the `loop` recover), so the panic is unrecovered → whole-process crash. Also double-`Stop()` panics ("close of closed channel"). **Fix:** use a `done`/context to signal stop, guard with `sync.Once`, and have `Trigger*` select on `done`. Lower: unchecked type assertion in `Current()` (`:136`); fixed 10s blocking backoff on any error stalls the loop and delays shutdown (`:86-89`).

### 12. dependencies — Reachable vulnerabilities (`govulncheck`) — 🟡 PARTIALLY RESOLVED (2026-07-22)
`go.mod` / `go.sum`.

> **Status: stdlib findings fixed.** Added `toolchain go1.26.5` to `go.mod`; the build now uses go1.26.5 and `govulncheck` confirms **all 8 stdlib advisories are gone**, including both `html/template` XSS Highs (GO-2026-4980/4982) and the crypto/tls (5856), crypto/x509 (5037), net (4971), net/textproto (5039) items. **Still open:** the `x/net` (upgrade → v0.55.0) and `x/text` (→ v0.39.0) module findings, and the **High `pgx/v4` SQL-injection (GO-2026-5004)** which requires migrating `pqworkqueue` to `pgx/v5` (no v4 patch exists; also clears `pgproto3/v2` GO-2026-4518). Original audit below.

Original audit ran against toolchain go1.26.2. `govulncheck` reported **16 called vulnerabilities** across 4 modules + stdlib.

- **High** — `github.com/jackc/pgx/v4` v4.18.2 — GO-2026-5004 SQL injection (placeholder/dollar-quote confusion), reached from `pqworkqueue.Notify`. **No v4 fix → migrate to `pgx/v5`** (also clears the `pgproto3/v2` v2.3.3 DoS GO-2026-4518).
- **High** — stdlib `html/template` (go1.26.2) — GO-2026-4980 & GO-2026-4982 escaper-bypass/XSS, reached from `email.SendTemplate`. Fixed **go1.26.3**.
- **Medium** — `golang.org/x/net` v0.47.0 — multiple `x/net/html` XSS/DoS (GO-2026-5025/27/28/29/30), `idna` (GO-2026-5026), HTTP/2 loop (GO-2026-4918). Upgrade to **v0.55.0**.
- **Medium** — stdlib: GO-2026-5856 (crypto/tls ECH), GO-2026-5039 (net/textproto), GO-2026-5037 (crypto/x509 DoS), GO-2026-4971 (net NUL panic), GO-2026-4918 (net/http). Upgrade toolchain to **≥ go1.26.5**.
- **Low** — `golang.org/x/text` v0.37.0 — GO-2026-5970 infinite loop. Upgrade to **v0.39.0**.
- **Info** — `azidentity` v1.5.2 < 1.6.0 (CVE-2024-35255) present but not reachable; bump opportunistically.

**Remediation order (highest leverage first):** (1) ~~upgrade Go to ≥ go1.26.5 — clears all 8 stdlib findings incl. both `html/template` Highs~~ ✅ **done** (`toolchain go1.26.5` pinned in `go.mod`); (2) `go get golang.org/x/net@v0.55.0 golang.org/x/text@v0.39.0`; (3) migrate `pqworkqueue` to `pgx/v5`.

---

## Medium

- **env** — `env/main.go:82,91,114,128`: env values (possible secrets) interpolated into parse-failure logs; log the key only. — `main.go:16,61-77`: `UNIT_TEST` env var flips `fatal()` from abort to log-and-return, so `Require*` return zero values in prod if the var is set → app can start with empty/false security config. Prefer a build tag; keep `Require*` fatal.
- **bgtaskutil** — `main.go:190-202`: `JsonErrorResult` unconditionally serializes `StackTrace` + raw error into the response body (only the `fmt.Println` is gated by `IsTesting`). Gate the payload too. (Plus Low nil-deref panics at `:104,147` and `userinterrupt/main.go:57`; unbounded job-slice/goroutine leak if CancelFunc not called.)
- **lg** — `main.go:271`: raw (unescaped) interpolation of `parent` into hand-built JSON log rows via exported `OptWithParent` → log forging/JSON injection. — `main.go:107-112`: `OptWithRequest` logs full `req.URL.String()` incl. query string (tokens/PII) for every request. Marshal a struct instead of `fmt.Sprintf`; log path only.
- **pkglog** — `main.go:28`: log file opened `os.ModePerm` (0777) and `O_RDWR` without `O_APPEND` (world-readable logs, clobbering). Use `0600 | O_APPEND`.
- **pqchan** — `main.go:137`: channel name concatenated into `listen ...` (identifiers can't be bound). Gated today by `nameValidation`, but the regex `^[A-z\_0-9]+$` (`:54`) is the classic `[A-z]` bug (allows `` [ \ ] ^ _ ` ``). Use `pgx.Identifier{}.Sanitize()` and `^[A-Za-z0-9_]+$`. (Plus Low blocking-send-under-lock at `:162`.)
- **ratelimit** — `main.go:62-70`: `IsLimited()` put-back can block indefinitely (peek race). — `main.go:27-42`: `Stop()` only stops the ticker; the `start()` goroutine leaks forever (unbounded if limiters are per-key). Add a `done` channel + `sync.Once`. Note: per-process only, so a security-relevant limit multiplies across replicas.
- **remotezip** — `main.go:369,389`: zip entry `name` never sanitized → the writer *emits* zip-slip payloads (`../../etc/...`) for downstream extractors. — `main.go:299,320,375`: no per-file/total size cap; the only guard (`checkDiskSpace`) is **Linux-only** and checked every 10th item → disk-exhaustion DoS. Also Low data race on `z.err`. (Package is a zip *writer*; fetching/SSRF/TLS is delegated to caller closures — document that.)
- **pqworkqueue** — `main.go:398-419`: no retry cap / dead-letter → a job that hard-crashes the worker (OOM, fatal) is re-claimed oldest-first every restart, stalling the queue. Add `attempts` + dead-letter. — `main.go:299-364`: outer tx (and a `FOR UPDATE` lock + 2 pooled conns per job) held for the entire callback → pool exhaustion under backlog. Claim in a short tx + visibility timeout. — `main2.go:60-72`: stack trace + error persisted into `result` returned by `GetResult`. *Clean:* SQL fully parameterized; JSON payloads can't RCE; no double-execution under normal operation.

---

## Low / Informational

- **encoding/tsv** — `tsv/main.go:30-41`: CSV/formula injection — cells starting with `= + - @` pass through unquoted and execute when opened in a spreadsheet. Prefix a guard quote. (Package is a write-only encoder; no decoder/XXE/zip surface.)
- **email** — `main.go:54,65,74-79` (`template.HTML` / `SectionHTML`): unescaped-HTML sinks; safe only if callers never pass untrusted input. `main.go:258`: comma-join of `To` allows multi-recipient smuggling if one element contains a comma. No SMTP/CRLF, SSRF, TLS, or hardcoded-secret issues (API-based sending via HTTPS; `json.Marshal` neutralizes header injection).
- **er** — `main.go:37,48` raw error → client-facing `Message` for 500s; `:26,39,50` full stack trace in `StackTrace`; `:96-102` `HttpError.Error()`/`Unwrap()` nil-deref inside the recover handler → process crash. Separate client-safe fields from server-only fields; nil-guard. (Root of the cross-cutting disclosure theme.)
- **model** — SQL value path is sound (squirrel `$`-placeholders throughout; no concatenation). Latent identifier-injection surfaces if a caller ever passes user-controlled `table`/order-by: `modelutil/main.go:227,251,276`, `schedule.go:37,49`, `squtil/union.go:66-73`. Info: verbose logging writes queries + bound args (`main.go:156-165`); connection-string may surface in a fatal log (`main.go:39-43`); scheduled jobs use `context.Background()` with no timeout; `modelperf2` writes `/tmp/perf.txt` `0644`.
- **jsutil** — `main.go:12`: XSS-safety for JS embedding is inherited from `res`'s `jsoniter.ConfigDefault` (`EscapeHTML:true`) rather than guaranteed locally; U+2028/U+2029 not escaped. No callers today. Pin its own encoder.
- **random** — `main.go:19`: modulo bias (`256 % 62 = 8`) skews the first 8 charset chars ~25% higher; use rejection sampling. `Int` panics on non-positive range (`:30-39`). Entropy source is `crypto/rand` throughout — good.
- **imgutil** — no untrusted-decode/file/exec surface of its own; allocation is driven by caller-supplied `width`/`height` (`main.go:29,40`) — callers wiring these to request params must clamp them. Test-only `0777` write.
- **bgtaskutil / parallelize / worker (concurrency)** — **parallelize** `main.go:17-19`: unbounded goroutine fan-out (DoS if task count is attacker-influenced); `:33` embeds `debug.Stack()` into the returned error. Panics are correctly contained; no data race.
- **httpdefaults** — timeouts are correctly set (good Slowloris defense). Gaps: no request-body size cap, no default security headers, plaintext-only (no TLS variant). All "harden the defaults," relevance depends on edge vs behind-proxy.
- **pqshared** — `main.go:16-19`: raw connect error logged may echo DSN/credentials on a malformed connection string. No query-building surface.
- **nonce** — Not a generator; it's a used-nonce middleware. `main.go:20` empty `X-Nonce` skips the check (opt-out bypass); `:34-44` accepts any unseen string (unsigned, unbound → no real CSRF/authenticity); `:46-63` replay after the 10-min eviction window; in-memory per-process (replay on restart / across instances); unbounded map (memory DoS). Needs HMAC-signed, session-bound, shared-store nonces to be meaningful.
- **timedebug / timeutil** — no backdoor, no untrusted time/duration parsing. Low: `AddDays` loop only advances on `Contains==true` → potential unbounded loop / CPU DoS with a hostile calendar (`timeutil/main.go:26-31`). `ScheduleJob` division confirmed safe.
- **jv** — generic collection lib (not JSON validation). No deserialization/regex/reflection. Low: `Min`/`Max`/`Average` panic on empty variadic input (`main.go:38-60`, `arrayutil.go:595`).
- **strs** — single `Coalesce` helper. Nothing exploitable.
- **apiversion** — parses `X-APIVersion` header into ints; no SQL/exec/fs/net. Two Info hardening notes (unchecked type assertion `main.go:100`; unbounded header split `:32-41`, bounded in practice by `MaxHeaderBytes`).
- **demux** — channel fan-out utility, not HTTP routing. No web surface. Info: blocking send under lock can wedge all consumers in non-lossy mode.
- **httpversion** — empty package (`// renamed to apiversion`). Nothing to scan.

---

## Appendix: `sample/` demo code
`sample.go` / `sample/users/main.go` are mostly clean (parameterized SQL, tenant isolation, env-sourced secrets, no hardcoded creds). Patterns dangerous if copy-pasted:
- **Medium** — `sample.go:84-92`: `POST /api/print-my-html` is **public** (`rt.Add` with no role) and pipes raw multipart `html` into the PDF renderer → the SSRF/LFI of finding #8, unauthenticated.
- **Low** — `sample.go:109-113`: GitHub webhook runs `./rebuild-and-migrate.sh` (RCE by design; guarded by env HMAC — keep the secret mandatory and verification constant-time, see #2).
- **Low/Info** — auth stubs return `errors.New("todo")` (fail-closed, good) but `RegisterHandler` returns `nil, nil` (ambiguous footgun); prefer `panic("not implemented")`.
- **Info** — routes are public unless a role is specified — the root cause of the exposed print endpoint. Add explicit comments / prefer fail-closed defaults.

---

## Recommended next steps
1. **Fix the Critical/High leads first**, verifying each: ~~cors exact-match (#1)~~ ✅, ~~webhook `hmac.Equal` (#2)~~ ✅, ~~JWT key `0600` (#3)~~ ✅, access-cookie `HttpOnly` + token typing (#4), requestip trusted-proxy model (#5).
2. **One framework fix, several findings:** stop serializing `StackTrace`/raw error toward clients in `er` + `res` (closes #7 and the Medium disclosure items in `bgtaskutil`, `pqworkqueue`, `parallelize`).
3. **Dependencies:** upgrade Go to ≥ go1.26.5, bump `x/net`→v0.55.0 and `x/text`→v0.39.0, migrate `pqworkqueue` to `pgx/v5`.
4. **Defaults & caps:** default `cors` credentials off; add `http.MaxBytesReader` for JSON/multipart in `res`; clamp `paginate` page size low-end; `0600`/`0700` for `auth`/`pkglog`/`pdfprinter` file writes.
5. **`requestip`/`nonce`/`ratelimit`** should not be relied on as security controls in their current form — document or redesign (trusted-proxy parsing; signed+shared-store nonces; distributed limiter).
