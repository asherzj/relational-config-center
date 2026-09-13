// Temporary #113/D05–D06 native feedback driver. Delete before final issue delivery.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { execFileSync, spawn } = require('node:child_process');

const BASE = 'e4d801f5127db44f4d5fdb457dda046208a43341';
const ROUNDS = 12;
const PREFIX = [
  'unsaved-changes.cjs@chromium', 'rule-clarity.cjs@chromium',
  'write-recovery.cjs@chromium', 'operation-coverage.cjs@chromium',
  'complex-fields.cjs@chromium', 'browser-accessibility.cjs@chromium',
  'browser-accessibility.cjs@firefox',
];
const CASE = 'browser-accessibility.cjs@webkit';
const HASHES = {
  'web/e2e/browser-accessibility.cjs': 'd638254da29c8eeba909198c354cb92086950f0ad2e2230a113290e14e89418c',
  'scripts/browser-acceptance.sh': 'cfafd6d6a2a0a5c390a3aca763583819984f18980fb8e58c6410bcd20ee19681',
  'web/package.json': '14ad8e40e0ea0432edd329d62d23857618aaa74cfaa79d81e45e900f315a9297',
  'web/pnpm-lock.yaml': 'df1e4ed005ef942a49e5c819cafd2ec2de7b29fd6ed068582f7e74b25bc76f96',
};
const sha256 = value => createHash('sha256').update(value).digest('hex');
const json = (file, value) => fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);

function verifySource(root) {
  for (const [file, expected] of Object.entries(HASHES)) {
    assert.equal(sha256(fs.readFileSync(path.join(root, file))), expected, `original source mismatch: ${file}`);
  }
}

// This is the only repeated business invocation. A nonzero function return
// reaches the unchanged runner's set -e/EXIT trap; no retry follows failure.
const WEBKIT_LOOP = `  if [[ $browser_engine == webkit ]]; then
    for diagnostic_round in {1..12}; do
      # SECONDS includes setup, so this is conservative relative to both
      # unchanged 2400s service lifetimes. Never shorten a started 420s case.
      if (( SECONDS + 420 + 60 > 2400 )); then
        printf '%s\\tbudget-exhausted\\t%s\\n' "$diagnostic_round" "$SECONDS" >> "$artifact_root/round-events.tsv"
        exit 125
      fi
      printf '%s\\tstarted\\t%s\\n' "$diagnostic_round" "$SECONDS" >> "$artifact_root/round-events.tsv"
      DEBUG=pw:browser run_browser_suite "original accessibility WebKit round $diagnostic_round/12" "$repo_root/web/e2e/browser-accessibility.cjs" "$artifact_root/browser-accessibility/webkit/iteration-$diagnostic_round" 420 webkit
      printf '%s\\tpassed\\t%s\\n' "$diagnostic_round" "$SECONDS" >> "$artifact_root/round-events.tsv"
    done
  else
    run_browser_suite "browser-accessibility ($browser_engine)" "$repo_root/web/e2e/browser-accessibility.cjs" "$artifact_root/browser-accessibility/$browser_engine" 420 "$browser_engine"
  fi`;

const IDENTITY_CHECK = `node - "$repo_root" "$artifact_root" <<'RCC_D05_IDENTITY'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const [root, output] = process.argv.slice(2);
assert.equal(process.platform, 'linux');
assert.equal(process.arch, 'x64');
const modulePath = path.join(root, 'web/node_modules/playwright');
const playwright = require(modulePath);
const realModulePath = path.dirname(require.resolve(modulePath));
const coreRoot = path.dirname(require.resolve('playwright-core/package.json', { paths: [realModulePath] }));
const pkg = require(path.join(modulePath, 'package.json'));
const core = require(path.join(coreRoot, 'package.json'));
const webkit = require(path.join(coreRoot, 'browsers.json')).browsers.find(browser => browser.name === 'webkit');
assert.equal(pkg.version, '1.63.0');
assert.equal(core.version, '1.63.0');
assert.equal(webkit.revision, '2359');
assert.equal(webkit.browserVersion, '26.6');
const executable = playwright.webkit.executablePath();
assert.match(executable, /[/\\\\]webkit-2359[/\\\\]/);
assert.ok(fs.existsSync(executable));
fs.writeFileSync(path.join(output, 'native-identity.json'), JSON.stringify({
  platform: process.platform, arch: process.arch, node: process.version,
  playwright: pkg.version, core: core.version, revision: webkit.revision,
  advertisedBrowserVersion: webkit.browserVersion, executable,
}, null, 2) + '\\n');
RCC_D05_IDENTITY
`;

