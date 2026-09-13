// Temporary #113/D10 single-block CR/LF minimization. Delete before final issue delivery.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { execFileSync, spawn } = require('node:child_process');

const BASE = 'e4d801f5127db44f4d5fdb457dda046208a43341';
const ROUNDS = 12;
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

const sha = sha256;
function unique(source, text) {
  assert.equal(source.split(text).length, 2, `nonunique/missing source anchor: ${text}`);
  return source.indexOf(text);
}
function classify(failure, phase) {
  if (!failure) return 'not-reproduced-in-bounded-experiment';
  if (phase === 'setup') return 'incomplete-setup';
  if (phase === 'round-start-observation' || phase === 'round-result-observation') return 'incomplete-observation';
  return phase === 'original-LF-355' && failure.name === 'TimeoutError'
    && /locator\.click: Timeout 15000ms exceeded/.test(failure.message)
    && /element is not stable/.test(failure.message)
    ? 'exact-original-LF-notstable-symptom' : 'other-failure';
}
function generate(original, rounds) {
  assert.equal(sha(original), HASHES['web/e2e/browser-accessibility.cjs']);
  assert.ok(Number.isInteger(rounds) && rounds >= 1 && rounds <= 12);
  const prefix = unique(original, '    // Rule editing, native browser history and modal ownership.');
  const start = unique(original, '    // Narrow drawer, raw CR boundary and a real MySQL validation failure.');
  const end = unique(original, '    // API fault injection happens after the release draft is durably created.');
  const after = unique(original, "    assert.equal(requests.some((entry) => /^\\/api\\/v1\\/tables");
  const raw = original.slice(start, end);
  const removedStart = unique(raw, "    const drawerBody = page.locator('.drawer-body');");
  const removedEnd = unique(raw, "    await button('查看 Change Set').click();") - 1; // Retain the original blank separator.
  const removed = raw.slice(removedStart, removedEnd);
  assert.equal(sha(removed), 'a0abd1efca501e633cd7a9911ea9e7b704b3a5bbd76bbd2084e4ab692672b94c');
  const reduced = raw.slice(0, removedStart) + raw.slice(removedEnd);
  let body = reduced;
  const lf = "    await button('note 申请值：转换为 LF 再编辑').click();";
  const leave = "    await button('放弃修改并离开').click();";
  unique(body, lf); unique(body, leave);
  body = body.replace(lf, `    phase = 'original-LF-355';\n    const lfStarted = Date.now();\n${lf}\n    currentRound.lfMs = Date.now() - lfStarted;\n    currentRound.lfPassed = true;\n    phase = 'post-LF-assertions';`)
    .replace(leave, `    phase = 'original-leave-360';\n${leave}\n    phase = 'post-leave-assertions';`);
  const observations = `\n  const compactStarted = Date.now();\n  const roundResults = [];\n  const maxRounds = ${rounds};\n  let phase = 'setup';\n  let currentRound = null;\n  const classify = ${classify.toString()};\n`;
  let script = original.slice(0, prefix) + `    for (let round = 1; round <= maxRounds; round++) {\n    phase = 'round-start-observation';\n    currentRound = { round, state: 'started', started: Date.now(), checksBefore: checks.length };\n    roundResults.push(currentRound);\n    await fs.appendFile(rootOutput + '/round-events.jsonl', JSON.stringify({ round, state: 'started' }) + '\\n');\n    output = rootOutput + '/round-' + round;\n    await fs.mkdir(output, { recursive: true });\n    phase = 'CR-path';\n` + body + `    currentRound.elapsedMs = Date.now() - currentRound.started;\n    currentRound.checks = checks.length - currentRound.checksBefore;\n    assert.equal(currentRound.checks, 3);\n    assert.deepEqual(pageErrors, []);\n    phase = 'round-result-observation';\n    await fs.appendFile(rootOutput + '/round-events.jsonl', JSON.stringify({ ...currentRound, state: 'passed' }) + '\\n');\n    currentRound.state = 'passed';\n    phase = 'round-complete';\n    console.log('COMPACT_ROUND', JSON.stringify(currentRound));\n    }\n    phase = 'final-assertions';\n` + original.slice(after);
  script = script.replace('const output = process.env.RCC_E2E_OUTPUT;', 'const rootOutput = process.env.RCC_E2E_OUTPUT;\nlet output = rootOutput;')
    .replace('  const checks = [];', observations + '  const checks = [];')
    .replace('  } finally {\n    if (browser)', `  } finally {\n    if (currentRound && currentRound.state === 'started') {\n      currentRound.state = 'failed';\n      currentRound.elapsedMs = Date.now() - currentRound.started;\n      currentRound.checks = checks.length - currentRound.checksBefore;\n    }\n    output = rootOutput;\n    if (browser)`)
    .replace('      ok: failure === null,', `      ok: failure === null,\n      compact: { originalIssueResolved: false, originalBase: '${BASE}', phase,\n        outcome: classify(failure, phase), elapsedMs: Date.now() - compactStarted,\n        maxRounds, rounds: Array.from({ length: maxRounds }, (_, i) => roundResults[i] || { round: i + 1, state: 'unexecuted' }),\n        platform: process.platform, arch: process.arch, node: process.version,\n        playwright: require(require('node:path').join(process.env.RCC_PLAYWRIGHT_MODULE, 'package.json')).version,\n        lifecycleBoundary: 'one browser and two accounts; fresh page per round; shared isolated services',\n      },`);
  script = script.replace('    browserVersion = browser.version();', "    browserVersion = browser.version();\n    assert.equal(browserVersion, '26.6');");
  const coverageStart = unique(script, '      coverageBoundaries: {');
  const coverageEnd = script.indexOf('      failure,', coverageStart);
  script = script.slice(0, coverageStart) + `      coverageBoundaries: {\n        browser: 'Playwright WebKit; actual platform recorded; not installed Safari',\n        clipboard: 'original synthetic paste Event with DataTransfer, not OS clipboard',\n        omitted: 'original checks 1, 2, 3, 7; ordinary suite unchanged; not replacement acceptance',\n      },\n` + script.slice(coverageEnd);
  assert.ok(!script.includes('page.route('));
  assert.ok(!script.includes('force:'));
  assert.ok(script.includes(lf));
  // Strip only our two host-side phase insertions and prove the retained body is byte-identical.
  assert.equal(body.replace(`    phase = 'original-LF-355';\n    const lfStarted = Date.now();\n`, '')
    .replace(`\n    currentRound.lfMs = Date.now() - lfStarted;\n    currentRound.lfPassed = true;\n    phase = 'post-LF-assertions';`, '')
    .replace(`    phase = 'original-leave-360';\n`, '')
    .replace(`\n    phase = 'post-leave-assertions';`, ''), reduced);
  const line = offset => original.slice(0, offset).split('\n').length;
  return { script, provenance: { base: BASE, originalHashes: HASHES, generatedSha256: sha(script),
    retainedBusiness: { spans: [{ firstLine: line(start), lastLine: line(start + removedStart)-1 },
      { firstLine: line(start + removedEnd), lastLine: line(end)-1 }], sha256: sha(reduced), byteIdentityExcludingHostPhaseInsertions: true },
    deleted: [{ firstLine: line(prefix), lastLine: line(start)-1, sha256: sha(original.slice(prefix,start)) },
      { firstLine: line(start + removedStart), lastLine: line(start + removedEnd)-1, sha256: sha(removed) },
      { firstLine: line(end), lastLine: line(after)-1, sha256: sha(original.slice(end,after)) }],
    targetOriginalLine: line(unique(original, lf)), leaveOriginalLine: line(original.indexOf(leave,start)), maxRounds: rounds,
  }};
}
const IDENTITY_CHECK = `node - "$repo_root" "$artifact_root" <<'RCC_D09_IDENTITY'
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
fs.writeFileSync(path.join(output, 'browser-identity.json'), JSON.stringify({
  platform: process.platform, arch: process.arch, node: process.version,
  playwright: pkg.version, core: core.version, revision: webkit.revision,
  advertisedBrowserVersion: webkit.browserVersion, executable,
}, null, 2) + '\\n');
RCC_D09_IDENTITY
`;

