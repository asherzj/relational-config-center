// Driver contract tests only; these controls do not reproduce the #113 bug.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync, spawnSync } = require('node:child_process');
const driver = require('./diagnose-a11y-webkit-repeat.cjs');
const repo = path.resolve(__dirname, '..');
const original = execFileSync('git', ['show', `${driver.BASE}:scripts/browser-acceptance.sh`], { cwd: repo, encoding: 'utf8' });
const generated = driver.generateRunner(original);
const start = generated.indexOf('if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == unsaved-changes ]]');
const end = generated.indexOf('if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == release-workflow ]]', start);
const caseSection = generated.slice(start, end);

function fixture(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rcc-a11y-driver-test-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function exercise(t, { fail = '', seconds = 0, exhaustAfter = '' } = {}) {
  const directory = fixture(t);
  // The generated shell is executed against an external case boundary returning
  // controlled exit statuses. It never starts a browser, application or Docker.
  const script = `set -Eeuo pipefail
artifact_root=$1
repo_root=/frozen-original
browser_engine_list=(chromium firefox webkit)
RCC_E2E_SUITE=all
SECONDS=${seconds}
run_browser_suite() {
  local engine=\${5:-chromium}
  local id="\${2##*/}@$engine"
  printf '%s\\t%s\\t%s\\n' "$id" "$3" "\${4:-180}" >> "$artifact_root/calls.tsv"
  mkdir -p "$3"
  if [[ $id == '${driver.CASE}' ]]; then
    [[ $4 == 420 ]] || exit 91
    [[ \${DEBUG:-} == pw:browser ]] || exit 92
    if [[ $diagnostic_round == '${exhaustAfter}' ]]; then SECONDS=1921; fi
    if [[ $diagnostic_round == '${fail}' ]]; then return 7; fi
  elif [[ $id == '${fail}' ]]; then
    return 9
  fi
}
${caseSection}
printf complete > "$artifact_root/complete"
`;
  const result = spawnSync('bash', ['-s', '--', directory], { input: script, encoding: 'utf8' });
  assert.equal(result.error, undefined);
  const callsFile = path.join(directory, 'calls.tsv');
  const calls = fs.existsSync(callsFile) ? fs.readFileSync(callsFile, 'utf8').trim().split('\n').map(row => row.split('\t')) : [];
  return { directory, result, calls };
}

test('generated runner changes only the bounded loop, pre-service gates and WebKit process observer', t => {
  const directory = fixture(t);
  const file = path.join(directory, 'runner.sh');
  fs.writeFileSync(file, generated);
  execFileSync('bash', ['-n', file]);
  const originalA11y = execFileSync('git', ['show', `${driver.BASE}:web/e2e/browser-accessibility.cjs`], { cwd: repo });
  assert.equal(driver.sha256(originalA11y), driver.HASHES['web/e2e/browser-accessibility.cjs']);
  assert.match(originalA11y.toString(), /await button\('note 申请值：转换为 LF 再编辑'\).click\(\)/);
  assert.doesNotMatch(originalA11y.toString(), /clickWithDiagnostics/);
  const serviceLimits = source => source.split('\n').filter(line => line.includes('run-with-timeout.cjs') && line.includes(' 2400 '));
  assert.deepEqual(serviceLimits(generated), serviceLimits(original));
  assert.equal(serviceLimits(generated).length, 2);
  const originalCall = '  run_browser_suite "browser-accessibility ($browser_engine)" "$repo_root/web/e2e/browser-accessibility.cjs" "$artifact_root/browser-accessibility/$browser_engine" 420 "$browser_engine"';
  assert.equal(generated.replace(driver.WEBKIT_LOOP, originalCall).replace(`${driver.IDENTITY_CHECK}\n`, '')
    .replace(`${driver.TRACE_SETUP}\n`, '').replace(driver.TRACE_SELECT, '')
    .replace(driver.TRACED_COMMAND, driver.ORIGINAL_COMMAND), original);
  assert.ok(generated.indexOf(driver.TRACE_SETUP) < generated.indexOf("printf 'Starting disposable MySQL"));
  assert.ok(generated.indexOf("assert.equal(process.arch, 'x64')") < generated.indexOf("printf 'Starting disposable MySQL"));
  assert.match(generated, /assert.equal\(pkg.version, '1.63.0'\)/);
  assert.match(generated, /assert.equal\(webkit.revision, '2359'\)/);
  assert.throws(() => driver.generateRunner(`${original}\n`), /original runner mismatch/);
});

test('all-success boundary runs the exact seven-case prefix and twelve unique WebKit outputs', t => {
  const { calls, result, directory } = exercise(t);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(calls.slice(0, 7).map(row => row[0]), driver.PREFIX);
  assert.equal(calls.length, 19);
  assert.ok(calls.slice(7).every(row => row[0] === driver.CASE && row[2] === '420'));
  assert.equal(new Set(calls.slice(7).map(row => row[1])).size, 12);
  assert.ok(fs.existsSync(path.join(directory, 'complete')));
});

test('the original supervisor covers the observer only for accessibility WebKit', t => {
  const directory = fixture(t);
  const functionSource = generated.slice(generated.indexOf('run_browser_suite() {'), generated.indexOf('\nif [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == unsaved-changes ]]'));
  const script = `set -Eeuo pipefail
repo_root=/frozen-original
artifact_root=$1
runtime_dir=/owned-runtime
mysql_container=owned-mysql
web_url=http://localhost
mysql_port=3306
mysql_password=controlled-placeholder
run_timeout() { printf '%s\\n' "$@"; }
${functionSource}
for engine in chromium firefox webkit; do
  run_browser_suite controlled /frozen-original/web/e2e/browser-accessibility.cjs "$artifact_root/$engine" 420 "$engine"
done
`;
  const result = spawnSync('bash', ['-s', '--', directory], { input: script, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  for (const engine of ['chromium', 'firefox', 'webkit']) {
    const args = fs.readFileSync(path.join(directory, engine, 'runner.log'), 'utf8').trim().split('\n');
    assert.deepEqual(args, engine === 'webkit'
      ? ['420', 'node', '/frozen-original/scripts/.a11y-native-trace.cjs', path.join(directory, engine), 'node', '/frozen-original/web/e2e/browser-accessibility.cjs']
      : ['420', 'node', '/frozen-original/web/e2e/browser-accessibility.cjs']);
  }
});

for (const fail of [1, 6, 12]) test(`round ${fail} failure stops with its original status and no later call`, t => {
  const { calls, result, directory } = exercise(t, { fail: String(fail) });
  assert.equal(result.status, 7, result.stderr);
  assert.equal(calls.length, 7 + fail);
  assert.equal(fs.existsSync(path.join(directory, 'complete')), false);
});

test('a prefix failure prevents every WebKit round', t => {
  const { calls, result, directory } = exercise(t, { fail: driver.PREFIX[0] });
  assert.equal(result.status, 9);
  assert.equal(calls.length, 1);
  assert.equal(fs.existsSync(path.join(directory, 'round-events.tsv')), false);
});

test('insufficient complete-case budget starts no WebKit round', t => {
  const { calls, result, directory } = exercise(t, { seconds: 1921 });
  assert.equal(result.status, 125);
  assert.equal(calls.length, 7);
  assert.match(fs.readFileSync(path.join(directory, 'round-events.tsv'), 'utf8'), /^1\tbudget-exhausted\t/);
});

test('budget exhaustion after a completed round does not shorten or start the next round', t => {
  const { calls, result, directory } = exercise(t, { exhaustAfter: '3' });
  assert.equal(result.status, 125);
  assert.equal(calls.length, 10);
  const events = fs.readFileSync(path.join(directory, 'round-events.tsv'), 'utf8');
  assert.match(events, /3\tpassed\t/);
  assert.match(events, /4\tbudget-exhausted\t/);
  assert.doesNotMatch(events, /4\tstarted\t/);
});

test('summary distinguishes unexecuted rounds and rejects incomplete success', t => {
  const directory = fixture(t);
  const prefix = driver.PREFIX.map(id => `${id}\tselected\t0\n`).join('');
  fs.writeFileSync(path.join(directory, 'case-results.tsv'), `case\tselection\texit_status\n${prefix}${driver.CASE}\tselected\t7\n`);
  fs.writeFileSync(path.join(directory, 'round-events.tsv'), '1\tstarted\t100\n');
  const summary = driver.summarize(directory, 7);
  assert.equal(summary.rounds[0].state, 'failed-or-interrupted');
  assert.ok(summary.rounds.slice(1).every(round => round.state === 'unexecuted'));
  assert.equal(summary.originalIssueResolved, false);
  assert.throws(() => driver.summarize(directory, 0), /incomplete experiment/);
});

test('source mismatch prevents the external execution boundary and removes only its snapshot', async t => {
  const directory = fixture(t);
  const external = path.join(directory, 'do-not-remove');
  fs.writeFileSync(external, 'existing data');
  const marker = path.join(directory, 'executed');
  const result = await driver.withOwnedSnapshot(directory, async snapshot => {
    for (const file of Object.keys(driver.HASHES)) {
      const destination = path.join(snapshot, file);
      fs.mkdirSync(path.dirname(destination), { recursive: true });
      fs.writeFileSync(destination, execFileSync('git', ['show', `${driver.BASE}:${file}`], { cwd: repo }));
    }
    fs.appendFileSync(path.join(snapshot, 'web/e2e/browser-accessibility.cjs'), '\n');
    driver.verifySource(snapshot);
    return driver.runCommand(process.execPath, ['-e', 'require("node:fs").writeFileSync(process.argv[1],"started")', marker]);
  });
  assert.equal(result.status, 1);
  assert.match(result.failure.message, /original source mismatch/);
  assert.equal(result.removed, true);
  assert.equal(fs.existsSync(marker), false);
  assert.equal(fs.readFileSync(external, 'utf8'), 'existing data');
});

test('external-command failure survives successful snapshot cleanup', async t => {
  const directory = fixture(t);
  const result = await driver.withOwnedSnapshot(directory, async snapshot => {
    fs.writeFileSync(path.join(snapshot, 'owned'), 'temporary');
    return driver.runCommand(process.execPath, ['-e', 'process.exit(7)']);
  });
  assert.equal(result.status, 7);
  assert.equal(result.removed, true);
});

for (const originalStatus of [0, 7]) test(`filesystem cleanup failure cannot replace original status ${originalStatus} with success`, async t => {
  const directory = fixture(t);
  const result = await driver.withOwnedSnapshot(directory,
    () => driver.runCommand(process.execPath, ['-e', `process.exit(${originalStatus})`]),
    () => { throw Object.assign(new Error('controlled filesystem denial'), { code: 'EACCES' }); });
  assert.equal(result.status, originalStatus || 1);
  assert.equal(result.removed, false);
  assert.equal(result.cleanupError.code, 'EACCES');
});

test('post-command evidence failure retains an already-failed external exit status', async t => {
  const directory = fixture(t);
  const result = await driver.withOwnedSnapshot(directory, async () => {
    const exitStatus = await driver.runCommand(process.execPath, ['-e', 'process.exit(7)']);
    throw Object.assign(new Error('controlled invalid evidence'), { exitStatus });
  });
  assert.equal(result.status, 7);
  assert.equal(result.removed, true);
  assert.match(result.failure.message, /invalid evidence/);
});

test('identity gate resolves pnpm package symlinks and rejects the wrong installed core', t => {
  const directory = fixture(t);
  const modules = path.join(directory, 'web/node_modules');
  const playwright = path.join(modules, '.pnpm/playwright@1.63.0/node_modules/playwright');
  const core = path.join(modules, '.pnpm/playwright-core@1.63.0/node_modules/playwright-core');
  const executable = path.join(directory, 'webkit-2359/pw_run.sh');
  fs.mkdirSync(playwright, { recursive: true });
  fs.mkdirSync(core, { recursive: true });
  fs.mkdirSync(path.dirname(executable));
  fs.writeFileSync(executable, 'external executable fixture; never launched');
  fs.writeFileSync(path.join(playwright, 'package.json'), '{"version":"1.63.0","main":"index.js"}');
  fs.writeFileSync(path.join(playwright, 'index.js'), `module.exports={webkit:{executablePath:()=>${JSON.stringify(executable)}}};`);
  const corePackage = path.join(core, 'package.json');
  fs.writeFileSync(corePackage, '{"version":"1.63.0"}');
  fs.writeFileSync(path.join(core, 'browsers.json'), '{"browsers":[{"name":"webkit","revision":"2359","browserVersion":"26.6"}]}');
  fs.symlinkSync(playwright, path.join(modules, 'playwright'));
  fs.symlinkSync(core, path.join(path.dirname(playwright), 'playwright-core'));
  // Only the host-platform precondition is removed for this portable package
  // boundary test. The native driver retains both Linux and AMD64 assertions.
  const identity = driver.IDENTITY_CHECK.split("<<'RCC_D05_IDENTITY'\n")[1].split('\nRCC_D05_IDENTITY')[0]
    .replace("assert.equal(process.platform, 'linux');", '').replace("assert.equal(process.arch, 'x64');", '');
  const invoke = () => spawnSync(process.execPath, ['-', directory, directory], { input: identity, encoding: 'utf8' });
  const valid = invoke();
  assert.equal(valid.status, 0, valid.stderr);
  const identityFile = path.join(directory, 'native-identity.json');
  assert.equal(JSON.parse(fs.readFileSync(identityFile)).core, '1.63.0');
  fs.unlinkSync(identityFile);
  fs.writeFileSync(corePackage, '{"version":"1.62.1"}');
  const invalid = invoke();
  assert.notEqual(invalid.status, 0);
  assert.equal(fs.existsSync(identityFile), false);
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  for (const phase of ['setup', 'cleanup']) test(`${signal} during synchronous ${phase} removes the owned snapshot`, async t => {
    const directory = fixture(t);
    const ready = path.join(directory, 'ready');
    const release = path.join(directory, 'release');
    const snapshotFile = path.join(directory, 'snapshot');
    const resultFile = path.join(directory, 'result.json');
    const blocker = `const fs=require('node:fs');fs.writeFileSync(${JSON.stringify(ready)},'ready');const until=Date.now()+3000;while(!fs.existsSync(${JSON.stringify(release)})&&Date.now()<until)Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10);`;
    const childSource = `
      const fs=require('node:fs');
      const {execFileSync}=require('node:child_process');
      const {withOwnedSnapshot}=require(${JSON.stringify(require.resolve('./diagnose-a11y-webkit-repeat.cjs'))});
      const block=()=>execFileSync(process.execPath,['-e',${JSON.stringify(blocker)}]);
      withOwnedSnapshot(${JSON.stringify(directory)}, snapshot=>{
        fs.writeFileSync(${JSON.stringify(snapshotFile)},snapshot);
        fs.writeFileSync(snapshot+'/owned','temporary');
        if(${JSON.stringify(phase)}==='setup')block();
        return 0;
      },(snapshot,options)=>{
        if(${JSON.stringify(phase)}==='cleanup')block();
        fs.rmSync(snapshot,options);
      }).then(result=>{
        fs.writeFileSync(${JSON.stringify(resultFile)},JSON.stringify(result));
        process.exitCode=result.status;
      });`;
    const child = require('node:child_process').spawn(process.execPath, ['-e', childSource], { stdio: 'ignore' });
    const completion = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('close', (code, exitedSignal) => resolve({ code, signal: exitedSignal }));
    });
    t.after(() => { if (fs.existsSync(directory)) fs.writeFileSync(release, 'released'); child.kill('SIGKILL'); });
    const deadline = Date.now() + 2000;
    while (!fs.existsSync(ready) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 10));
    assert.ok(fs.existsSync(ready), 'child must enter the real synchronous boundary before signaling');
    const snapshot = fs.readFileSync(snapshotFile, 'utf8');
    assert.ok(fs.existsSync(snapshot));
    child.kill(signal);
    // Let the signal reach the blocked driver before releasing its sync child.
    await new Promise(resolve => setTimeout(resolve, 30));
    fs.writeFileSync(release, 'released');
    const exited = await completion;
    assert.equal(fs.existsSync(snapshot), false, 'handled interruption must not leave its source snapshot');
    assert.deepEqual(exited, { code: signal === 'SIGINT' ? 130 : 143, signal: null });
    const result = JSON.parse(fs.readFileSync(resultFile, 'utf8'));
    assert.equal(result.removed, true);
    assert.equal(result.cleanupError, null);
  });
}
