# Goose / main integration for MR #91

Integration parents: Goose `f48b1c474c33a729d09e5d4aca87c267b0e7f16b`
and main `4aeb54ed8664d569555a3800f9a3c21994b1e890` (field interactions,
PR #90). This records the additional integration work; the dated T1–T3 and
field-interaction verification reports remain the evidence for their original
versions. MR #91 targets main; this work does not merge the MR or deploy it.

## Conflict resolutions and integration behavior

- Keep the serial 60-minute MySQL test command and 70-minute CI budget; retain
  all account, Policy, schema and release-reset build targets.
- Use Goose's read-only complete readiness check and safe startup guidance.
  Field-policy structure joins the same manifest, replacing the incoming partial
  field-policy Ready definition.
- Keep the old current initialization entry deleted. Disable rename inference
  during this merge so main's changes to that file cannot silently alter the
  frozen historical snapshot. Published 00001/00002 SQL and manifests and the
  frozen pre-Goose SQL/data remain byte-for-byte unchanged.
- Append 00003 with the exact field-policy CREATE statement from main's historical
  014. Generate its field-table manifest entry with real MySQL 8.4, explicit
  UTF-8, and SHOW CREATE TABLE; all prior manifest entries remain unchanged.
  New installations and Goose v1/v2 upgrades reach v3. Unmanaged databases first
  complete applicable historical upgrades including 014, then explicitly adopt.
- Retain release-reset as independent offline development/test maintenance. Its
  seven-table preflight now reads the shared manifest under the existing metadata
  locks; transaction, target, trigger, foreign-key and preservation checks remain.
  It does not require a Goose ledger, field-policy table, or HTTP readiness.
- Main's new field/query/reset test callers use Goose initialization. Historical
  acceptance explicitly combines the unchanged frozen snapshot with historical
  014, including existing field-configuration preservation. Current runbooks and
  Compose adoption setup use this same boundary.

## Verification

Go unit/architecture tests, all-module builds, vet and race checks passed after
resolving the release-reset dependency on deleted partial checkers. The first
compile failure is retained in `/private/tmp/rcc-goose-merge-unit.log`; passing
results are in `/private/tmp/rcc-goose-merge-unit-verified.log` and
`/private/tmp/rcc-goose-merge-{build,vet,race}.log`.

The affected MySQL command selects all Schema, current-schema equivalence,
FieldPolicy, ReleaseReset, Combined and legacy-account-upgrade tests:

```sh
go -C admin test -p 1 -json -count=1 -timeout=30m -tags=integration ./cmd/admin \
  -run '^(TestSchema|TestCurrentGoose|TestFieldPolicy|TestReleaseReset|TestAccountUpgradeFromLegacy|TestCombined)'
```

The initial 38-parent run completed in 387.819 seconds with 27 parents passing
and 11 failing. Failures are retained in
`/private/tmp/rcc-goose-merge-integration.jsonl`:

- The legacy process test hit its unchanged three-second rejection deadline.
  Its complete parent passed three consecutive independent runs with the same
  assertions and deadline (50.181 seconds total). A scheduling/resource cause
  is not proven; the initial failure remains recorded.
- Historical 014 imported by MySQL image init scripts acquired incorrectly
  decoded Chinese column comments. The dedicated historical helper now imports
  that unchanged file over an explicit UTF-8 connection; strict production
  comparison is not weakened.
- The new v2→v3 fault test attempted to revoke a nonexistent scoped CREATE
  grant. Its fixture now revokes all grants, then grants exactly the permissions
  needed to reach the intended CREATE and confirmation failure boundaries.

The latter fixes affect only dedicated test setup. Successful tests from the
initial run remain applicable to unchanged production code. All ten other failed
parents passed their complete reruns in 130.032 seconds. The union of the initial
run and complete-parent reruns covers **38 unique top-level tests / 83 including
subtests, all finally passing, with no skips or missing items**. This is affected
integration coverage across multiple invocations, not a single all-green run or
a complete regression run of the newly combined repository.

The [coverage ledger](2026-09-10-goose-main-integration-coverage.csv) maps every
test to its passing log. Log labels map to
`/private/tmp/rcc-goose-merge-{integration,legacy-recheck,corrections-verified}.jsonl`.
Coverage includes v2→v3 data preservation, interrupted DDL and confirmation,
explicit recovery, strict field-table readiness, verified historical adoption,
combined queries and independent release-reset behavior.

Web frozen-lockfile installation, typecheck, **35 files / 375 tests**, and build
passed. Logs are in `/private/tmp/rcc-goose-merge-web-{install,typecheck,tests,build}.log`.
The web source remains identical to main `4aeb54e`.

Real Compose acceptance passed all six scenarios: fresh volume, adopted-volume
repeat, unmanaged-volume blocking, explicit adoption, unconfirmed-state blocking,
and explicit recovery. The official browser acceptance script passed the
`field-interactions` suite in Chromium 151.0.7922.34, Firefox 153.0 and WebKit 26.5:
six checks per engine across 1440×1000 and 390×844, zero browser errors, policies
restored, authentication boundary and database post-checks passed. Database suites
ran serially with disposable owned resources and completed cleanup.

```sh
python3 scripts/compose-migration-acceptance.py \
  --artifacts /private/tmp/rcc-goose-merge-compose
RCC_E2E_ARTIFACTS=/private/tmp/rcc-goose-merge-browser \
RCC_E2E_SUITE=field-interactions RCC_E2E_ENGINES=chromium,firefox,webkit \
  ./scripts/browser-acceptance.sh
```

Local runs used `DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`;
Go integration also used `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`.
[Runtime evidence](2026-09-10-goose-main-integration-runtime.json) retains the
Compose service results, browser checks and post-checks. Original logs and browser
screenshots remain in the artifact directories named above, with run output in
`/private/tmp/rcc-goose-merge-{compose,browser}-run.log`.

Final checks found no unresolved index entries or whitespace errors. Independent
byte comparisons confirmed immutable 00001/00002 migrations, manifests, historical
014 and pre-Goose SQL/data; 00003's Up body exactly matches historical 014 and its
manifest only appends the field-policy table.

## Independent review

Standards found no written-rule violations or reportable subjective code smells.
Spec identified missing 014 steps in account upgrade instructions; those steps
were corrected and independently rechecked. The complete-parent MySQL coverage
ledger was independently reconciled with all three original JSONL logs. Final
runtime evidence was independently reconciled with the original Compose and
browser results. Spec finished with zero unresolved findings or remaining blockers.
