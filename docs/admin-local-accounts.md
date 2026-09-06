# Local Account entry and sessions: T1–T2

[#35](https://github.com/asherzj/relational-config-center/issues/35) implements the
account-entry slice and [#36](https://github.com/asherzj/relational-config-center/issues/36)
implements profile and session lifecycle from
[#34](https://github.com/asherzj/relational-config-center/issues/34).
Open `/register`, `/login` or `/account` in Web. Registration immediately enables
the account and logs it in. Current identity shows the real display name,
username and unverified email. Login uses username, never email. No mail service
or mail operation exists.

**TMP-01:** only the account entry is authenticated by these sessions during T1–T2.
The existing business workspace still uses its previous deployment Token and
fixed Operator. #37 owns connecting the protected workspace, migrating its tests
and removing those old paths. T1/T2 are development slices, not a complete release
of local-account protection. The account entry now has final profile forms,
foreground activity reporting and session cleanup. #37 connects the protected
workspace to this identity and removes TMP-01. The final upgrade, required-schema
startup checks and real browser path belong to #40.

## Start the development entry

Fresh MySQL installations use `deploy/mysql/init/001-schema.sql`. An existing
final Policy Catalog uses `deploy/mysql/migrations/007-local-accounts.sql` once.
The account table can be empty. Do not replay this migration on a fresh schema.

Keep the existing development database and TMP-01 settings, and add:

```dotenv
ADMIN_PUBLIC_ORIGIN=http://127.0.0.1:5173
ADMIN_ALLOW_LOCAL_HTTP=true
```

Run Web at that exact loopback origin and proxy `/api` to Admin using the existing
Vite setup. The account routes ignore a proxy-injected deployment Token. Cookie
or CSRF data never goes in a URL or Web persistent storage. For HTTPS set the exact
public `https://...` origin and omit the local HTTP exception. The public origin
is required; it is never inferred from Host or forwarding headers.

`ADMIN_TRUSTED_PROXIES` optionally contains comma-separated CIDRs. With no setting,
only the socket peer supplies the source IP. A trusted peer permits walking
`X-Forwarded-For` from right to left until the first untrusted address; malformed
chains fall back to the peer. `X-Real-IP` and `Forwarded` are not used. Public API
and Web must share an origin; account routes do not offer cross-origin CORS access.

## HTTP contract

All responses use `Cache-Control: no-store`. Error responses use the existing
`error.code`, safe `error.message` and `error.request_id` envelope.

| Method and path | Request | Success |
| --- | --- | --- |
| `GET /api/v1/auth/csrf` | No credentials required | `200 {"csrf_token":"..."}` and a 10-minute pre-login Cookie |
| `POST /api/v1/auth/register` | `username`, `email`, `password`, optional `display_name` | `201` current identity and a new session Cookie |
| `POST /api/v1/auth/login` | `username`, `password` | `200` current identity and a new session Cookie |
| `GET /api/v1/auth/session` | Session Cookie | `200` current identity; never extends idle time |
| `POST /api/v1/auth/activity` | Session Cookie; no body | `200` current identity after recording server activity time |
| `PATCH /api/v1/auth/profile` | `display_name` | `200` updated current identity |
| `PATCH /api/v1/auth/email` | `email`, `current_password` | `200` updated current identity; email remains unverified |
| `POST /api/v1/auth/password` | `current_password`, `new_password` | `204`; changes password and revokes every session |
| `POST /api/v1/auth/logout` | Session Cookie | `204`; revokes that session and clears its Cookie |
| `POST /api/v1/auth/logout-all` | Session Cookie | `204`; advances the account session version and revokes every session |

Every state-changing request requires `X-CSRF-Token` and the configured `Origin`, or a verifiable
same-origin `Referer` if Origin is absent. Registration/login use the preparation
Cookie and CSRF pair. Web obtains a fresh pair immediately before each submitted
registration/login and holds one browser-wide entry lock through the matching
request, so another tab cannot overwrite the shared preparation Cookie between
those two calls. Background session checks never mint preparation credentials.
Successful registration/login consumes that preparation
credential, replaces any current session in the same browser, and returns a new
session CSRF token. A pre-login Cookie can never read current identity. Missing,
forged, expired or revoked sessions return `401 session_invalid`.

Profile routes can affect only the account identified by the current session.
Display-name and email changes preserve the Account ID, username and session
deadlines. Email and password changes verify the current password. A wrong current
password returns `400 current_password_invalid` and leaves both profile and session
unchanged. Email conflicts return `409 account_conflict`. Password changes succeed
even when the replacement equals the old password and still revoke all sessions.
There is no account-list or other-account management route.

The current identity shape is:

```json
{
  "account": {
    "id": "ab09850e-ef9a-4317-a000-d67465416b5b",
    "username": "alice",
    "display_name": "Alice",
    "email": "alice@example.com",
    "email_verified": false,
    "status": "enabled"
  },
  "csrf_token": "opaque-CSRF-value",
  "idle_expires_at": "2026-09-07T00:30:00Z",
  "expires_at": "2026-09-07T08:00:00Z"
}
```

The account ID is permanent lowercase UUID v4. Username is trimmed and ASCII
lowercased, 3–32 ASCII characters, begins with a letter and otherwise permits
letters, numbers, `.`, `_`, `-`. Email is trimmed and ASCII lowercased, at most 254
ASCII characters, syntax checked, unique and always unverified. Dots and `+tag`
are preserved. Display name defaults to username only when omitted; when supplied,
it is trimmed, 1–64 Unicode code points and has no control characters. An explicit
empty value or JSON null is rejected instead of taking the omitted-field default. Passwords
are 15–128 Unicode code points, preserving all spaces and case without truncation.

| Error | Status | Meaning |
| --- | --- | --- |
| `invalid_account_fields` | 400 | Required account field format/length failed |
| `invalid_credentials` | 401 | Identical message for unknown username, wrong password or disabled account |
| `current_password_invalid` | 400 | A profile/password form supplied the wrong current password; the session remains valid |
| `csrf_invalid` | 403 | CSRF pair or origin failed; no account/session transition |
| `account_conflict` | 409 | Normalized username or email is occupied |
| `auth_rate_limited` | 429 | Wait the integer seconds in `Retry-After` |
| `auth_unavailable` | 503 | Authentication/database dependency unavailable |
| `auth_timeout` | 504 | Authentication/database operation timed out |

Web does not automatically retry writes. After a lost registration response, check
current identity or log in using the original username/password. A committed
registration may produce a conflict if submitted again. Account request deadlines are shorter than driver socket and HTTP deadlines;
lock waits and elapsed deadlines return 504. Service errors preserve
existing cookies and offer explicit state rechecking.

## Persisted security and resource limits

Passwords use Argon2id v19 with independent 128-bit random salts, 19 MiB memory,
two iterations, parallelism one and a 256-bit output. At most two password
computations run concurrently per process. The encoded hash includes parameters.
Sessions and pre-login credentials use independent random 256-bit opaque tokens;
MySQL stores SHA-256 token digests and CSRF digests. CSRF is derived from the opaque
Cookie token with a purpose prefix, so it can be returned after refresh without
persisting a raw CSRF token. It cannot be used as a session token.

Session cookies last up to eight hours. Every identity check enforces both the
30-minute idle and eight-hour absolute boundary, current enabled status and the
account's password/session versions. No query extends idle time. Ordinary logout
deletes the current session. HTTPS uses `__Host-rcc-session` and
`__Host-rcc-preauth`, Secure, HttpOnly, SameSite=Lax, Path=/ and no Domain. Explicit
loopback HTTP uses `rcc-session-dev` and `rcc-preauth-dev` without Secure.

Only `POST /activity` changes `last_active_at`. It accepts no activity payload and
uses server time. Identity reads and other background queries never extend idle
time. At exactly 30 minutes idle or eight hours after creation, a session is invalid
and cannot be revived. Web reports only visible `pointerdown`, `keydown`, or
`touchstart` interaction and reserves at most one report per 60 seconds across
tabs. The browser-wide activity lock serializes the shared timestamp check,
reservation and request; a failed report removes the reservation so a later
interaction can retry.
Logout, logout-all, password change and browser-session replacement publish a
credential-free storage event so other tabs clear or recheck their in-memory view.
The only persistent Web values added here are an activity timestamp and an event
kind/time/nonce; account data, email, session/CSRF credentials and passwords remain
out of browser storage.

| Setting | Default window and limit |
| --- | --- |
| `ADMIN_REGISTER_LIMIT` | 10 registration submissions per source IP/hour, including conflicts and invalid fields |
| `ADMIN_LOGIN_IP_LIMIT` | 60 login attempts per source IP/minute |
| `ADMIN_LOGIN_FAILURE_LIMIT` | 10 failed or in-progress attempts per normalized username/15 minutes |

Username attempts are reserved transactionally before password computation, so
concurrent source IPs cannot all pass the same counter. In-flight reservations and
completed failures are separate counts. A failed credential check settles its
reservation into a failure; other rejected paths release it. Successful login
clears completed failures and settles only its own reservation in the same
transaction that issues the session, preserving other in-flight reservations.
Reservations include the window end so late completions cannot change a new window.
Rejected requests settle with a separate one-second deadline even if their request
was cancelled. If MySQL is still unavailable, settlement fails closed and that
reservation remains conservatively occupied until the window expires, just as
when a process terminates during password work. Unknown usernames
use the same rules and perform equivalent-cost dummy password verification.
Counters and time windows persist in MySQL; dependency failures reject admission.

Control tables are `rcc_accounts`, `rcc_login_sessions`,
`rcc_preauth_credentials`, `rcc_auth_rate_limits`, `rcc_auth_control_lock`.
The existing protected `rcc_` prefix excludes them from generic discovery,
assignment and data APIs. Admissions serialize on one database lock row, with
caps of 10,000 active pre-login credentials, 100,000 sessions and 100,000 rate
buckets. Every admission removes expired pre-login credentials, expired/idle
sessions and ended rate windows before checking capacity; it never evicts an
unexpired session or unfinished rate window to make room. Full capacity returns
503 until expiry frees room. There is no in-memory rate reset at restart.

## Verification and contract checks

`admin/cmd/admin/account_integration_test.go` verifies the public HTTP contract
against real MySQL 8.4. Normal setup registers/logs in; database writes only set up
explicit faults or security-state scenarios. The Web account tests verify UI
feedback and no write replay through the real API client with a simulated network
boundary. `currentIdentitySchema` checks returned JSON at runtime with Zod.
Domain tests cover Unicode and normalization rules. The full existing discovery
regression also proves that adding these protected tables does not reveal them.

The repository has no generated shared OpenAPI contract or independent import-graph
checker. Public HTTP integration assertions, runtime Zod validation and Web tests
are the current machine checks; they do not claim generated Go/TypeScript parity.
The Go `internal` boundary continues to protect the service package boundary.

```bash
make test
make build
pnpm --dir web test:run
pnpm --dir web typecheck
pnpm --dir web build
# For this development host's Colima context:
DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock \
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
go test -v -count=1 -timeout=20m -tags=integration ./admin/...
```

On other hosts use that host's Docker provider settings. Confirm integration tests
actually ran rather than treating Testcontainers provider skips as success.
