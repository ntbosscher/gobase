# Upgrading gobase

This is a guide for **projects that import `github.com/ntbosscher/gobase`**. It tells you what
to fix when you bump the version. Every entry is tagged:

- **`[compile]`** — your code will not build until you change the call site. Loud, self-announcing.
- **`[behavior]`** — same signature, different runtime behavior. Compiles fine, acts differently.
  These are the dangerous ones — read them.

It covers two upgrade windows, newest first:

- [**`v0.8.0` → `v0.9.0`**](#v080--v090) — per-client rate limiting, request/websocket DoS caps,
  pqworkqueue multi-tenancy (has `[compile]` breaks), and further error-disclosure tightening.
- [**`2cb8487` (pre-`v0.8.0`) → `v0.8.0`**](#v080-and-earlier-2cb8487--v080) — the original
  security-hardening pass.

Regenerate the raw diff for any package with:

```bash
git diff v0.8.0..HEAD -- <path>          # the v0.9.0 window
git diff 2cb8487..v0.8.0 -- <path>       # the earlier window
```

---

# `v0.8.0` → `v0.9.0`

## TL;DR — do these before you ship

**Won't compile until fixed:**

1. **`pqworkqueue` worker/read signatures changed** for multi-tenancy. The `Worker` callback is now
   `func(ctx, input, meta WorkerJobMeta)` (the old `id string` param is gone — the id is on
   `meta.ID`), and `GetStatus`/`GetResult` take input structs instead of a bare id. See
   [pqworkqueue](#pqworkqueue).

**Compiles, but will bite silently — verify each:**

2. **All request bodies are now capped at 64 MB** (`res.MaxRequestBodySize`). Handlers that accept
   larger uploads must raise or disable it, or reads start failing. See [res](#res).
3. **`r.RateLimit` / `r.RateLimitErr` now bucket per-client-IP, not globally.** Correct keying
   requires the `requestip.Middleware` (with a trust policy) to be installed; without it every caller
   collapses to the raw peer address. See [res / ratelimit](#res).
4. **Login now enforces per-IP (10/min) and per-username (5/15min) throttles by default.** Failed
   logins return 429 once a bucket empties. Tune or disable via `Config.LoginPerIP*` /
   `LoginPerUsername*`. See [auth/httpauth](#authhttpauth).
5. **Websocket connections are now capped** (`MaxConcurrentWebSockets = 1024`,
   `MaxWebSocketMessageBytes = 1 MB`). Excess upgrades get 503; oversize frames close the connection.
   See [res/websocket](#reswebsocket).
6. **More error text is now redacted from clients.** `res.Error(err)` logs server-side and returns a
   generic message; auth login/refresh/not-authorized responses no longer echo the underlying reason.
   Update any client/test asserting on those bodies. See [res](#res) and [auth/httpauth](#authhttpauth).
7. **`pqworkqueue` auto-migrates on init**, adding nullable `usr` / `company` columns. Single-tenant
   callers need no code change but the DDL runs at startup. See [pqworkqueue](#pqworkqueue).

---

## res

- **[behavior]** — New `MaxRequestBodySize` (default **64 MB**) caps the total bytes any handler can
  read from a request body, enforced at the router entry and again in `WrapHTTPFunc` via
  `http.MaxBytesReader`. This is separate from `MultipartMaxFormSize` (which only bounds the
  in-memory multipart buffer).
  - Why: without a whole-body cap, an upload can exhaust disk and `io.ReadAll`-style handlers are
    exposed to size/decompression DoS.
  - Fix: no change for normal traffic. Endpoints that legitimately accept larger bodies must raise
    `res.MaxRequestBodySize` (or set it to `0` to disable) **during setup, before serving traffic**.
    Over-limit reads fail through the normal handler/panic path.

- **[behavior]** — `r.RateLimit` / `r.RateLimitErr` (in `res/r`) switched from a single global
  `ratelimit.New` bucket to per-client `ratelimit.NewKeyed`, keyed via
  `requestip.KeyFromRequest(ctx, RemoteAddr)`.
  - Why: one shared bucket meant any client could exhaust the limit for everyone.
  - Fix: install `requestip.Middleware(<trust policy>)` so the key is the real client IP. Without it,
    keying falls back to the raw `RemoteAddr` — behind a proxy that can collapse every caller to one
    key (still per-route, but not per-client). Signatures are unchanged.

- **[behavior]** — `res.Error(err)` now follows the panic-handler disclosure policy: it logs
  `err` to `er.ErrorLog` and returns `er.GenericErrorMessage` to the client, unless
  `er.ReturnErrorMessageToClient` is set (dev). A nil error yields the generic message instead of
  nil-panicking.
  - Fix: any frontend/test that read the raw error text from an `res.Error(...)` response now sees the
    generic message. To pass detail through, set `er.ReturnErrorMessageToClient = true`.

- **[behavior]** — `Content-Disposition` headers (`Download` / `Display`) are now RFC 6266/5987
  compliant: the ASCII `filename` is stripped of control bytes (incl. CR/LF), quotes, backslashes and
  commas, and a `filename*=UTF-8''…` parameter is added for non-ASCII names. `SanitizeDispositionName`
  now removes more than just commas.
  - Fix: no code change. Downloaded filenames may differ slightly (control/quote chars removed;
    non-ASCII now preserved via `filename*`). Only relevant if you byte-compared the header.

## auth/httpauth

- **[behavior]** — The login endpoint now has two on-by-default throttles that consume a token only on
  **failed** logins:
  - per-IP: `LoginPerIPRateLimitCount` / `Window` — default **10 / minute**.
  - per-username: `LoginPerUsernameRateLimitCount` / `Window` — default **5 / 15 min** (normalized
    lowercased/trimmed username).

    Once a bucket empties, further attempts get `429 Too Many Requests` until it refills.
  - Why: throttles online credential-stuffing (per-IP) and distributed low-and-slow brute force
    against one account (per-username).
  - Fix: verify the defaults suit your login flow; raise the counts/windows if legitimate users hit
    them, or set `LoginPerIPRateLimitDisabled` / `LoginPerUsernameRateLimitDisabled`. The per-IP key
    comes from `requestip` — install `requestip.Middleware` for accurate keying behind a proxy.

- **[behavior]** — Auth errors are no longer echoed to the client. `notAuthorizedResponder` logs the
  reason to `er.ErrorLog` and returns a bare `NotAuthorized()`; `refreshHandler` returns
  `"Access denied"` (was `"Access denied: " + err`); `loginHandler` returns `"Login failed"` /
  `"Invalid request"` (was the raw credential-checker / parse error). Access-token creation failures
  now route through `res.Error`.
  - Why: the old messages could distinguish "no such user" vs "bad password" and leak internal detail.
  - Fix: update any client/test asserting on the specific auth failure text; use `er.ErrorLog` (with
    the correlation id) to find the real reason. Set `NotAuthorizedResponder` for a custom message.

## res/websocket

- **[behavior]** — Two new process-wide DoS caps, read at connection setup:
  - `MaxConcurrentWebSockets` (default **1024**) — once reached, further upgrades are rejected with
    `503 Service Unavailable` until a slot frees. Set `0` to disable.
  - `MaxWebSocketMessageBytes` (default **1 MB**) — a larger inbound frame closes the connection (via
    `conn.SetReadLimit`). Set `0` to disable.
  - Fix: raise these if your app legitimately runs many concurrent sockets or exchanges larger
    messages; otherwise no change. The slot is released exactly once on connection close.

- **[docs]** — Clarified that `Router.WebSocket` registers the endpoint with **no auth/role
  middleware** — the handler must authenticate/authorize the connection itself before doing work. No
  behavior change, but audit your socket handlers to confirm they enforce this.

## pdfprinter/pdfprinter2/ssrfproxy

- **[behavior]** — `ssrfproxy.IsBlockedIP` now **allows loopback** (`127.0.0.0/8`, `::1`) when
  `env.IsTesting` (`TEST=true`), so tests can render against a local `httptest` server. Private,
  link-local, cloud-metadata, and CGNAT ranges stay blocked even under test.
  - Fix: none in production. **Never set `TEST=true` in an environment that renders untrusted HTML** —
    it re-opens loopback egress.

## pqworkqueue

- **[compile]** — The `Worker` callback signature changed for multi-tenancy:
  ```go
  // before
  type Worker = func(ctx context.Context, id string, input json.RawMessage) []byte
  // after — id moved onto meta.ID; meta also carries the job's User/Company
  type Worker = func(ctx context.Context, input json.RawMessage, meta WorkerJobMeta) []byte
  ```
  - Fix: update every `WorkerInfo.Callback` (raw API). `Queue2.RegisterWorker` callbacks
    (`func(ctx, arg T) []byte`) are unaffected — the wrapper absorbs the new signature.

- **[compile]** — `GetStatus` and `GetResult` now take input structs instead of a bare id:
  ```go
  // before
  GetStatus(ctx, id)          //  (*Status, error)
  GetResult(ctx, id)          //  ([]byte, error)
  // after
  GetStatus(ctx, GetStatusInput{ID: id})   // optional User/Company to scope the read
  GetResult(ctx, GetResultInput{ID: id})   // optional User/Company to scope the read
  ```
  - Fix: wrap the id in the input struct. For multi-tenant callers, also pass `User` / `Company` so a
    tenant can't read another tenant's status/result by id.

- **[behavior]** — On package init, `pqworkqueue` auto-migrates `pq_worker_queue`, adding nullable
  `usr bigint` / `company bigint` columns (idempotent `add column if not exists`). Enqueue via
  `AddOption{User, Company}` / `AddOption2[T]{User, Company}`; both stay null for single-tenant setups.
  Set `PQWORKQUEUE_SKIP_MIGRATE=true` to skip the DDL if you manage schema yourself.
  - Note: workers are still shared across tenants — isolation is advisory (the callback receives
    `meta.User/Company` and must enforce any per-tenant logic itself).

- **[behavior]** — A dispatch race was fixed so a freed concurrency slot no longer risks waiting up to
  `FallbackCheckInterval` for its next job. No API change; queues just drain more promptly.

- **[behavior]** — Debounce without a merge func, with `DebounceKeepOriginalStart=false`, now updates
  the stored `job_arg` to the newest payload (in addition to `start_after`). Previously the newest
  arg was dropped and only the schedule moved. If you relied on the first payload winning, set
  `DebounceKeepOriginalStart=true` or supply a `DebounceMerge`.

- **[docs]** — A panic inside a `Queue2.RegisterWorker` callback is recovered, recorded as the job's
  result, and the job is marked **complete and not retried** — and because the panic is caught inside
  the worker's own transaction, any DB work it committed (or wrote before panicking) stays committed.
  Unchanged behavior, now documented: keep your worker transaction consistent yourself.

---

# `v0.8.0` and earlier (`2cb8487` → `v0.8.0`)

The original security-hardening pass, between `2cb8487` (2026-06-24) and `v0.8.0` (2026-07-23).

## TL;DR — do these before you ship

**Won't compile until fixed:**

1. **`requestip.Middleware()` now requires a trust policy argument.** Pass `requestip.NoProxies()`,
   `requestip.TrustProxies(cidrs...)`, or `requestip.TrustProxyHops(n)`. See [requestip](#requestip).
2. **`pqshared.Pool` is now `*pgx/v5/pgxpool.Pool`** (was v4). Update imports anywhere you touch it,
   and run `go mod tidy`. Migrate your own `pgx/v4` usage to `v5`. See [pqshared / model](#pqshared).

**Compiles, but will bite silently — verify each:**

3. **All existing auth sessions are invalidated on deploy.** Tokens are now type-bound (`typ` claim);
   pre-upgrade tokens have no `typ` and are rejected. Every user must re-login. See [auth/httpauth](#authhttpauth-1).
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
   flows and any JS reading the access token break. See [auth/httpauth](#authhttpauth-1).
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
