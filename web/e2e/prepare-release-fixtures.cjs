// Runner-only setup: keep historical SQL intact, then explicitly associate the
// newly loaded policies through the same public catalog contract as Web.
const assert = require('node:assert/strict');
const {execFileSync} = require('node:child_process');
const {readFileSync} = require('node:fs');
const {resolve} = require('node:path');
const {request} = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const {registerFixtureAccount, authenticatedRequest} = require('./local-account.cjs');
const {configureFixtureReleaseTemplates} = require('./release-template-fixture.cjs');

(async () => {
  assert.equal(process.env.RCC_E2E_ISOLATED, '1');
  const container = process.env.RCC_E2E_MYSQL_CONTAINER;
  assert.match(container, /^rcc-browser-[0-9]+-[0-9]+-mysql$/);
  const origin = process.env.RCC_WEB_URL;
  assert.ok(['127.0.0.1', 'localhost'].includes(new URL(origin).hostname));
  const client = await request.newContext();
  const context = {request: client};
  const api = async (path, options = {}) => {
    const response = await authenticatedRequest(context, origin, path, options);
    assert.equal(response.status(), 200, `${path}: ${await response.text()}`);
    return response.json();
  };
  try {
    // grant-admin performs its own full readiness check before any late SQL.
    await registerFixtureAccount(context, origin);
    const before = (await api('/api/v1/table-policies')).policies;
    const files = [
      'docs/verification/fixtures/stage1_acceptance.sql',
      'web/e2e/fixtures/stage1-policies.sql',
      'admin/cmd/admin/testdata/014-batch-browser.sql',
      'admin/cmd/admin/testdata/015-draft-targets-browser.sql',
      'admin/cmd/admin/testdata/016-multitable-browser.sql',
      'web/e2e/fixtures/release-rollbacks.sql',
      'web/e2e/fixtures/field-display.sql',
    ];
    for (const file of files) {
      execFileSync('docker', ['exec', '--interactive', container, 'sh', '-c',
        'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u"$MYSQL_USER" "$MYSQL_DATABASE"'],
      {input: readFileSync(resolve(__dirname, '../..', file)), timeout: 60000, stdio: ['pipe', 'pipe', 'pipe']});
    }
    const policies = (await api('/api/v1/table-policies')).policies;
    const added = policies.filter(policy => !before.some(previous => previous.table_name === policy.table_name));
    assert.ok(added.length > 0, 'late historical fixtures must load their real policies');
    const associations = await configureFixtureReleaseTemplates(context, origin, added.map(policy => policy.table_name));
    console.log(JSON.stringify({files, added:added.map(policy => policy.table_name), associations}));
  } finally {
    await client.dispose();
  }
})().catch(error => {console.error(error);process.exitCode = 1;});
