# #101 migration test repair evidence

Workspace: `/private/tmp/rcc-issue-101-release-templates`
Branch: `codex/issue-101-release-templates`
Fixed base: `56dff5360e94d904ab85c17cea5cc575919a29b1`
Date: 2026-09-11. Go 1.27.0 darwin/arm64; MySQL image `mysql:8.4`; Colima Docker 29.5.2; testcontainers-go v0.44.0.

All runs used the admin directory and environment:
`TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache`

## Effective result

29 top-level tests have valid passing evidence, including their selected full subtest groups. `effective-passing-coverage.json` maps each test to its source log. No required affected test remains failed. This is combined evidence, not a claim that the original broad run passed.

- Run 04: 26 top-level tests attempted, 25 passed and one failed; 23 non-readiness migration/baseline successes remain applicable. Command and exact anchored regular expression are preserved in `04-full-affected.sh`; enumerated tests in `04-test-names.json`. The run remains recorded as exit 1.
- Run 09: all 5 top-level tests passed, exit 0, 67.947s. This reruns all three readiness groups plus field-policy and release-stage historical callers. Exact command below.
- Run 11: the remaining legacy-account upgrade caller passed, exit 0, 15.548s. Exact command below.
- Run 03 independently passed historical baseline5 → pending → explicit up8 with all historical data, ledger prefix, and existing login session preserved.

```sh
go test -tags=integration ./cmd/admin -run '^(TestSchemaReadinessRejectsUnmanagedStartup|TestSchemaReadinessContinuouslyChecksStateAndCompleteStructureReadOnly|TestSchemaReadinessRejectsKnownOldRelease|TestReleaseSchemaStagesRecoverWithoutRewritingLegacyBusinessFacts|TestSchemaMigrationAddsFieldPoliciesWithoutChangingPublishedState)$' -count=1 -v
go test -tags=integration ./cmd/admin -run '^TestAccountUpgradeFromLegacyMatchesFreshSchema$' -count=1 -v
```

Each command above requires the recorded environment. Output was redirected to the matching numbered `.log`; actual shell exit status is preserved in the matching `.exit`.

## Original failures and diagnostic runs

- 01: sandbox could not access the Docker socket; exit 1. No false skip/pass.
- 02: authorized real MySQL red: original historical baseline expected current, actual pending/current5/required8.
- 04: new template unique-key readiness fault correctly rejected the fault, but its restore did not reproduce the original table definition.
- 05: accidental repetition of the unmodified readiness test after a preceding edit used the wrong working-directory path. Despite its initial filename, this was not a fixed run; exit 1 is retained.
- 06: restoring index order alone still failed. This remains exit 1.
- 07: real SHOW CREATE before/after showed MySQL index DDL reserialized ASCII-column CHECK literals from `_utf8mb4` to `_ascii`; strict schema validation correctly rejected that altered definition. Full definitions are in the log.
- 08: restoring both indexes and the original UTF-8 CHECK expressions passed the isolated unique-key fault, with exact before/after SHOW CREATE equality. This selected subtest run is not substituted for the full readiness group; full group passed in 09.
- 10: legacy-account historical adoption reproduced the same current-vs-pending error before its repair; exit 1 preserved.

`runs.json` records all original results without reclassifying failed runs as passes.

## Changes and evidence validity

Only five authorized test files were edited by this subtask; their final SHA-256 hashes are in `owned-files.sha256.json`, and their full diff against the fixed base is `final-owned.diff`. This includes pre-existing #101 migration-test edits handed over by the primary agent. No production Go, schema SQL/JSON, docs, or other repository files were edited by this subtask.

- Historical adoption version and count are independently fixed at 5. Baseline/recover report pending until explicit up. Data/idempotency assertions remain intact.
- The current-build historical test asserts ledger `0,1,2,3,4,5`, absence of the template table before up, then ledger `0,1,2,3,4,5,8` and preservation of historical data/session after up.
- Historical physical-schema and release-stage tests use migration binaries frozen at their historical versions; current production requirements remain 8.
- Current new install, main5→8 and SELECT-only startup, next release, partial DDL, bootstrap, commit confirmation, process interruption, ledger rejection, and historical recovery all have passing evidence.
- Complete read-only readiness includes template absence, unique key, CHECK enforcement, and default emergency template presence. The test restores the original CHECK text and index order, with exact SHOW CREATE equality; production validation was not relaxed.
- Legacy-account adoption now checks preservation before and after explicit up, then exercises existing sessions, passwords, policies and business values.
- Fresh/upgrade comparisons use separate databases on one disposable server, rather than concurrent MySQL containers.

`04-admin-inputs.sha256.json`, `09-admin-inputs.sha256.json`, and `final-admin-inputs.sha256.json` record Admin Go/SQL/JSON and module inputs. Since run04 only the readiness, release-details, and account-delivery test files changed; each was fully rerun within its affected selected groups. Production inputs and shared baseline/migration helpers were unchanged.

All 10 published migration SQL/manifest files for versions 1–5 are byte-identical to the fixed base; hashes are in `released-1-through-5.sha256.json`, and `released-1-through-5.diff` is empty. `git diff --check` passed.

No commit, push, issue close, deployment, or Notion update was performed. All created databases were inside disposable task containers; each run was sequential, and the final container was terminated at 06:03:22 UTC+08:00. Database resources and all five file ownerships were handed back to the primary agent for final browser validation.
