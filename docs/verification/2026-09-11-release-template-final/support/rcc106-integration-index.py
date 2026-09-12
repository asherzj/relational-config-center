import json
from pathlib import Path
import re

root = Path('/private/tmp/rcc-issue-106-release-template-final/docs/verification/2026-09-11-release-template-final')
inventory = {}
pending = []
for line in (root / 'integration-all-package-inventory.txt').read_text().splitlines():
    if re.fullmatch(r'(Test|Example|Fuzz|Benchmark)\S+', line):
        pending.append(line)
    ending = re.match(r'^(ok|FAIL|\?)\s+(\S+)', line)
    if ending:
        inventory[ending[2]] = {'tests': [name for name in pending if not name.startswith('Benchmark')], 'benchmarks_not_requested': [name for name in pending if name.startswith('Benchmark')]}
        pending = []
assert not pending

def parse(log):
    packages = {}
    tests = {}
    started = set()
    for line in log.read_text().splitlines():
        beginning = re.match(r'^=== RUN\s+(\S+)$', line)
        if beginning and '/' not in beginning[1]:
            started.add(beginning[1])
        status = re.match(r'^--- (PASS|FAIL|SKIP): (\S+) \(([^)]*)\)', line)
        if status and '/' not in status[2]:
            tests[status[2]] = {'status': status[1], 'duration': status[3]}
        ending = re.match(r'^(ok|FAIL|\?)\s+(github\.com/\S+)', line)
        if ending:
            for name in started - set(tests):
                tests[name] = {'status': 'STARTED_WITHOUT_COMPLETION'}
            packages[ending[2]] = {'package_status': ending[1], 'tests': tests}
            tests = {}
            started = set()
    return packages, {'started': sorted(started), 'results': tests}

full, running = parse(root / 'raw/mysql-full.log')
rechecks = []
for filename in ['foreign-key-fault-recheck.log', 'business-flow-fixture-recheck.log', 'mutation-catalog-fixture-recheck.log', 'mixed-batch-summary-recheck.log', 'identity-flow-fixture-recheck.log', 'http-error-fixture-recheck.log', 'mysql-unfinished-admin-02.log']:
    log = root / 'raw' / filename
    if log.exists():
        packages, _ = parse(log)
        rechecks.append((filename, packages))
rows = []
for package, content in inventory.items():
    for test in content['tests']:
        attempts = []
        initial = full.get(package, {}).get('tests', {}).get(test)
        if initial:
            attempts.append({'log': 'raw/mysql-full.log', **initial})
        for filename, packages in rechecks:
            result = packages.get(package, {}).get('tests', {}).get(test)
            if result:
                attempts.append({'log': 'raw/' + filename, **result})
        rows.append({'package': package, 'test': test, 'attempts': attempts, 'effective': attempts[-1] if attempts else {'status': 'NOT_EXECUTED_OR_CURRENT_PACKAGE_PENDING'}})
summary = {}
for row in rows:
    status = row['effective']['status']
    summary[status] = summary.get(status, 0) + 1
result = {'inventory_command': 'go -C admin test -tags=integration -list . ./...', 'packages': inventory, 'complete_full_packages': {name: content['package_status'] for name, content in full.items()}, 'pending_package_output': running, 'expected_test_count': len(rows), 'summary': summary, 'rows': rows}
(root / 'integration-results.json').write_text(json.dumps(result, indent=2) + '\n')
print(len(inventory), len(rows), summary, 'pending package top completions', len(running['results']))