const TRACE_SETUP = `run_timeout 30 node "$repo_root/scripts/.a11y-native-trace.cjs" "$artifact_root/native-trace-capability" --check\n`;
const ORIGINAL_COMMAND = '    run_timeout "${RCC_E2E_TIMEOUT_SECONDS:-$suite_timeout}" node "$script"';
const TRACED_COMMAND = '    run_timeout "${RCC_E2E_TIMEOUT_SECONDS:-$suite_timeout}" "${diagnostic_command[@]}" "$script"';
const TRACE_SELECT = `  local diagnostic_command=(node)
  if [[ $case_id == browser-accessibility.cjs@webkit ]]; then
    diagnostic_command=(node "$repo_root/scripts/.a11y-native-trace.cjs" "$output" node)
  fi
`;

function generateRunner(original) {
  assert.equal(sha256(original), HASHES['scripts/browser-acceptance.sh'], 'original runner mismatch');
  const invocation = '  run_browser_suite "browser-accessibility ($browser_engine)" "$repo_root/web/e2e/browser-accessibility.cjs" "$artifact_root/browser-accessibility/$browser_engine" 420 "$browser_engine"';
  const block = `for browser_engine in "\${browser_engine_list[@]}"; do\n${invocation}\ndone\nfi\n\nif [[ \${RCC_E2E_SUITE:-all} == all || \${RCC_E2E_SUITE:-all} == release-workflow ]]`;
  assert.equal(original.split(block).length, 2, 'expected exactly one original accessibility block');
  const build = "printf 'Building the Web preview artifact...\\n'";
  assert.equal(original.split(build).length, 2);
  const commandStart = '  if RCC_PLAYWRIGHT_MODULE="$repo_root/web/node_modules/playwright"';
  assert.equal(original.split(commandStart).length, 2);
  assert.equal(original.split(ORIGINAL_COMMAND).length, 2);
  return original.replace(block, block.replace(invocation, WEBKIT_LOOP))
    .replace(build, `${IDENTITY_CHECK}\n${TRACE_SETUP}\n${build}`)
    .replace(commandStart, `${TRACE_SELECT}${commandStart}`).replace(ORIGINAL_COMMAND, TRACED_COMMAND);
}

function readRows(file) {
  if (!fs.existsSync(file)) return [];
  return fs.readFileSync(file, 'utf8').trim().split('\n').filter(Boolean).map(line => line.split('\t'));
}

function summarize(output, exitStatus) {
  const selected = readRows(path.join(output, 'case-results.tsv')).slice(1).filter(row => row[1] === 'selected');
  const prefix = selected.filter(row => row[0] !== CASE);
  assert.deepEqual(prefix.map(row => row[0]), PREFIX.slice(0, prefix.length), 'unexpected prefix order');
  assert.ok(prefix.length <= PREFIX.length);
  const cases = selected.filter(row => row[0] === CASE);
  assert.ok(cases.length <= ROUNDS, 'round limit exceeded');
  const events = readRows(path.join(output, 'round-events.tsv'));
  const rounds = Array.from({ length: ROUNDS }, (_, index) => {
    const number = index + 1;
    const started = events.some(row => row[0] === String(number) && row[1] === 'started');
    const file = path.join(output, `browser-accessibility/webkit/iteration-${number}/result.json`);
    const result = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, 'utf8')) : null;
    const nativeFile = path.join(path.dirname(file), 'native-trace.json');
    const nativeTrace = fs.existsSync(nativeFile) ? JSON.parse(fs.readFileSync(nativeFile, 'utf8')) : null;
    return { number, state: !started ? 'unexecuted' : cases[index]?.[2] === '0' ? 'passed' : 'failed-or-interrupted',
      exitStatus: cases[index] ? Number(cases[index][2]) : null,
      checks: result?.checks?.length ?? null, browserVersion: result?.browserVersion ?? null,
      resultValid: result?.ok === true && result?.pageErrors?.length === 0 && result?.cleanup?.remainingRows === 0,
      failure: result?.failure ?? null, nativeTrace };
  });
  const firstFailed = rounds.findIndex(round => round.state === 'failed-or-interrupted');
  if (firstFailed >= 0) assert.ok(rounds.slice(firstFailed + 1).every(round => round.state === 'unexecuted'), 'ran after failure');
  if (exitStatus === 0) {
    assert.equal(prefix.length, PREFIX.length);
    assert.ok(prefix.every(row => row[2] === '0'));
    assert.ok(rounds.every(round => round.state === 'passed' && round.checks === 7 && round.browserVersion === '26.6' && round.resultValid), 'incomplete experiment cannot pass');
    assert.ok(rounds.every(round => round.nativeTrace?.complete && !round.nativeTrace.diagnosticFailure), 'incomplete process evidence cannot pass');
    assert.equal(events.filter(row => row[1] === 'passed').length, ROUNDS);
  }
  const capabilityFile = path.join(output, 'native-trace-capability/capability.json');
  const nativeCapability = fs.existsSync(capabilityFile) ? JSON.parse(fs.readFileSync(capabilityFile, 'utf8')) : null;
  const incompleteTrace = rounds.some(round => round.state !== 'unexecuted' && !round.nativeTrace?.complete);
  return { exitStatus, outcome: exitStatus === 0 ? 'not-reproduced-in-bounded-experiment'
    : events.some(row => row[1] === 'budget-exhausted') ? 'incomplete-budget-exhausted'
    : nativeCapability?.ok === false || incompleteTrace ? 'diagnostic-tool-failed-or-interrupted' : 'failed-or-interrupted',
    nativeCapability,
    prefix: prefix.map(([id, , status]) => ({ id, exitStatus: Number(status) })), rounds,
    originalIssueResolved: false };
}

