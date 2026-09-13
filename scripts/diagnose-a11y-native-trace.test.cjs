// Linux process controls validate D06 instrumentation, not the #113 browser bug.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn, spawnSync } = require('node:child_process');
const { inspectTrace, checkCapability } = require('./diagnose-a11y-native-trace.cjs');
const helper = path.join(__dirname, 'diagnose-a11y-native-trace.cjs');
const supervisor = path.join(__dirname, 'run-with-timeout.cjs');
const linux = { skip: process.platform !== 'linux' };

function fixture(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rcc-a11y-trace-test-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function invoke(directory, source, env = process.env) {
  const file = path.join(directory, 'control.py');
  fs.writeFileSync(file, source);
  return spawnSync(process.execPath, [helper, directory, 'python3', file, 'RCC_D06_ARGUMENT_CANARY'], {
    env: { ...env, RCC_D06_SECRET: 'RCC_D06_ENV_CANARY' }, encoding: 'utf8', timeout: 10000,
  });
}
const metadata = directory => JSON.parse(fs.readFileSync(path.join(directory, 'native-trace.json'), 'utf8'));

test('trace root completeness cannot be inferred from a different child exit', () => {
  assert.equal(inspectTrace('100<parent> 123.123 clone() = 101\n101<child> 123.124 +++ exited with 0 +++\n', 0).complete, false);
  assert.equal(inspectTrace('', 0).complete, false);
});

test('Linux preflight proves active tracing and an additional seccomp filter', linux, async t => {
  const directory = fixture(t);
  await checkCapability(directory);
  const capability = JSON.parse(fs.readFileSync(path.join(directory, 'capability.json'), 'utf8'));
  assert.ok(Number(capability.after.TracerPid) > 0);
  assert.ok(Number(capability.after.Seccomp_filters) > Number(capability.before.Seccomp_filters));
});

for (const exitStatus of [0, 7]) test(`clean process exit ${exitStatus} remains unchanged`, linux, t => {
  const directory = fixture(t);
  const result = invoke(directory, `import sys\nsys.exit(${exitStatus})\n`);
  assert.equal(result.status, exitStatus, result.stderr);
  assert.equal(metadata(directory).complete, true);
  assert.equal(metadata(directory).rootExitStatus, exitStatus);
});

test('a child fatal signal is retained even when its parent exits zero; trace excludes secrets and I/O', linux, t => {
  const directory = fixture(t);
  const result = invoke(directory, `import os, signal, resource, sys
resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
child = os.fork()
if child == 0:
    os.kill(os.getpid(), signal.SIGABRT)
else:
    _, status = os.waitpid(child, 0)
    assert os.WIFSIGNALED(status) and os.WTERMSIG(status) == signal.SIGABRT
    sys.exit(0)
`);
  assert.equal(result.status, 0, result.stderr);
  const raw = fs.readFileSync(path.join(directory, 'native-signals.log'), 'utf8');
  assert.match(raw, /SIGABRT/);
  assert.match(raw, /killed by SIGABRT/);
  assert.match(raw, /si_pid=/);
  assert.match(raw, /WTERMSIG\(s\) == SIGABRT|CLD_KILLED/);
  assert.doesNotMatch(raw, /RCC_D06_ARGUMENT_CANARY|RCC_D06_ENV_CANARY|execve(?:at)?\(|(?:read|write|recv|send)\(/);
  assert.equal(metadata(directory).complete, true);
});

for (const mode of ['missing', 'unsupported']) test(`${mode} tracer is an explicit diagnostic failure`, linux, t => {
  const directory = fixture(t);
  const bin = path.join(directory, 'bin');
  fs.mkdirSync(bin);
  if (mode === 'unsupported') fs.writeFileSync(path.join(bin, 'strace'), '#!/bin/sh\nexit 64\n', { mode: 0o755 });
  const result = invoke(directory, 'raise AssertionError("must not execute")\n', { ...process.env, PATH: bin });
  assert.equal(result.status, mode === 'unsupported' ? 64 : 70, result.stderr);
  assert.equal(metadata(directory).complete, false);
  assert.ok(metadata(directory).diagnosticFailure);
});

test('a tracer that exits zero without running the command cannot pass', linux, t => {
  const directory = fixture(t);
  fs.writeFileSync(path.join(directory, 'strace'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  const result = invoke(directory, 'raise AssertionError("must not execute")\n', { ...process.env, PATH: directory });
  assert.equal(result.status, 70, result.stderr);
  assert.equal(metadata(directory).complete, false);
});

const running = pid => {
  try { return !/\) [ZX] /.test(fs.readFileSync(`/proc/${pid}/stat`, 'utf8')); }
  catch (error) { if (error.code === 'ENOENT') return false; throw error; }
};
async function until(predicate, description) {
  const limit = Date.now() + 3000;
  while (!predicate()) {
    assert.ok(Date.now() < limit, description);
    await new Promise(resolve => setTimeout(resolve, 10));
  }
}

for (const mode of ['tracer-loss', 'supervisor-timeout', 'supervisor-SIGTERM']) {
  test(`${mode} stops the traced process tree without leaving live descendants`, linux, async t => {
    const directory = fixture(t);
    const ready = path.join(directory, 'ready.json');
    const control = path.join(directory, 'tree.py');
    fs.writeFileSync(control, `import os, time, json, resource
resource.setrlimit(resource.RLIMIT_CORE, (0, 0))
child = os.fork()
if child == 0:
    while True: time.sleep(1)
else:
    fields = dict(line.strip().split(':', 1) for line in open('/proc/self/status') if ':' in line)
    with open(${JSON.stringify(ready)}, 'w') as out:
        json.dump({'parent': os.getpid(), 'child': child, 'tracer': int(fields['TracerPid'])}, out)
    while True: time.sleep(1)
`);
    const command = [helper, directory, 'python3', control];
    const args = mode === 'tracer-loss' ? command : [supervisor, mode === 'supervisor-timeout' ? '1' : '30', process.execPath, ...command];
    const child = spawn(process.execPath, args, { stdio: 'ignore', env: { ...process.env, RCC_TIMEOUT_KILL_GRACE_MS: '1000' } });
    const exit = new Promise(resolve => child.once('close', (code, signal) => resolve({ code, signal })));
    let pids;
    t.after(() => {
      for (const pid of [child.pid, ...Object.values(pids ?? {})]) {
        if (running(pid)) { try { process.kill(pid, 'SIGKILL'); } catch {} }
      }
    });
    await until(() => fs.existsSync(ready), 'control process did not become ready');
    pids = JSON.parse(fs.readFileSync(ready, 'utf8'));
    if (mode === 'tracer-loss') process.kill(pids.tracer, 'SIGKILL');
    if (mode === 'supervisor-SIGTERM') child.kill('SIGTERM');
    const result = await exit;
    assert.equal(result.code, mode === 'tracer-loss' ? 1 : mode === 'supervisor-timeout' ? 124 : 143);
    await until(() => Object.values(pids).every(pid => !running(pid)), 'owned traced process survived cleanup');
    if (mode === 'tracer-loss') {
      assert.equal(metadata(directory).tracerExit.signal, 'SIGKILL');
      assert.equal(metadata(directory).complete, false);
      assert.ok(metadata(directory).diagnosticFailure);
    }
  });
}