const ORIGINAL_INVOCATION = '  run_browser_suite "browser-accessibility ($browser_engine)" "$repo_root/web/e2e/browser-accessibility.cjs" "$artifact_root/browser-accessibility/$browser_engine" 420 "$browser_engine"\ndone\nfi';
const COMPACT_INVOCATION = `  if (( SECONDS + 420 + 60 > 2400 )); then
    printf 'incomplete-budget-exhausted\\n' > "$artifact_root/compact-budget.txt"
    exit 125
  fi
${ORIGINAL_INVOCATION.replace('web/e2e/browser-accessibility.cjs', 'web/e2e/compact-lf.cjs')}`;

function generateRunner(original) {
  assert.equal(sha256(original), HASHES['scripts/browser-acceptance.sh'], 'original runner mismatch');
  unique(original, ORIGINAL_INVOCATION);
  const build = "printf 'Building the Web preview artifact...\\n'";
  unique(original, build);
  return original.replace(ORIGINAL_INVOCATION, COMPACT_INVOCATION).replace(build, `${IDENTITY_CHECK}\n${build}`);
}

function summarize(output, exitStatus, rounds = ROUNDS) {
  assert.ok(Number.isInteger(rounds) && rounds >= 1 && rounds <= ROUNDS);
  const resultFile = path.join(output, 'browser-accessibility/webkit/result.json');
  const result = fs.existsSync(resultFile) ? JSON.parse(fs.readFileSync(resultFile, 'utf8')) : null;
  const eventsFile = path.join(path.dirname(resultFile), 'round-events.jsonl');
  const events = fs.existsSync(eventsFile) ? fs.readFileSync(eventsFile, 'utf8').trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : [];
  const progress = Array.from({ length: rounds }, (_, i) => {
    const recorded = result?.compact?.rounds?.[i];
    const started = events.find(event => event.round === i + 1 && event.state === 'started');
    const passed = events.find(event => event.round === i + 1 && event.state === 'passed');
    return recorded || passed || { round: i + 1, state: started ? 'interrupted' : 'unexecuted' };
  });
  const firstFailed = progress.findIndex(round => ['failed', 'interrupted'].includes(round.state));
  if (firstFailed >= 0) assert.ok(progress.slice(firstFailed + 1).every(round => round.state === 'unexecuted'), 'ran after failure');
  const serviceLog = path.join(output, 'run.txt');
  const serviceCleanup = fs.existsSync(serviceLog) && /^cleanup verified: true$/m.test(fs.readFileSync(serviceLog, 'utf8'));
  const valid = result?.ok === true && result?.pageErrors?.length === 0 && result?.cleanup?.remainingRows === 0
    && result?.browserVersion === '26.6' && result?.checks?.length === rounds * 3
    && progress.every((round, i) => round.round === i + 1 && round.state === 'passed' && round.checks === 3 && round.lfPassed === true)
    && serviceCleanup;
  if (exitStatus === 0) assert.ok(valid, 'incomplete experiment cannot pass');
  let outcome = exitStatus === 0 ? 'not-reproduced-in-bounded-experiment'
    : result?.failure ? classify(result.failure, result.compact?.phase) : 'incomplete-experiment';
  if (!serviceCleanup || (result && result.cleanup?.remainingRows !== 0)) outcome = 'incomplete-cleanup';
  if (fs.existsSync(path.join(output, 'compact-budget.txt'))) outcome = 'incomplete-budget-exhausted';
  return { exitStatus, outcome, rounds: progress, checks: result?.checks?.length ?? 0,
    phase: result?.compact?.phase ?? null, failure: result?.failure ?? null,
    serviceCleanup, originalIssueResolved: false };
}