async function runCommand(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, options);
    let interrupted = null;
    const interrupt = signal => { interrupted = signal === 'SIGINT' ? 130 : 143; child.kill(signal); };
    const onInt = () => interrupt('SIGINT');
    const onTerm = () => interrupt('SIGTERM');
    process.on('SIGINT', onInt);
    process.on('SIGTERM', onTerm);
    const detach = () => { process.off('SIGINT', onInt); process.off('SIGTERM', onTerm); };
    child.once('error', error => { detach(); reject(error); });
    child.once('close', code => { detach(); resolve(interrupted ?? code ?? 1); });
  });
}

// The callback launches an external command. Always remove only the directory
// created here, including when setup, the command or evidence validation fails.
async function withOwnedSnapshot(parent, action, remove = fs.rmSync) {
  let interrupted = null;
  const onInt = () => { interrupted = 130; };
  const onTerm = () => { interrupted = 143; };
  process.on('SIGINT', onInt);
  process.on('SIGTERM', onTerm);
  // Sync archive, extraction and filesystem calls defer JavaScript signal
  // callbacks. Drain those callbacks before another phase or listener removal.
  const settleSignals = () => new Promise(resolve => setImmediate(resolve));
  const checkpoint = async () => {
    await settleSignals();
    if (interrupted) throw Object.assign(new Error('diagnostic interrupted'), { exitStatus: interrupted });
  };
  try {
    const snapshot = fs.mkdtempSync(path.join(parent, 'rcc-a11y-original-'));
    let status = 1;
    let failure = null;
    let cleanupError = null;
    try { await checkpoint(); status = await action(snapshot, checkpoint); }
    catch (error) { failure = error; status = error.exitStatus || 1; }
    await settleSignals();
    if (interrupted && status === 0) status = interrupted;
    try { remove(snapshot, { recursive: true, force: true }); } catch (error) { cleanupError = error; }
    await settleSignals();
    if (interrupted && status === 0) status = interrupted;
    const removed = !fs.existsSync(snapshot);
    return { status: status === 0 && (cleanupError || !removed) ? 1 : status, failure, cleanupError, removed };
  } finally {
    process.off('SIGINT', onInt);
    process.off('SIGTERM', onTerm);
  }
}

