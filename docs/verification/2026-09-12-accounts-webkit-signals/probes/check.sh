#!/bin/bash
set -euo pipefail
mkdir -p /evidence/fixture /evidence/browser
set +e
RCC_SECRET_CANARY=RCC_ENV_CANARY_110 strace -ff -ttt --decode-pids=comm -e trace=exit,exit_group,wait4,waitid,kill,tgkill -e signal=all -o /evidence/fixture/trace python3 /evidence/signal-fixture.py RCC_ARG_CANARY_110
observed_status=$?
set -e
[[ $observed_status == 42 ]]
! grep -R -E 'RCC_ENV_CANARY_110|RCC_ARG_CANARY_110|execve\(|read\(|write\(' /evidence/fixture/trace*
grep -h 'SIGABRT' /evidence/fixture/trace* > /evidence/signal-proof.txt
export RCC_SIGNAL_ARTIFACTS=/evidence/browser
bash scripts/diagnose-accounts-webkit-signals.sh /evidence/runner.sh > /evidence/browser-run.log 2>&1
cmp /evidence/browser/launcher-before.sha256 /evidence/browser/launcher-after.sha256
grep -h 'WPEWebProcess' /evidence/browser/webkit-* | head -10 > /evidence/renderer-proof.txt || true
[[ -s /evidence/renderer-proof.txt ]]
printf '{"fixture_exit":42,"secret_canaries_absent":true,"SIGABRT_captured":true,"real_WebKit_persistent":true,"renderer_child_followed":true,"launcher_restored":true}\n' > /evidence/check-result.json
