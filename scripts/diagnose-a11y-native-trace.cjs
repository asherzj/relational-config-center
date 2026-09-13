// Temporary #113/D06–D07 process observer. Delete before final issue delivery.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { spawn, execFileSync } = require('node:child_process');

// Stack frames contain only symbols/addresses. No argument values, environment,
// I/O, network, locals, register/memory dumps or core contents.
const TRACE_ARGS = ['-f', '-ttt', '--decode-pids=comm', '--seccomp-bpf', '-k', '--stack-trace-frame-limit=12',
  '-e', 'trace=clone,clone3,fork,vfork,exit,exit_group,wait4,waitid,kill,tgkill', '-e', 'signal=all'];
const writeJSON = (file, value) => fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);

function inspectTrace(text, status) {
  const rootPID = text.match(/^(\d+)(?:<[^>]*>)?\s+\d+\.\d+\s/m)?.[1] ?? null;
  const rootExit = rootPID && text.match(new RegExp(`^${rootPID}(?:<[^>]*>)?\\s+\\d+\\.\\d+\\s+\\+\\+\\+ exited with (\\d+) \\+\\+\\+$`, 'm'));
  // An absent root exit includes tracer loss, unsupported tracing, and signal
  // termination. Keep the raw evidence; do not call this an ordinary case exit.
  return { rootPID: rootPID ? Number(rootPID) : null,
    rootExitStatus: rootExit ? Number(rootExit[1]) : null,
    complete: Boolean(rootExit && Number(rootExit[1]) === status) };
}

async function traceCommand(output, command, args) {
  fs.mkdirSync(output, { recursive: true });
  const traceFile = path.join(output, 'native-signals.log');
  const startedAt = new Date().toISOString();
  let interrupted = null;
  let launchError = null;
  let tracerPID = null;
  const exit = await new Promise(resolve => {
    const child = spawn('strace', [...TRACE_ARGS, '-o', traceFile, command, ...args], { stdio: 'inherit' });
    tracerPID = child.pid ?? null;
    const interrupt = signal => { interrupted = signal === 'SIGINT' ? 130 : 143; child.kill(signal); };
    const onInt = () => interrupt('SIGINT');
    const onTerm = () => interrupt('SIGTERM');
    process.on('SIGINT', onInt);
    process.on('SIGTERM', onTerm);
    child.once('error', error => { launchError = error.message; });
    child.once('close', (code, signal) => {
      process.off('SIGINT', onInt);
      process.off('SIGTERM', onTerm);
      resolve({ code, signal });
    });
  });
  const closedAt = new Date().toISOString();
  const status = interrupted ?? (launchError ? 70 : exit.code ?? 1);
  const observation = inspectTrace(fs.existsSync(traceFile) ? fs.readFileSync(traceFile, 'utf8') : '', status);
  const diagnosticFailure = launchError || (!observation.complete ? 'trace did not retain a matching normal root exit' : null);
  writeJSON(path.join(output, 'native-trace.json'), {
    startedAt, closedAt, tracerPID, tracerExit: exit, interrupted,
    ...observation, diagnosticFailure,
    // Preserve an already-failed command; missing evidence can only turn a
    // successful command into failure, never the reverse.
    exitStatus: status || (diagnosticFailure ? 70 : 0),
  });
  return status || (diagnosticFailure ? 70 : 0);
}

async function checkCapability(output) {
  const fields = () => Object.fromEntries(fs.readFileSync('/proc/self/status', 'utf8').split('\n')
    .filter(line => /^(TracerPid|Seccomp|Seccomp_filters):/.test(line)).map(line => line.trim().split(/:\s+/)));
  const before = fields();
  const version = execFileSync('strace', ['--version'], { encoding: 'utf8' }).split('\n')[0];
  const status = await traceCommand(output, process.execPath, ['-e',
    `const fs = require('node:fs'); fs.writeFileSync(process.argv[1], JSON.stringify((${fields.toString()})()))`, path.join(output, 'tracee-status.json')]);
  assert.equal(status, 0, 'native process tracing probe failed');
  const after = JSON.parse(fs.readFileSync(path.join(output, 'tracee-status.json'), 'utf8'));
  assert.ok(Number(after.TracerPid) > 0, 'probe was not traced');
  assert.equal(after.Seccomp, '2', 'seccomp filtering unavailable');
  assert.ok(Number(after.Seccomp_filters) > Number(before.Seccomp_filters), 'strace seccomp filter was not installed');
  writeJSON(path.join(output, 'capability.json'), { ok: true, version, platform: process.platform, arch: process.arch, before, after });
}

module.exports = { TRACE_ARGS, inspectTrace, traceCommand, checkCapability };
if (require.main === module) {
  const [output, command, ...args] = process.argv.slice(2);
  const operation = command === '--check' ? checkCapability(output).then(() => 0)
    : output && command ? traceCommand(output, command, args) : Promise.reject(new Error('usage: native-trace <output> <command> [args...] | --check'));
  operation.then(status => { process.exitCode = status; }).catch(error => {
    if (command === '--check') {
      fs.mkdirSync(output, { recursive: true });
      writeJSON(path.join(output, 'capability.json'), { ok: false, error: error.message });
    }
    console.error(`[DEBUG-113-D06] ${error.message}`);
    process.exitCode = 70;
  });
}
