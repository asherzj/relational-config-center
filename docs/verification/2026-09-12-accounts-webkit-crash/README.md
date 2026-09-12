# #110: obtain native evidence after PR #111 records a reviewer page crash

Delta base: `d4f12028732086769006cf7b55d7c19a2b0e036b`. Earlier `2026-09-12-accounts-webkit/` evidence is immutable. This delta changes only CI diagnostic configuration and adds a temporary resource sampler; it does not change accounts/application source or claim a crash fix.

## Actual PR #111 failure

`formal-pr111/` preserves run `34679068825` attempt 1. The original publication-confirmation click passed in **916 ms**, with a maximum observed frame gap of 24 ms. The later failure is the second reviewer's “确认批准” click at `accounts.mjs:435`, after the real record-conflict/latest-baseline rebuild.

The actual lifecycle establishes this order:

| Relative time | Event |
| --- | --- |
| 40721 ms | `page-crash`, reviewer context, current modification order |
| 40722 ms | `test-failure`: unstable confirmation click, then TargetClosed |
| 40882 ms | `cleanup-start` |
| 40883–40971 ms | Deliberate page/context/browser closes during cleanup |

This proves the reviewer page crashed **before** failure handling and cleanup. The healthy main page could still produce its failure screenshot and evaluate its state. It does not prove why the renderer crashed, nor retroactively prove the cause of the original main-run publication-confirmation failure. The case lasted about 41.36 seconds, far short of its unchanged 600-second deadline.

A trace replay checking that no `page-crash` precedes `test-failure` and `cleanup-start` failed with `1 !== 0`, as expected on this real trace. This is a real captured failure signal, not a local live reproduction of the underlying native fault. The original run's actual scope remains 16 completed invocations, then accounts WebKit failure, with subsequent invocations unexecuted.

## Evidence needed next

Ranked, falsifiable directions communicated before further probes:

1. A native WebKit/JSC/rendering fault: browser stderr or kernel output should identify a signal/assertion and affected process, without an OOM indication.
2. Memory pressure/system termination: cgroup OOM counters or kernel OOM logs should identify a kill; sampled memory/RSS provides context.
3. Navigation/lifecycle races: an earlier deliberate close or relevant navigation would precede the failure. The new trace already excludes final cleanup as the initiating close; other races remain unproven.

The public [Playwright webkit-2336 crash report #42330](https://github.com/microsoft/playwright/issues/42330) is only a lead: its reported long-lived 20–40 minute journey differs from this approximately 40-second case, and no matching native stack or offset is available here. No dependency upgrade, renderer flag, motion suppression or permission change is justified by that report alone.

## Temporary CI collection

The formal Browser acceptance job gets only `DEBUG=pw:browser`, which records browser launch/process stderr/exit information. Its full case sequence, shared real-service state, all business assertions, timeout, result and cleanup behavior are unchanged. Removing that one added environment line and the new diagnostic job reproduces the prior workflow byte-for-byte.

The separate `Accounts WebKit native diagnostic` job runs only for PR #111 on Ubuntu's native amd64 runner. It invokes the same isolated runner and unchanged `accounts.mjs@webkit` once, with the same 600-second case limit. It uses no retry or forced click and retains failure exit status. Because this starts with a fresh service fixture, it does **not** recreate the 16 previous successful invocations' accumulated service state in the formal suite. A pass in this diagnostic job does not negate a formal-suite failure. Native browser logging is enabled in both jobs so the formal path retains its own evidence.

The separate resource sampler records at most 900 samples, approximately one per second: selected `/proc/meminfo` fields, available unified-cgroup memory events/current/max, and `ps` PID/parent PID/executable-name/RSS columns. It does not inspect argv, environment variables, HTTP data, cookies or process memory. Per-sample `ps` has a 500-ms deadline and a 1-MiB output limit. Unsupported reads become explicit unavailable fields. File-write failures do not change the case result. The shell kills/joins the monitor in its EXIT trap and preserves the case's exit code.

After the diagnostic case, an `always()` step collects kernel warnings/errors, including potential native crashes/OOM records. A denied kernel read remains visible in the artifact and does not override the case outcome. There are no core dumps, arbitrary memory captures, remote shells or external services. One-second resource samples can miss brief peaks; absence of a sampled peak is not proof against OOM without matching system evidence.

**Required exit condition:** after #110 has an evidenced resolution and its affected regressions pass, remove this diagnostic job, `scripts/diagnose-accounts-webkit-native.cjs`, and the temporary formal `DEBUG` setting within #110 before delivery. Keep the resulting immutable evidence. These probes are not a deferred permanent testing framework.

## Local validation of this delta

- `node --check scripts/diagnose-accounts-webkit-native.cjs` and `git diff --check` pass.
- Ruby/Psych parses the workflow; comparing the old jobs after removing only the diagnostic job and formal DEBUG line gives exact original bytes.
- `monitor-check/resources.jsonl` has three real Linux samples from the official cached arm64 Playwright image. It contains actual meminfo, cgroup OOM counters and process RSS columns. A running Node process carried `rcc110_argv_canary` in argv; the canary is absent from all samples.
- `monitor-check-initial/` preserves an initial sampler check that missed Node processes because Linux Node 24 uses `MainThread` as its executable name. The final sampler records the four explicitly allowed columns for every process instead of guessing names; it still never records argv.
- `monitor-check-initial/exit-preservation.json` records a deliberately missing monitor PID while the case returns 42: the shell still returns **42**. Sampling/cleanup failure does not hide the case's failure.
- `monitor-check/browser-native.log` is a real Linux WebKit launch/close with `DEBUG=pw:browser`; it verifies the diagnostic channel yields native browser lifecycle output.

The local containers use `--rm`, an explicit task name and a read-only source mount; they are removed on exit. This delta does not rerun unchanged business tests locally: the prior frozen accounts three-engine evidence remains valid for its unchanged source, while native amd64 collection requires the new CI run. No new CI crash explanation is claimed before its logs arrive. Independent review and a coordinating-agent push precede that run.
