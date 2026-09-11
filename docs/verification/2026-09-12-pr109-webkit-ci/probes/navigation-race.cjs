// Diagnostic fixture, not a database acceptance suite. Run from the repository root.
// RCC_PROBE_READY=0 reproduces reload -> immediate goto; 1 waits for the same
// Managed Table control used by browser-accessibility.cjs before the second goto.
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = process.env.RCC_PROBE_REPO || process.cwd();
const { webkit } = require(path.join(root, 'web/node_modules/playwright'));
const ready = process.env.RCC_PROBE_READY === '1';
const iterations = 20;
const identity = {
  account: { id: 'ab09850e-ef9a-4317-a000-d67465416b5b', username: 'fixture.user', display_name: 'Fixture', email: 'fixture@example.invalid', email_verified: false, status: 'enabled', roles: ['ADMIN'] },
  csrf_token: 'non-secret-diagnostic-fixture', expires_at: '2099-09-07T08:00:00Z', idle_expires_at: '2099-09-07T00:30:00Z',
};
const policy = { version: '1', table_name: 'probe_items', query_policy_code: 'probe_query', mutation_policy_code: 'probe_mutation', enabled: true, creator: 'fixture', modifier: 'fixture', created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z' };
(async () => {
  const timeline = [];
  const errors = [];
  let completed = 0;
  let failure = null;
  const record = (event, data = {}) => timeline.push({ at: Date.now(), event, ...data });
  const server = http.createServer((req, res) => {
    const pathname = new URL(req.url, 'http://fixture.invalid').pathname;
    if (pathname.startsWith('/api/')) {
      record('api.request', { path: pathname });
      let body;
      if (pathname === '/api/v1/auth/session') body = identity;
      else if (pathname === '/api/v1/table-policies') body = { policies: [policy] };
      else if (pathname === '/api/v1/approval-notifications') body = { unread_count: 0, pending_count: 0 };
      else if (pathname === '/api/v1/release-orders') body = { orders: [], next_cursor: '' };
      res.writeHead(body ? 200 : 503, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(body || { error: { code: 'diagnostic_fixture', message: 'Outside navigation probe', request_id: 'fixture' } }));
      return;
    }
    const file = pathname.startsWith('/assets/') ? path.join(root, 'web/dist', pathname) : path.join(root, 'web/dist/index.html');
    res.writeHead(200, { 'Content-Type': file.endsWith('.js') ? 'application/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html' });
    res.end(fs.readFileSync(file));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  const browser = await webkit.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 390, height: 640 } });
  const managedReady = () => page.getByRole('combobox', { name: 'Managed Table', exact: true }).waitFor();
  page.on('pageerror', error => {
    const item = { name: error.name, message: error.message, stack: error.stack };
    errors.push(item);
    record('pageerror', item);
  });
  page.on('requestfailed', request => record('requestfailed', { url: request.url(), failure: request.failure() }));
  await page.exposeFunction('reportProbe', data => record('window', data));
  await page.addInitScript(() => {
    window.addEventListener('error', event => window.reportProbe({ type: 'error', message: event.message }));
    window.addEventListener('unhandledrejection', event => window.reportProbe({ type: 'unhandledrejection', message: String(event.reason) }));
    for (const type of ['pagehide', 'pageshow', 'visibilitychange', 'beforeunload']) {
      window.addEventListener(type, () => window.reportProbe({ type, visibility: document.visibilityState }));
    }
  });
  try {
    for (let i = 0; i < iterations; i++) {
      await page.goto(`${base}/configuration/managed-data`);
      await managedReady();
      record('reload.start', { iteration: i });
      await page.reload();
      record('reload.end');
      if (ready) await managedReady();
      record('second-navigation.start');
      await page.goto(`${base}/configuration/release-orders`);
      await page.getByRole('button', { name: '新建草稿', exact: true }).waitFor();
      record('second-navigation.ready');
      completed++;
    }
    assert.equal(errors.length, 0, 'No page errors during the restored workspace -> draft catalog transition');
  } catch (error) {
    failure = { name: error.name, message: error.message };
    process.exitCode = 1;
  } finally {
    await browser.close();
    server.closeAllConnections();
    await new Promise(resolve => server.close(resolve));
    const result = { ready, iterations, completed, browserVersion: browser.version(), platform: process.platform, architecture: process.arch, pageErrorCount: errors.length, nativeErrorCount: timeline.filter(item => item.event === 'window' && ['error', 'unhandledrejection'].includes(item.type)).length, errors, timeline, failure };
    fs.writeFileSync(process.env.RCC_PROBE_OUTPUT || '/tmp/rcc-navigation-race.json', JSON.stringify(result, null, 2) + '\n');
    console.log(JSON.stringify({ ready, completed, pageErrorCount: errors.length, nativeErrorCount: result.nativeErrorCount, failure }));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
