// Temporary #110 probe generator. Remove after the bounded investigation and
// before #110 delivery; never modify the formal runner.
const assert = require('node:assert/strict');
const { readFileSync, writeFileSync } = require('node:fs');
const { dirname, join, resolve } = require('node:path');

const destination = process.argv[2];
assert.ok(destination, 'Expected a temporary runner path inside scripts/');
assert.equal(dirname(resolve(destination)), __dirname, 'Diagnostic copy must stay beside the formal runner to preserve repo_root');
const source = readFileSync(join(__dirname, 'browser-acceptance.sh'), 'utf8');
const invocation = 'run_browser_suite "session, conflict and unknown recovery ($browser_engine)" "$repo_root/web/e2e/accounts.mjs" "$artifact_root/release-workflow/recovery-$browser_engine" 600 "$browser_engine"';
assert.equal(source.split(invocation).length - 1, 2, 'Original accounts invocation changed; inspect the runner before generating a diagnostic copy');
const repeatedInvocation = invocation
  .replace('recovery ($browser_engine)"', 'recovery ($browser_engine), diagnostic iteration $accounts_diagnostic_iteration/12"')
  .replace('recovery-$browser_engine"', 'recovery-$browser_engine/iteration-$accounts_diagnostic_iteration"');
const loop = `for accounts_diagnostic_iteration in {1..12}; do\n  ${repeatedInvocation}\ndone`;
writeFileSync(destination, source.split(invocation).join(loop), { mode: 0o600 });
