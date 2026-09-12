# #110: bounded repetition on native Linux with accumulating real accounts

Delta base: `2ecfeef3e155a659358bb3a06d212f7f3fe63458`. This temporary diagnostic delta leaves the original accounts script, formal browser runner, 900-sample resource monitor, and formal CI jobs byte-identical. Earlier evidence is immutable.

## Question and stopping boundary

Run `34680438033` passed both the independent native accounts case and all 34 formal browser invocations. It produced no native crash explanation. The isolated case's account directory initially contained four accounts; the prior failing formal case returned 25 with another page. Both correctly found the intended account after search. This is a reproducible state difference, not evidence that pagination caused a renderer crash.

The next bounded question is whether repeating the **unchanged** full accounts case against one real backend raises the spontaneous crash rate as accounts accumulate. There are exactly **12 rounds**, not retry-until-green. Each original round registers two accounts; expected totals progress from four to 26. The final round can therefore cross the 25-row directory boundary (25 returned plus `next_cursor`). Actual `account-role-targeting.json` evidence must confirm that state; the estimate alone is not an assertion that it happened.

Any first failure stops the run. If all 12 rounds pass without a natural crash, stop expanding this experiment, state that the intermittent native fault remains unexplained, remove all temporary structures, and complete #110 as bounded diagnostic improvement after final affected validation and independent review. Do not claim a native root-cause fix.

## Minimal implementation

Only the PR #111 temporary diagnostic job changes. `diagnose-accounts-webkit-repeat.cjs` reads the existing runner and requires exactly two occurrences of its original accounts invocation (the normal/release-workflow and approval-regression branches). It replaces only those invocation strings with a fixed 12-round loop; the selected `release-workflow` branch executes one such loop.

The generated copy must be beside the original runner in `scripts/`, preserving the runner's directory-derived repository root. Each call still executes the original `run_browser_suite` → original `run_timeout 600` → independent Node process → unchanged `web/e2e/accounts.mjs`. Consequently the real profile creation and within-round persistent close/reopen semantics are unchanged. The database/services start once and survive between successful rounds. Each round gets its own `recovery-webkit/iteration-N` artifact directory; the runner records every attempted result and exits on the first failure.

The job retains its **15-minute** budget, each case retains **600 seconds**, and resource collection retains **900 samples**. This does not promise 12 rounds will finish if a case consumes its full limit. A job deadline or first case failure means incomplete repetition, never success. DEBUG, kernel collection and the original runner's SQL/credential/cleanup checks remain active. A failed monitor stop or temporary-file removal cannot replace the case's exit code; removal errors remain on stderr.

**Required cleanup before #110 delivery:** remove the temporary diagnostic job, native sampler, repeat generator, any generated runner copy, and temporary formal DEBUG setting. The EXIT trap removes each generated copy during the job. Preserve the diagnostic evidence, including failed attempts. Completion of this bounded investigation does not require claiming the underlying native crash is resolved.

## Local checks

`generator-check.json` is produced by:

```sh
python3 docs/verification/2026-09-12-accounts-webkit-repeat/probes/check-generator.py
```

The probe generates the actual temporary runner and verifies replacing its two inserted loops with the exact original call reconstructs the original runner bytes. Bash syntax passes. It exercises the generated loop with a small infrastructure-only case fixture: 12 successes create 12 distinct Node processes/output directories and retain the 600-second argument; a third-round exit 42 stops after exactly three rounds and propagates **42**. Changed source match counts and a wrong output directory fail before emitting a runner. These checks verify orchestration, not business behavior; the real business loop runs in CI.

`cleanup-failure-check.json` separately confirms a missing monitor and deliberately failed temporary-file removal still preserve case exit 42 while exposing the cleanup error. `unchanged-scope.json` records byte equality for the formal workflow outside the temporary diagnostic block, original runner, accounts script and native sampler. Node syntax, workflow YAML parsing and `git diff --check` pass.

No business regression was rerun locally for unchanged source. The native amd64 12-round result is pending the reviewed CI run. No commit or push was made by the implementation agent.