function artifactDirectory(value) {
  assert.equal(typeof value, 'string', 'artifact directory is required');
  assert.ok(value.trim() && !/[\0\r\n]/.test(value), 'invalid artifact directory');
  const output = path.resolve(value);
  assert.notEqual(output, path.parse(output).root, 'artifact directory cannot be filesystem root');
  if (fs.existsSync(output)) {
    assert.ok(fs.lstatSync(output).isDirectory() && !fs.lstatSync(output).isSymbolicLink(), 'artifact directory must be a real directory');
    assert.equal(fs.readdirSync(output).length, 0, 'artifact directory must be empty');
  }
  fs.mkdirSync(output, { recursive: true });
  return output;
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
  assert.equal(process.platform, 'linux', 'CI feedback requires Linux');
  assert.equal(process.arch, 'x64', 'CI feedback requires AMD64');
  for (const key of ['RCC_E2E_TIMEOUT_SECONDS', 'RCC_E2E_CASES', 'RCC_E2E_SUITE', 'RCC_E2E_ENGINE', 'RCC_E2E_ENGINES', 'RCC_BROWSER_EXECUTABLE', 'RCC_PLAYWRIGHT_MODULE', 'DEBUG']) {
    assert.ok(!process.env[key], `unexpected diagnostic override: ${key}`);
  }
  const repo = path.resolve(__dirname, '..');
  assert.equal(execFileSync('git', ['rev-parse', `${BASE}^{commit}`], { cwd: repo, encoding: 'utf8' }).trim(), BASE);
  const output = artifactDirectory(artifacts);
  const browserOutput = path.join(output, 'browser');
  let summary = { ...summarize(browserOutput, 1), outcome: 'incomplete-setup' };
  const result = await withOwnedSnapshot(os.tmpdir(), async (snapshot, checkpoint) => {
    const archive = path.join(snapshot, 'source.tar');
    const fd = fs.openSync(archive, 'w');
    try { execFileSync('git', ['archive', '--format=tar', BASE], { cwd: repo, stdio: ['ignore', fd, 'pipe'] }); }
    finally { fs.closeSync(fd); }
    await checkpoint();
    const source = path.join(snapshot, 'source'); fs.mkdirSync(source);
    execFileSync('tar', ['-xf', archive, '-C', source]); fs.unlinkSync(archive);
    await checkpoint();
    verifySource(source);
    const compact = generate(fs.readFileSync(path.join(source, 'web/e2e/browser-accessibility.cjs'), 'utf8'), ROUNDS);
    const test = path.join(source, 'web/e2e/compact-lf.cjs');
    fs.writeFileSync(test, compact.script);
    const generated = generateRunner(fs.readFileSync(path.join(source, 'scripts/browser-acceptance.sh'), 'utf8'));
    const runner = path.join(source, 'scripts/.a11y-webkit-repeat.sh');
    fs.writeFileSync(runner, generated);
    execFileSync('bash', ['-n', runner]); execFileSync(process.execPath, ['--check', test]);
    json(path.join(output, 'provenance.json'), { ...compact.provenance,
      baseTree: execFileSync('git', ['rev-parse', `${BASE}^{tree}`], { cwd: repo, encoding: 'utf8' }).trim(),
      driverCommit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(),
      driverSHA256: sha256(fs.readFileSync(__filename)), generatedRunnerSHA256: sha256(generated),
      caseTimeoutSeconds: 420, serviceLifetimeSeconds: 2400, cleanupReserveSeconds: 60,
      scope: 'D10 removes only the drawer scroll/geometry block from the known-red D09 path; other original CR/LF steps, state reuse and deadlines unchanged. Three retained checks per round; bounded green does not prove the deleted block necessary or irrelevant.',
      earlierCompactDifference: 'Earlier local compact omitted intermediate scrolling/focus steps and launched fresh browsers. D09 retained the contiguous original segment and reused one browser; D10 changes only its drawer-scroll block and retains browser reuse.',
    });
    fs.writeFileSync(path.join(output, 'generated-runner.sh'), generated);
    fs.writeFileSync(path.join(output, 'generated-compact.cjs'), compact.script);
    await checkpoint();
    const log = fs.openSync(path.join(output, 'runner.log'), 'w');
    let status;
    try {
      status = await runCommand('bash', [runner], { cwd: source, stdio: ['ignore', log, log], env: {
        ...process.env, RCC_E2E_ARTIFACTS: browserOutput, RCC_E2E_SUITE: 'browser-accessibility', RCC_E2E_ENGINES: 'webkit',
      } });
    } finally { fs.closeSync(log); }
    try { verifySource(source); summary = summarize(browserOutput, status); }
    catch (error) { error.exitStatus = status || 1; throw error; }
    return status;
  });
  summary.snapshotRemoved = result.removed;
  summary.cleanupError = result.cleanupError?.message ?? null;
  summary.driverError = result.failure?.message ?? null;
  summary.driverExitStatus = result.status;
  if (result.status !== 0 && summary.outcome === 'not-reproduced-in-bounded-experiment') summary.outcome = 'incomplete-driver-or-cleanup';
  json(path.join(output, 'summary.json'), summary);
  console.log(JSON.stringify({ outcome: summary.outcome, exitStatus: result.status, artifacts: output, snapshotRemoved: result.removed }));
  return result.status;
}

module.exports = { BASE, ROUNDS, HASHES, IDENTITY_CHECK, ORIGINAL_INVOCATION, COMPACT_INVOCATION, sha256, verifySource, generate, classify, generateRunner, summarize, artifactDirectory, runCommand, withOwnedSnapshot };
if (require.main === module) {
  if (process.argv.length !== 3) { console.error('usage: node diagnose-a11y-webkit-repeat.cjs <empty-artifact-directory>'); process.exitCode = 2; }
  else main(process.argv[2]).then(status => { process.exitCode = status; }).catch(error => { console.error(error.message); process.exitCode = 1; });
}
