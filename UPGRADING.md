# Upgrading gobase

Security-hardening changes between `2cb8487` (2026-06-24) and `HEAD` (2026-07-23).

This is a guide for **projects that import `github.com/ntbosscher/gobase`**. It tells you what
to fix when you bump the version. Every entry is tagged:

- **`[compile]`** — your code will not build until you change the call site. Loud, self-announcing.
- **`[behavior]`** — same signature, different runtime behavior. Compiles fine, acts differently.
  These are the dangerous ones — read them.

Regenerate the raw diff for any package with:

```bash
git diff 2cb84875666997b4683acc1cef3b441b2fbe8ae3..HEAD -- <path>
```

---

## TL;DR — do these before you ship

**Won't compile until fixed:**

1. **`requestip.Middleware()` now requires a trust policy argument.** Pass `requestip.NoProxies()`,
   `requestip.TrustProxies(cidrs...)`, or `requestip.TrustProxyHops(n)`. See [requestip](#requestip).
2. **`pqshared.Pool` is now `*pgx/v5/pgxpool.Pool`** (was v4). Update imports anywhere you touch it,
   and run `go mod tidy`. Migrate your own `pgx/v4` usage to `v5`. See [pqshared / model](#pqshared).

**Compiles, but will bite silently — verify each:**

3. **All existing auth sessions are invalidated on deploy.** Tokens are now type-bound (`typ` claim);
   pre-upgrade tokens have no `typ` and are rejected. Every user must re-login. See [auth/httpauth](#authhttpauth).
4. **CORS will start rejecting origins** that were bare hostnames or prefixes. Convert
   `AllowOrigins` entries to full origins or `*.`-wildcards. See [cors](#cors).
5. **`UNIT_TEST=true` no longer forces test mode.** Non-test binaries now boot strict — a missing
   `.env` or required var is now `log.Fatal`. See [env](#env).
6. **Deploy webhooks (githubcd) now default to `https://`.** Ensure `SERVERS` hosts are reachable
   over TLS, or explicitly prefix `http://`. See [githubcd](#githubcd).
7. **Clients no longer receive raw error text / stack traces** from panics (5xx now returns a generic
   message + `correlationId`). Update any frontend/test that asserts on error bodies. See [error disclosure](#error-disclosure-er-res-bgtaskutil-parallelize-pqworkqueue).
8. **PDF rendering now blocks SSRF egress and enforces size/timeout/concurrency limits.** Templates
   loading internal subresources will fail; high-concurrency callers must queue. See [pdfprinter](#pdfprinter--pdfprinter2).
9. **New auth cookie defaults:** `SameSite=Lax` and access cookie is `HttpOnly`. Cross-site cookie
   flows and any JS reading the access token break. See [auth/httpauth](#authhttpauth).
10. **`currency.Parse` now parses negatives correctly** (`"-5.50"` → `-550`, was `-450`). Re-check any
    stored/derived values. See [currency](#currency).

---

## auth/httpauth

- **[behavior]** — **All previously-issued tokens are invalidated.** Tokens are now bound to a type
  (access vs refresh) via a `typ` claim; an access token presented to the refresh endpoint (or vice
  versa) is rejected, and the signing alg is pinned to HS256.
  - Why: blocks replaying a JS-readable access token at the refresh endpoint, plus alg-substitution.
  - Impact: tokens minted before this upgrade have no `typ` claim → they fail the type check.
  - Fix: no code change, but **every user must re-login on deploy**. Make sure your login flow handles
    the forced re-auth gracefully (don't hard-error the UI on the first rejected old token).

- **[behavior]** — Default `Config.SameSite` changed from unset (no attribute) to `http.SameSiteLaxMode`.
  - Why: no SameSite attribute left cookie auth open to CSRF.
  - Fix: keep the default for the secure behavior. If you genuinely need cross-site cookies, set
    `SameSite: http.SameSiteNoneMode` (requires Secure). To restore the old no-attribute behavior,
    `SameSite: http.SameSiteDefaultMode`.

- **[behavior]** — The access-token cookie is now `HttpOnly: true` (login and refresh).
  - Why: a JS-readable access token is exposed to XSS exfiltration.
  - Fix: verify no frontend reads the access token from `document.cookie` — it now returns nothing.
    Rely on the cookie being sent automatically.

- **[additive]** — New `auth.TokenType` type + `TokenTypeAccess`/`TokenTypeRefresh` consts, and
  `UserInfo.TokenType` (`json:"typ,omitempty"`). No change required unless you mint tokens yourself.

> `auth/httpauth/jwtgen` is a `package main` CLI (not importable); it only tightened the generated
> `.jwtkey` mode to `0o600`. No downstream impact.

## cors

- **[behavior]** — `WrapOpts.AllowOrigins` matching changed from loose `strings.HasPrefix` to exact-origin
  / `*.`-subdomain-wildcard matching. Field type is still `[]string` (compiles), but the matching rules
  changed.
  - Why: prefix matching let `https://example.com` match `https://example.com.evil.com`.
  - Before → after:
    ```go
    AllowOrigins: []string{"example.com"}              // ❌ bare hostname — now matches nothing
    AllowOrigins: []string{"https://app.example.com"}  // ✅ full origin
    AllowOrigins: []string{"https://*.example.com"}    // ✅ subdomain wildcard (NOT the apex)
    AllowOrigins: []string{"http://localhost:3000"}    // ✅ full origin incl. scheme+port
    ```
  - Fix: audit your list; convert bare hostnames / partial prefixes to full origins or `*.`-wildcards.
    Silently-dropped entries mean CORS starts failing for those origins.

- **[behavior]** — `Access-Control-Allow-Credentials` is emitted only when `AllowCredentials` is exactly
  `"true"` (case-insensitive); a `Vary: Origin` header is now always added.
  - Fix: ensure `AllowCredentials` is exactly `"true"` when you need credentialed CORS — other
    truthy-looking strings no longer enable it.

## requestip

- **[compile]** — `Middleware()` now requires at least one `Option`: `Middleware(opt Option, opts ...Option)`.
  - Why: the old middleware trusted `X-Forwarded-For` / `X-Real-IP` from any client, making `IP(ctx)`
    trivially spoofable. The trust model is now an explicit, mandatory choice.
  - Before → after:
    ```go
    r.Use(requestip.Middleware())                              // before
    r.Use(requestip.Middleware(requestip.NoProxies()))         // exposed directly
    r.Use(requestip.Middleware(requestip.TrustProxies("10.0.0.0/8")))  // behind proxy w/ known ranges
    r.Use(requestip.Middleware(requestip.TrustProxyHops(1)))   // behind N known proxy hops
    ```
  - Fix: add an `Option` matching your deployment. `TrustProxies` / `TrustProxyHops` **panic at startup**
    on an invalid CIDR/IP.

- **[behavior]** — `IP(ctx)` returns a different value even after you add the Option. With `NoProxies()`
  (or `TrustProxyHops(n<=0)`) it returns only the TCP peer (`RemoteAddr`), ignoring forwarding headers.
  With `TrustProxies`, headers are honored only when the direct peer is in a trusted range.
  - Fix: verify the Option matches your actual topology, or `IP(ctx)` returns the proxy address
    (trust too narrow) or a spoofable client address (trust too broad).

## env

- **[behavior]** — `env.IsUnitTest` now derives from `testing.Testing()`, not the `UNIT_TEST` env var.
  Setting `UNIT_TEST=true` no longer enables test mode.
  - Why: a production binary could be forced into test mode via an env var, causing `Require*` to return
    zero-values and boot with empty security config.
  - Impact: non-test binaries now run strict — a missing `.env` → `log.Fatal`, `Require*` enforced.
    Real `go test` binaries auto-detect correctly.
  - Fix: remove any reliance on `UNIT_TEST=true` in non-test tooling/scripts; provide a real `.env` or
    the required vars.

- **[behavior]** — `fatal` messages from `RequireInt`/`RequireBool`/`OptionalInt`/`OptionalBool` no longer
  include the offending value (avoids logging sensitive env values). Only matters if you string-match
  those log lines.

## pdfprinter / pdfprinter2

- **[behavior]** — `pdfprinter.Print` now delegates to `pdfprinter2` (Playwright/Chromium) instead of
  shelling out to `google-chrome`. Signature unchanged (`Print(ctx, html) ([]byte, error)`).
  - Fix: ensure the Playwright/Chromium runtime is available (auto-installed on first use). All the
    limits/SSRF behavior below now apply to these calls. `pdfprinter.Logger` is deprecated/unused;
    renderer logs go to `pdfprinter2.Logger`.

- **[behavior]** — **Outbound egress is blocked to loopback/private/link-local IPs by default** (SSRF
  guard + always-on egress proxy, `EgressProxyEnabled = true`). `data:`/`blob:`/`about:` and public
  http(s) are allowed; http(s) to private/loopback/link-local hosts (e.g. `169.254.169.254` metadata,
  RFC1918) and non-http schemes (`file:`, `ws:`) are aborted. WebRTC APIs are stripped from every page.
  - Fix: no change for public-asset HTML. If your templates load subresources from **internal hosts**,
    they will now fail — allow-list them via the `AllowRequestURL` hook (`PrintOpt`) /
    `EgressProxyOptions.IsBlockedIP`, or set `pdfprinter2.EgressProxyEnabled = false` (not recommended
    for untrusted HTML).

- **[behavior]** — New hard limits (all overridable package vars, read dynamically):
  - `MaxConcurrentRenders = 8` — excess `Print` calls **block** until a slot frees (cancellable via
    `ctx`); if none can be acquired the call errors (`"...overloaded..."`). High-concurrency callers
    must queue/limit their own concurrency and handle the overload error.
  - `MaxHTMLBytes = 20 MB` — `len(html)` over this errors before rendering. Set `= 0` to disable.
  - `RenderTimeout = 30s` — bounds each page op. Raise if legit renders are slower (0 disables).
  - `RenderSettleDelay` (3s) is now `ctx`-cancellable and configurable.
  - `InactiveShutdownDelay` (5m), `MaxBrowserLifetime` (30m) now configurable; recycle drains in-flight
    renders (brief latency spike) instead of killing them.

- **[additive]** — New `pdfprinter2.PrintOpt(ctx, PrintOptInput{...})` with `OnSetupContent` / `OnRender`
  hooks and per-call `AllowRequestURL` SSRF allow-listing. `Print(ctx, html)` still works unchanged.

- **[additive]** — New `pdfprinter2/ssrfproxy` package (resolve-once-and-pin forward proxy defeating DNS
  rebinding). Wired in automatically; only relevant if you customize `EgressProxyOptions` or reuse
  `ssrfproxy.IsBlockedIP` for your own policy.

## remotezip

- **[behavior]** — New process-wide size caps `MaxFileSize` (2 GB) and `MaxTotalSize` (10 GB). A per-entry
  write over the file cap, or cumulative over the total cap, errors and puts the `Zip` into a permanent
  error state.
  - Why: zip-bomb / disk-exhaustion protection.
  - Fix: no change if under the caps. If you build larger archives, raise or zero them **before use**
    (`remotezip.MaxFileSize = 0` / `remotezip.MaxTotalSize = 0`). These are package globals.

- **[behavior]** — Entry names are now sanitized (zip-slip): backslash-normalized, path-cleaned, leading
  `/` and `../` stripped; empty/`.` result errors the `Zip`.
  - Fix: no change for normal relative names. If you relied on leading-slash or `..` in entry names,
    those are now rewritten or rejected.
  - `AddMemoryFile` / `AddRemoteFile` signatures are unchanged.

## paginate

- **[behavior]** — Zero/negative page size now clamps to `DefaultPageSize`, and a negative page clamps to
  0. Previously negatives underflowed to an enormous `uint64` `LIMIT`/`OFFSET`.
  - Fix: no change. Note the request page-size clamp to `MaxPageSize` (default 50) still applies — a
    request for 500 rows still silently returns `MaxPageSize`. Confirm callers don't expect more.

## currency

- **[behavior]** — `currency.Parse` now parses negatives correctly, rejects malformed input it used to
  accept, and errors on overflow instead of wrapping.
  ```go
  currency.Parse("-5.50")                  // before: -450 cents (wrong) → after: -550 cents
  currency.Parse("2.-5")                   // before: 195 cents (garbage) → after: error
  currency.Parse("99999999999999999999")   // before: wrapped/negative → after: "out of range" error
  ```
  - Fix: no signature change (still `(Cents, error)`). Ensure you handle the returned error, and re-check
    any code/data that relied on the old buggy negative or garbage results.

## random

- **[behavior]** — `GetAlphaNumericChars` now rejects modulo-biased bytes and validates charset size;
  output distribution changed (no reproducibility contract existed — crypto output). Only matters if you
  snapshot-tested exact output.
- **[behavior]** — `random.Int(min, max)` returns an error when `max <= min` instead of panicking.
  ```go
  random.Int(5, 5)   // before: panic → after: (0, error)
  ```
  Signature already `(int, error)`; switch any panic-recovery to error handling.

## pqchan

- **[behavior]** — Channel-name validation tightened from `^[A-z\_0-9]+$` to `^[A-Za-z0-9_]+$`.
  - Why: `[A-z]` matched the ASCII range between `Z` and `a`, silently allowing `` [ \ ] ^ _ ` ``. Names
    feed `LISTEN`/`NOTIFY`.
  - Fix: ensure channel names use only letters, digits, and underscore. Names like `foo]bar` / `foo^bar`
    now fail validation and must be renamed. (Identifiers are also `pgx.Identifier{}.Sanitize()`d.)

## pqshared

- **[compile]** — `pqshared.Pool` type changed from `*pgx/v4/pgxpool.Pool` to `*pgx/v5/pgxpool.Pool`
  (init uses `pgxpool.New` instead of `pgxpool.Connect`).
  ```go
  // before: import "github.com/jackc/pgx/v4/pgxpool"; var p *pgxpool.Pool = pqshared.Pool
  // after:  import "github.com/jackc/pgx/v5/pgxpool"; var p *pgxpool.Pool = pqshared.Pool
  ```
  - Fix: switch imports to `pgx/v5` anywhere you touch `pqshared.Pool`, migrate your own pgx v4 API usage
    to v5, and run `go mod tidy`.

## model

- **[behavior/dependency]** — Postgres driver blank-import bumped from `pgx/v4` to `pgx/v5`. No exported
  Go API changed; `sqlx.Open` path and `DB_TYPE=postgres` default unchanged.
  - Fix: run `go mod tidy`. If you directly import `pgx/v4` elsewhere, migrate to v5 to avoid two driver
    versions in your module graph.

<a id="githubcd"></a>
## integrations/github/githubcd

- **[behavior]** — Deploy webhooks + health checks now default to **`https://`** for target servers
  (unless a `SERVERS` entry is explicitly prefixed `http://`).
  - Why: the forwarded request carries a replayable GitHub-style HMAC header (no timestamp/nonce). Over
    cleartext an on-path attacker could capture and replay it into the deploy/RCE path.
  ```
  // before: POST http://<server><remoteHookPath>
  // after:  POST https://<server><remoteHookPath>
  ```
  - Fix: ensure each `SERVERS` host is reachable over HTTPS. To keep plaintext for a specific internal
    server, prefix that entry with `http://`. (Send/connect errors now `continue` to the next server.)

- **[behavior]** — `githubcdutil.Verify(secret, r)` now uses constant-time HMAC comparison (`hmac.Equal`
  on decoded bytes) and requires a valid-hex signature.
  - Why: the old `value != sig` string compare leaked timing that could recover a valid signature
    byte-by-byte, gating a deploy/RCE path.
  - Fix: no signature change. Third-party/webhook senders must send HMAC-SHA256 as valid hex in
    `x-hub-signature-256` (GitHub already does). Non-hex / wrong-length signatures are rejected as
    `"invalid hash"`. First-party `SignAndSetHeader` callers are unaffected.

<a id="error-disclosure-er-res-bgtaskutil-parallelize-pqworkqueue"></a>
## Error disclosure (er, res, bgtaskutil, parallelize, pqworkqueue)

A cross-cutting change: recovered errors are now logged in full server-side (with a stack trace, under a
**correlation id**), while clients/consumers get only a generic message + that id. Applies everywhere the
recover path surfaces an error.

- **[behavior]** — `er` gained `ErrorLog`, `ReturnErrorMessageToClient`, `GenericErrorMessage`, and
  `SafeError(*HandlerInput)`. In production (`!env.IsTesting`) recovered errors return
  `GenericErrorMessage` ("An unexpected error occurred") + a correlation id; full message + stack go to
  `er.ErrorLog` (stderr). Dev/testing still returns raw message + stack. `HttpError.Error()`/`Unwrap()`
  are now nil-safe (`&HttpError{}` no longer nil-derefs in the recover path).

- **[behavior]** — `res` panic/error responses (`WrapHTTPFunc`, `AutoHandleHttpPanics`) now return
  `{"error":"An unexpected error occurred", "stackTrace":"", "correlationId": <id>, ...}` instead of the
  raw message + full stack. Signatures are source-compatible.

- **[behavior]** — `bgtaskutil.JsonErrorResult`, `parallelize.Run` errors, and `pqworkqueue` persisted
  failure results (via `GetResult`) all now carry the redacted view + `correlationId`, no raw
  error/stack in prod.

**To restore old behavior where you need it:** set `er.ReturnErrorMessageToClient = true` (returns the
real message, still no stack outside dev), run in test mode for full detail, or reassign `er.ErrorLog`
to redirect diagnostics.

**Fix everywhere:** any frontend/UI/test that asserted on the raw `error` text or `stackTrace` of a 5xx
response will now see the generic message + `correlationId` — update those assertions, and use the
correlation id to find the full error in server logs.

- **[behavior]** — `bgtaskutil.RetryPQLockingErrors` / `IsUserInterupt` return `false` instead of
  panicking on nil input (hardening; no caller change).

## worker

- **[behavior]** — `Worker.Stop()` is now idempotent (`sync.Once` on a private `done` channel);
  `Stop()` twice, or `Trigger`/`TriggerWithInput` after stop, no longer panic (were double-close /
  send-on-closed-channel). Error backoff also aborts promptly on `Stop()` instead of always sleeping 10s.
  Struct signatures unchanged; no caller change needed.

## ratelimit

- **[behavior]** — `Stop()` is now idempotent and also terminates the background goroutine (previously
  only stopped the ticker, leaking it). A previously-panicking double `Stop()` no longer panics. No
  caller change; signatures unchanged.

## Non-breaking hardening (FYI, no action)

- **apiversion** — `NewVer` caps parsing at 8 dotted segments; `Current` returns `nil` instead of
  panicking on a wrong-typed ctx value. Realistic version strings parse identically.
- **jsutil** — `Encode` now escapes U+2028/U+2029 (JS line terminators) in output. HTML escaping and
  camelCase key casing unchanged. Only matters for byte-exact output comparisons.
- **lg** — log lines now marshal every field through the JSON encoder (proper escaping) instead of
  `fmt.Sprintf` splicing. Output-formatting only; no Go API change.
- **pkglog** — `DEFAULT_LOG_OUTPUT` file now opened append-mode at `0600` (was truncate at `0777`).
  Verify no other-owner process needs to read the log; existing files keep their old perms.
- **email** — `TemplateInput.Title`, `Section.HTML`, `SectionHTML(string)` documented as raw un-escaped
  HTML sinks (comments only). **Audit callers:** never pass untrusted input into these; pre-escape with
  `template.HTMLEscapeString`.
- **sample.go** — signals recommended usage: pass a trust policy to `requestip.Middleware` and a role
  (e.g. `RoleUser`) to route registrations that trigger tasks.
