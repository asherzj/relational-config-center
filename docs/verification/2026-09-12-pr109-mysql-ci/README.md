# PR 109 MySQL CI timeout verification

## Observed failure

`initial-ci-mysql.log` is the unmodified GitHub Actions log from the failed MySQL job. The log records `admin/cmd/admin` reaching Go's 60 minute timeout and returning exit code 2. Because that command did not use `-v` or `-json`, the log does not establish which individual top-level tests passed before the timeout. It does establish that the other five packages containing tests completed after the timed-out package.

`initial-ci-final.json` is the final workflow state captured with that run.

## Inventory and assignment

`independent-inventory.json` and `independent-expected-shards.json` were produced independently from the runner using `go test -tags=integration -list . -json ./...`. They contain 431 unique `(package, name)` identities across six packages.

`runner-inventory.jsonl`, `runner-assignment.json`, and `runner-inventory-result.json` came from the implemented runner's inventory-only path. `assignment-comparison.json` records the exact comparison: the independent and runner assignments have the same normalized SHA-256, with group sizes 108, 108, 108, and 107. The `admin/cmd/admin` package contributes 94, 94, 93, and 93 tests respectively.

The assignment rule sorts all discovered `(package, name)` identities and distributes them round-robin across four groups. It is deterministic for one source tree and automatically includes newly added top-level Test, Example, or Fuzz items.

`runner-tests.log` covers the launcher's accounting rules and a temporary Go module end to end. The fixture proves that package-qualified duplicate names, nested tests, an Example, a Fuzz seed, and the legal Unicode name `Test容量` are discovered and selected by the actual Go tool. It also injects one failing package and proves the following package still runs. Parser cases reject top-level and nested skips, missing or extra top-level items, test failures, and nonzero process exits.

## Targeted real MySQL execution

`targeted-schema-baseline.jsonl` is the direct JSON event stream from:

```text
go test -p 1 -count=1 -timeout=10m -tags=integration -json ./cmd/admin -run '^TestSchemaBaselineRecoversBeforeAttemptWasRecorded$'
```

The selected test emitted `run` then `pass` in 10.23 seconds, and its package emitted `pass` in 11.14 seconds. No other top-level test event appeared. This proves the item reported in the timeout stack can complete on its own against the real local MySQL 8.4 provider; it does not claim that the four complete CI groups have run locally. The formal full-range gate is the four-job GitHub Actions matrix.

`go-unit.log` records the complete ordinary Go suite passing. `go-unit-sandbox.log` preserves the first attempt, where the workspace sandbox denied localhost binds; the same command passed after granting the test process localhost access.
