# Local Accounts, sessions, protected workspace and maintenance: T1–T5

[#35](https://github.com/asherzj/relational-config-center/issues/35) implements the
account-entry slice and [#36](https://github.com/asherzj/relational-config-center/issues/36)
implements profile and session lifecycle from
[#34](https://github.com/asherzj/relational-config-center/issues/34).
Open `/register`, `/login` or `/account` in Web. Registration immediately enables
the account and logs it in. Current identity shows the real display name,
username and unverified email. Login uses username, never email. No mail service
or mail operation exists.

[#37](https://github.com/asherzj/relational-config-center/issues/37) protects every
business route and attributes every authored row/catalog change to the requesting
account's permanent ID. TMP-01 has been removed: no shared Token, disabled-auth
mode, proxy credential injection or normal fixed Operator remains. #38 adds
interrupted in-memory edit recovery; #39 adds account maintenance commands; #40 owns final
required-schema startup checks and complete release acceptance.

## Start the development entry

Fresh MySQL installations use `deploy/mysql/init/001-schema.sql`. An existing
final Policy Catalog uses `deploy/mysql/migrations/007-local-accounts.sql` once.
The account table can be empty. Do not replay this migration on a fresh schema.

Remove old deployment Token, disabled-auth and fixed Operator settings, and set:

```dotenv
ADMIN_PUBLIC_ORIGIN=http://127.0.0.1:5173
ADMIN_ALLOW_LOCAL_HTTP=true
```

Run Web at that exact loopback origin and proxy `/api` to Admin using the existing
Vite setup. No business or account route accepts a deployment Token. Cookie
or CSRF data never goes in a URL or Web persistent storage. For HTTPS set the exact
public `https://...` origin and omit the local HTTP exception. The public origin
is required; it is never inferred from Host or forwarding headers.

`ADMIN_TRUSTED_PROXIES` optionally contains comma-separated CIDRs. With no setting,
only the socket peer supplies the source IP. A trusted peer permits walking
`X-Forwarded-For` from right to left until the first untrusted address; malformed
chains fall back to the peer. `X-Real-IP` and `Forwarded` are not used. Public API
and Web must share an origin; the API does not offer cross-origin CORS access.

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
| `account_disabled` | 401 | A previously valid session belongs to a disabled account; Web destroys its recoverable in-memory state |
| `auth_rate_limited` | 429 | Wait the integer seconds in `Retry-After` |
| `auth_unavailable` | 503 | Authentication/database dependency unavailable |
| `auth_timeout` | 504 | Authentication/database operation timed out |

Web does not automatically retry writes. After a lost registration response, check
current identity or log in using the original username/password. A committed
registration may produce a conflict if submitted again. After a lost password-change
response, use the login path with the intended new password. After a lost business
write response, use the offered read-only catalog, rule-detail or Managed Data query;
the original write stays blocked until leaving that uncertain result. Account request deadlines are shorter than driver socket and HTTP deadlines;
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

## Interrupted workspace recovery

An expired or ordinarily revoked session immediately clears the in-memory business
credential and hides the mounted workspace. Query filters, configuration row input,
rule forms, table assignments and an open Change Set stay only in that tab's memory.
The interruption view accepts a normal public login. If the permanent Account ID is
unchanged, Web refetches every active rule, Schema, catalog and target query before it
reveals the workspace. A pending Managed Data Change Set is rebuilt from the current
Schema and, when it has an ID, an exact current-row query. The user must press the
confirmation button again; recovery never sends a business write.

A different Account ID clears the query cache and remounts the workspace, destroying
the previous account's drafts before the new account is shown. Explicit logout,
logout-all, refresh and a cross-tab ended event also destroy them. Each authenticated
business response carries `X-RCC-Account-ID`; Web compares it with the in-memory
session before parsing the body. Session generation checks before and after parsing
turn late success, 401, error and chained readback results into `stale_session` instead
of exposing them to the new account. When local storage rejects publication,
`BroadcastChannel` carries the same event; visibility verification remains the final
supported-browser fallback for a missed notification or changed Cookie.

Authentication dependency failures (`503` or `504`) hide the workspace but retain the
Cookie and same-account in-memory intent so the user can retry identity inspection.
They are distinct from session invalidation. A disabled account returns
`401 account_disabled` for its still-identifiable old session and Web destroys all
recoverable state. Login still returns the same `invalid_credentials` response for an
unknown username, wrong password and disabled account.

The disable operation preserves enough old login-session rows to classify a
previously authenticated Cookie as `account_disabled`, while atomically setting the
account disabled and advancing its session version. It does not delete those rows as
ordinary logout does. Re-enabling the account does not revive them: with the
account enabled again, their older version must resolve to `session_invalid`. Expired
session cleanup may remove them at the normal expiry boundary. This maintenance
contract keeps disabled-account draft destruction observable without revealing
account status through login.

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

## Protected business requests and Operator attribution

Every business endpoint under `/api/v1` requires a current enabled account and
valid Login Session. Non-GET/HEAD requests additionally require the session CSRF
credential and configured Origin (or same-origin Referer), including POST queries.
Health endpoints remain public and an empty account directory accepts registration.
Missing or revoked sessions return `401 session_invalid`; authenticated rule
rejections and CSRF failures remain stable 403 responses, and Web does not treat
those as login failures. Business responses use `Cache-Control: no-store` and
include the permanent requesting Account ID in `X-RCC-Account-ID` so Web can
reject a response when another tab replaced the shared Cookie.

HTTP authenticates once before entering a business use case. The immutable
`AuthenticatedOperator` result binds its private Account ID to that request's
context. Shared services never store a mutable actor. Configuration Auto Fill and
all Query, Mutation and Table Policy creation/updates use that account ID. A
revocation prevents subsequent authentication; a write already authenticated may
finish and retains the original account attribution. Authentication lookup has
its own deadline and does not impose that deadline on a later business transaction.

Historical Operator text stays unchanged. Each ADD/MODIFY checks the live metadata
of the Operator slots it actually fills: an unrestricted text column must hold
at least 36 characters. Short CHAR/VARCHAR and ENUM return
`422 operator_field_incompatible` before the row changes. Reads preserve historical
text, unaffected slots do not block a write, and DELETE fills no Operator slots.
Admin never alters business Schema; the table maintainer must correct incompatible
columns. All `rcc_` tables, including accounts, sessions, preauth, rate windows and
the capacity lock, are excluded from discovery and generic policies/row operations.

## Script login and one business call

The following example uses Python's hidden password input and a private temporary
Cookie jar. Do not enable shell tracing. Run it against the configured public
origin after registering an account. It performs no automatic write retry.

```bash
umask 077
ORIGIN=http://127.0.0.1:5173
COOKIE_JAR=$(mktemp)
SESSION_JSON=$(mktemp)
trap 'rm -f "$COOKIE_JAR" "$SESSION_JSON"' EXIT
CSRF=$(curl --fail --silent --show-error -c "$COOKIE_JAR" \
  "$ORIGIN/api/v1/auth/csrf" | python3 -c 'import json,sys; print(json.load(sys.stdin)["csrf_token"])')
python3 -c 'import getpass,json; print(json.dumps({"username":getpass.getpass("Username: "),"password":getpass.getpass("Password: ")}))' | \
  curl --fail --silent --show-error -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -H "Origin: $ORIGIN" -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
    --data-binary @- "$ORIGIN/api/v1/auth/login" > "$SESSION_JSON"
CSRF=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["csrf_token"])' < "$SESSION_JSON")
curl --fail --silent --show-error -b "$COOKIE_JAR" "$ORIGIN/api/v1/table-policies"
# Example non-read request: report actual foreground activity, no payload.
curl --fail --silent --show-error -b "$COOKIE_JAR" -X POST \
  -H "Origin: $ORIGIN" -H "X-CSRF-Token: $CSRF" "$ORIGIN/api/v1/auth/activity"
```

For business changes use the same Cookie jar, Origin and CSRF header with the
existing JSON contract. A 401 requires a new login; 403 is a rule/CSRF rejection.
After a lost write response, query its outcome before deciding whether another
write is necessary. Old `ADMIN_API_TOKEN`, `ADMIN_AUTH_DISABLED`, `ADMIN_OPERATOR`
and `ADMIN_CORS_ORIGINS` settings now produce a configuration error. The Vite
proxy also rejects `RCC_ADMIN_TOKEN`. Maintenance `policy-migrate` reads only
`MYSQL_*` plus explicit `POLICY_MIGRATION_OPERATOR` for historical attribution;
its connection is independent of normal Admin HTTP configuration and readiness.

## Account maintenance without Web login

`make build` produces `bin/admin/account-maintain`. The normal Admin image also
ships `/usr/local/bin/account-maintain`; select that executable with the container
runtime's `--entrypoint account-maintain` and supply the same private `MYSQL_*`
environment. Neither command needs a running Admin, a Cookie, an HTTP origin, or
Policy Catalog readiness. It needs the final account control tables from migration
007 and database permission to read and update those tables. It does not migrate
schemas implicitly. Stop old shared-Token Admin instances before the account
cutover; final upgrade verification is tracked in #40.

Choose exactly one immutable Account ID or username. Username lookup uses the same
trimming and lowercase rule as login. The command prints JSON containing the
operation, Account ID, username and enabled status; it never prints an email,
password, hash, Cookie or raw database error.

```bash
bin/admin/account-maintain lookup --username alice.one
bin/admin/account-maintain lookup --id 550e8400-e29b-41d4-a716-446655440000
bin/admin/account-maintain disable --username alice.one
bin/admin/account-maintain enable --username alice.one
```

Reset passwords through standard input only. The executable rejects terminal
input, password arguments and extra positional arguments. For interactive use,
Python's hidden prompt supplies stdin without placing the password in shell
history, process arguments or environment variables:

```bash
python3 -c 'import getpass,sys; sys.stdout.write(getpass.getpass("New password: "))' | \
  bin/admin/account-maintain reset-password --username alice.one --password-stdin
```

Stdin is read through EOF as exact UTF-8 bytes, including spaces and newlines;
use a secret source that does not append an unintended newline. The password must
contain 15–128 Unicode characters, with no normalization or truncation. Resetting
to the same password is still successful and revokes all sessions. Password hash,
password version, session version and session revocation commit together. Resetting
a disabled account does not enable it and keeps its old session records available
for `account_disabled` draft destruction until normal expiry cleanup. Communicate a replacement password offline;
there is no email delivery, temporary-password state or forced-change flow.

Correct an email only after independently verifying the intended account:

```bash
python3 -c 'import getpass,sys; sys.stdout.write(getpass.getpass("Correct email: "))' | \
  bin/admin/account-maintain set-email --username alice.one --email-stdin
```

Email correction uses the same ASCII, format, length, trimming, lowercase and
database uniqueness rules as registration. It releases the old email, preserves
Account ID and username, stays unverified, and neither revokes nor renews sessions.
An email claim is not proof of ownership. There are no mail operations or account
rename, deletion, merge or public account-management APIs. If compromise is
suspected, separately reset the password or disable the account.

Disable advances the session version atomically with account status and retains
old session rows until ordinary expiry cleanup so Web can destroy drafts. Enable
requires a fresh login; repeated enable/disable of an unchanged status is a no-op.
Disabled accounts retain their username and email. Disabling an account is not a
person ban: open registration and unverified emails allow that person to register
different details. Organizational network admission remains the boundary.

Exit status 0 means success, 2 means a malformed command or unreadable/oversized
stdin, and 1 means rejected fields or a failed operation/configuration/database
problem. Diagnostics identify missing accounts,
invalid fields, occupied email, timeout or unavailable control storage without
echoing sensitive input. A timeout, connection failure or lost output can leave
the caller unsure whether a write committed: inspect status or verify login before
deciding to repeat an operation; a repeated password reset revokes sessions again.

`TestAccountMaintenance*` starts the delivered executable and observes its effects
over HTTP with MySQL 8.4. It also proves rollback when session revocation fails,
missing-schema diagnostics and independence from normal Admin readiness.
`TestLocalAccountSecurityChangesWinAgainstVerifiedLogin` uses a second real Admin
with a controlled clock after credential verification and before issuance; the
same barrier has a successful no-revocation control. Real HTTP changes and CLI
reset/disable operations then prove that a verified old snapshot cannot issue a
session after security versions change, including disable followed by enable.

The final regression also covers uncertain policy writes (AC-026): Query and
Mutation Policy details inherit the list's lifecycle-command block, including
after closing and reopening a drawer. Command and form entry points check that
block before writing. Query, Mutation and Table Policy forms/commands keep the
uncertain result after a failed 503/504 read check, including the normal safe-read
retry, and only clear it after successful reads. Read-only checks never replay
the original command. The three policy-page suites exercise these paths.