async function main(artifacts) {
  assert.equal(process.platform, 'linux', 'native diagnostic requires Linux');
  assert.equal(process.arch, 'x64', 'native diagnostic requires AMD64');
  for (const key of ['RCC_E2E_TIMEOUT_SECONDS', 'RCC_E2E_CASES', 'RCC_E2E_SUITE', 'RCC_E2E_ENGINE', 'RCC_E2E_ENGINES', 'RCC_BROWSER_EXECUTABLE', 'RCC_PLAYWRIGHT_MODULE']) {
    assert.ok(!process.env[key], `unexpected diagnostic override: ${key}`);
  }
  const repo = path.resolve(__dirname, '..');
  assert.equal(execFileSync('git', ['rev-parse', `${BASE}^{commit}`], { cwd: repo, encoding: 'utf8' }).trim(), BASE);
  const output = path.resolve(artifacts);
  fs.mkdirSync(output, { recursive: true });
  assert.equal(fs.readdirSync(output).length, 0, 'artifact directory must be empty');
  const browserOutput = path.join(output, 'browser');
  let summary = { ...summarize(browserOutput, 1), outcome: 'setup-failed' };
  const result = await withOwnedSnapshot(os.tmpdir(), async (snapshot, checkpoint) => {
    const archive = path.join(snapshot, 'source.tar');
    const fd = fs.openSync(archive, 'w');
    try { execFileSync('git', ['archive', '--format=tar', BASE], { cwd: repo, stdio: ['ignore', fd, 'pipe'] }); }
    finally { fs.closeSync(fd); }
    await checkpoint();
    const source = path.join(snapshot, 'source');
    fs.mkdirSync(source);
    execFileSync('tar', ['-xf', archive, '-C', source]);
    await checkpoint();
    fs.unlinkSync(archive);
    verifySource(source);
    const traceSource = fs.readFileSync(path.join(__dirname, 'diagnose-a11y-native-trace.cjs'));
    fs.writeFileSync(path.join(source, 'scripts/.a11y-native-trace.cjs'), traceSource);
    const original = fs.readFileSync(path.join(source, 'scripts/browser-acceptance.sh'), 'utf8');
    const generated = generateRunner(original);
    const runner = path.join(source, 'scripts/.a11y-webkit-repeat.sh');
    fs.writeFileSync(runner, generated);
    execFileSync('bash', ['-n', runner]);
    json(path.join(output, 'provenance.json'), {
      base: BASE, baseTree: execFileSync('git', ['rev-parse', `${BASE}^{tree}`], { cwd: repo, encoding: 'utf8' }).trim(),
      driverCommit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(),
      driverSHA256: sha256(fs.readFileSync(__filename)), originalSHA256: HASHES,
      nativeTraceSHA256: sha256(traceSource),
      generatedRunnerSHA256: sha256(generated), maxRounds: ROUNDS, prefix: PREFIX,
      caseTimeoutSeconds: 420, serviceLifetimeSeconds: 2400, cleanupReserveSeconds: 60,
    });
    fs.writeFileSync(path.join(output, 'generated-runner.sh'), generated);
    await checkpoint();
    const log = fs.openSync(path.join(output, 'runner.log'), 'w');
    let status;
    try {
      status = await runCommand('bash', [runner], { cwd: source, stdio: ['ignore', log, log], env: {
        ...process.env, DEBUG: '', RCC_E2E_ARTIFACTS: browserOutput, RCC_E2E_SUITE: 'all',
        RCC_E2E_ENGINES: 'chromium,firefox,webkit', RCC_E2E_CASES: [...PREFIX, CASE].join(','),
      } });
    } finally { fs.closeSync(log); }
    try {
      verifySource(source);
      summary = summarize(browserOutput, status);
    } catch (error) {
      error.exitStatus = status || 1;
      throw error;
    }
    return status;
  });
  summary.snapshotRemoved = result.removed;
  summary.cleanupError = result.cleanupError?.message ?? null;
  summary.driverError = result.failure?.message ?? null;
  summary.driverExitStatus = result.status;
  json(path.join(output, 'summary.json'), summary);
  console.log(JSON.stringify({ outcome: summary.outcome, exitStatus: result.status, artifacts: output, snapshotRemoved: result.removed }));
  return result.status;
}

module.exports = { BASE, ROUNDS, PREFIX, CASE, HASHES, WEBKIT_LOOP, IDENTITY_CHECK, TRACE_SETUP, TRACE_SELECT, ORIGINAL_COMMAND, TRACED_COMMAND, sha256, verifySource, generateRunner, summarize, runCommand, withOwnedSnapshot };
if (require.main === module) {
  if (process.argv.length !== 3) { console.error('usage: node diagnose-a11y-webkit-repeat.cjs <empty-artifact-directory>'); process.exitCode = 2; }
  else main(process.argv[2]).then(status => { process.exitCode = status; }).catch(error => { console.error(error.message); process.exitCode = 1; });
}
