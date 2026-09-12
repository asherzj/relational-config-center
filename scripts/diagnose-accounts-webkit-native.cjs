// Temporary #110 probe. Remove with the diagnostic CI job after the renderer
// crash has an evidenced resolution. Never inspect process arguments or env.
const { appendFileSync, readFileSync } = require('node:fs');
const { execFileSync } = require('node:child_process');
const { join } = require('node:path');

const output = process.argv[2];
if (!output) throw new Error('Expected an output JSONL path');
const started = Date.now();
const maximumSamples = 900;
let samples = 0;
const read = path => {
  try { return readFileSync(path, 'utf8').trim(); }
  catch (error) { return { unavailable: error.code || error.name }; }
};
const cgroup = read('/proc/self/cgroup');
const unifiedGroup = typeof cgroup === 'string' ? cgroup.split('\n').find(line => line.startsWith('0::'))?.slice(3) : null;
const cgroupRoots = [...new Set(['/sys/fs/cgroup', ...(unifiedGroup ? [join('/sys/fs/cgroup', unifiedGroup).replace(/\/$/, '')] : [])])];

function sample() {
  const memory = read('/proc/meminfo');
  const entry = {
    at: new Date().toISOString(), elapsedMs: Date.now() - started,
    memory: typeof memory === 'string'
      ? memory.split('\n').filter(line => /^(MemTotal|MemAvailable|MemFree|SwapTotal|SwapFree):/.test(line))
      : memory,
    cgroups: cgroupRoots.map(path => ({ path, events: read(join(path, 'memory.events')), current: read(join(path, 'memory.current')), max: read(join(path, 'memory.max')) })),
  };
  try {
    // comm contains the executable name, not its command line or credentials.
    entry.processes = execFileSync('ps', ['-eo', 'pid=,ppid=,comm=,rss='], { encoding: 'utf8', timeout: 500, maxBuffer: 1024 * 1024 })
      .trim().split('\n');
  } catch (error) { entry.processes = { unavailable: error.code || error.name }; }
  try { appendFileSync(output, `${JSON.stringify(entry)}\n`); }
  catch (error) { process.stderr.write(`Native diagnostic write unavailable: ${error.code || error.name}\n`); }
  samples++;
  if (samples >= maximumSamples) clearInterval(timer);
}
const timer = setInterval(sample, 1000);
sample();
