from pathlib import Path
import json, os, shutil, subprocess, tempfile

repo = Path(__file__).resolve().parents[4]
scripts = repo / 'scripts'
generator = scripts / 'diagnose-accounts-webkit-repeat.cjs'
original = (scripts / 'browser-acceptance.sh').read_text()
invocation = 'run_browser_suite "session, conflict and unknown recovery ($browser_engine)" "$repo_root/web/e2e/accounts.mjs" "$artifact_root/release-workflow/recovery-$browser_engine" 600 "$browser_engine"'
node = shutil.which('node')
fd, generated_path = tempfile.mkstemp(prefix='.accounts-repeat-check-', dir=scripts)
os.close(fd)
generated_path = Path(generated_path)
report = {}
try:
    subprocess.run([node, str(generator), str(generated_path)], check=True)
    generated = generated_path.read_text()
    start = generated.index('for accounts_diagnostic_iteration in {1..12}; do')
    end = generated.index('\ndone', start) + len('\ndone')
    loop = generated[start:end]
    assert generated.count(loop) == 2
    assert generated.replace(loop, invocation) == original
    subprocess.run(['bash', '-n', str(generated_path)], check=True)
    report['generated_only_two_accounts_call_sites'] = True
    report['generated_runner_bash_syntax'] = 'pass'
    with tempfile.TemporaryDirectory(prefix='rcc-repeat-check-') as temporary:
        temporary = Path(temporary).resolve()
        fixture = temporary / 'case.cjs'
        fixture.write_text('''const fs=require('fs'),path=require('path');
const [output,seconds,engine]=process.argv.slice(2);
fs.mkdirSync(output,{recursive:true});
const round=Number(path.basename(output).replace('iteration-',''));
const record={round,pid:process.pid,output,seconds,engine};
fs.appendFileSync(process.env.CHECK_RESULTS,JSON.stringify(record)+'\\n');
if(round===Number(process.env.CHECK_FAIL_ROUND))process.exit(42);
''')
        driver = temporary / 'driver.sh'
        driver.write_text('''set -Eeuo pipefail
run_browser_suite() {
  "$CHECK_NODE" "$CHECK_FIXTURE" "$3" "$4" "$5"
}
''' + loop + '\n')
        for label, fail, count, exit_code in [('twelve-successes', 0, 12, 0), ('first-failure-stops', 3, 3, 42)]:
            results = temporary / (label + '.jsonl')
            env = {**os.environ, 'CHECK_NODE': node, 'CHECK_FIXTURE': str(fixture), 'CHECK_RESULTS': str(results), 'CHECK_FAIL_ROUND': str(fail), 'repo_root': str(repo), 'artifact_root': str(temporary / label), 'browser_engine': 'webkit'}
            result = subprocess.run(['bash', str(driver)], env=env, capture_output=True, text=True)
            rows = [json.loads(line) for line in results.read_text().splitlines()]
            assert result.returncode == exit_code, result.stderr
            assert [r['round'] for r in rows] == list(range(1, count + 1))
            assert len({r['pid'] for r in rows}) == count
            assert len({r['output'] for r in rows}) == count
            assert all(r['seconds'] == '600' and r['engine'] == 'webkit' for r in rows)
            report[label] = {'rounds': count, 'distinct_node_processes': count, 'distinct_output_directories': count, 'case_timeout_argument': 600, 'exit_status': result.returncode}
        fixture_generator = temporary / generator.name
        fixture_generator.write_bytes(generator.read_bytes())
        (temporary / 'browser-acceptance.sh').write_text(original.replace(invocation, 'changed_invocation', 1))
        rejected_path = temporary / 'rejected.sh'
        result = subprocess.run([node, str(fixture_generator), str(rejected_path)], capture_output=True, text=True)
        assert result.returncode != 0 and not rejected_path.exists()
        assert 'Original accounts invocation changed' in result.stderr
        report['source_drift_rejected_before_output'] = True
        result = subprocess.run([node, str(generator), str(rejected_path)], capture_output=True, text=True)
        assert result.returncode != 0 and not rejected_path.exists()
        assert 'Diagnostic copy must stay beside the formal runner' in result.stderr
        report['wrong_directory_rejected_before_output'] = True
finally:
    generated_path.unlink(missing_ok=True)
assert (scripts / 'browser-acceptance.sh').read_text() == original
print(json.dumps(report, indent=2))
