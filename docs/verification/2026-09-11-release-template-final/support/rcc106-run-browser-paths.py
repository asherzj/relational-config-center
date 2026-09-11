import datetime
import json
import os
from pathlib import Path
import subprocess

root = Path('/private/tmp/rcc-issue-106-release-template-final')
evidence = root / 'docs/verification/2026-09-11-release-template-final'
targets = [
    'TestRollbackFlowBrowserSystemPath',
    'TestReleaseEmergencyBrowserSystemPath',
    'TestReleaseTemplateBrowserSystemPath',
    'TestTableReleaseTemplateBrowserSystemPath',
    'TestApprovalRoleBrowserSystemPath',
    'TestTableApprovalBrowserSystemPath',
    'TestNotificationCenterBrowserSystemPath',
    'TestApprovalNotificationsBrowserSystemPath',
    'TestReleaseNotificationsBrowserSystemPath',
    'TestApprovalFinalBrowserSystemPath',
    'TestAccountBrowserSystemPath',
]
results = []
def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()
for target in targets:
    output = evidence / 'go-browser' / (target + '-01')
    assert not output.exists(), output
    log = evidence / 'raw' / ('browser-' + target + '-01.log')
    command = ['go', '-C', 'admin', 'test', '-v', '-count=1', '-timeout=45m', '-tags=integration,browser', './cmd/admin', '-run', '^' + target + '$']
    env = dict(os.environ, RCC_E2E_ENGINE='chromium', RCC_E2E_OUTPUT=str(output))
    row = {'test': target, 'started_at': now(), 'command': command, 'source': 'source-11.json', 'log': str(log.relative_to(evidence)), 'output': str(output.relative_to(evidence)), 'status': 'running'}
    results.append(row)
    (evidence / 'go-browser-runs.json').write_text(json.dumps(results, indent=2) + '\n')
    print(now(), 'START', target, flush=True)
    with log.open('xb') as raw:
        result = subprocess.run(command, cwd=root, env=env, stdout=raw, stderr=subprocess.STDOUT)
    row.update(exit_code=result.returncode, finished_at=now(), status='passed' if result.returncode == 0 else 'failed')
    (evidence / 'go-browser-runs.json').write_text(json.dumps(results, indent=2) + '\n')
    print(now(), row['status'].upper(), target, flush=True)
raise SystemExit(0 if all(row['exit_code'] == 0 for row in results) else 1)
