// Shared support for the standalone management acceptance scripts.
// From web/: RCC_WEB_URL=http://127.0.0.1:15173 node e2e/unsaved-changes.cjs
//            RCC_WEB_URL=http://127.0.0.1:15173 node e2e/rule-clarity.cjs
// Optional: RCC_BROWSER_EXECUTABLE, RCC_PLAYWRIGHT_MODULE and RCC_E2E_OUTPUT.
// Run against an isolated Admin/Web/MySQL fixture with the notification rules,
// stage1_acceptance_items, and assigned Deprecated stage1_mutation_v1 loaded.
// Each run registers a random disposable account through the public auth API.
// Accounts have no public delete endpoint: remove them by tearing down that
// isolated database, never by deleting accounts from a shared deployment.
const { execFileSync } = require('node:child_process');
const assert = require('node:assert/strict');
const { randomBytes } = require('node:crypto');

function selectedBrowser(playwright) {
  const engine = process.env.RCC_E2E_ENGINE || 'chromium';
  assert.ok(['chromium', 'firefox', 'webkit'].includes(engine), `unknown browser engine: ${engine}`);
  return playwright[engine];
}

function browserOptions() {
  if (process.env.RCC_E2E_ENGINE && !process.env.RCC_BROWSER_EXECUTABLE) return { headless: true };
  return process.env.RCC_BROWSER_EXECUTABLE
    ? { executablePath: process.env.RCC_BROWSER_EXECUTABLE, headless: true }
    : { channel: 'chrome', headless: true };
}

async function fixtureRoleAccount(context, baseURL, accountID) {
  const response = await authenticatedRequest(context, baseURL, `/api/v1/account-roles?q=${encodeURIComponent(accountID)}`);
  assert.equal(response.status(), 200, 'role fixture lookup failed');
  const result = await response.json();
  const account = result.accounts.find(candidate => candidate.id === accountID);
  assert.ok(account, `role fixture lookup omitted ${accountID}`);
  return account;
}

async function setFixtureRoles(context, baseURL, accountID, roles) {
  const current = await fixtureRoleAccount(context, baseURL, accountID);
  const response = await authenticatedRequest(context, baseURL, `/api/v1/account-roles/${accountID}`, {
    method: 'PUT',
    headers: { 'Idempotency-Key': randomBytes(16).toString('hex') },
    data: { roles, expected_version: current.version },
  });
  assert.equal(response.status(), 200, `role fixture update failed: ${await response.text()}`);
  const updated = await response.json();
  assert.deepEqual(updated.roles, roles);
  return updated;
}

async function registerFixtureAccount(context, baseURL, options = {}) {
  const origin = new URL(baseURL).origin;
  const suffix = randomBytes(10).toString('hex');
  const username = `e2e.${suffix}`;
  const email = `${username}@example.invalid`;
  const password = randomBytes(24).toString('base64url');
  const preparation = await context.request.get(`${origin}/api/v1/auth/csrf`);
  assert.equal(preparation.status(), 200, 'public CSRF preparation failed');
  const { csrf_token: csrf } = await preparation.json();
  const registration = await context.request.post(`${origin}/api/v1/auth/register`, {
    headers: { Origin: origin, 'X-CSRF-Token': csrf },
    data: { username, email, password, display_name: 'Browser acceptance' },
  });
  assert.equal(registration.status(), 201, 'disposable account registration failed');
  const identity = await registration.json();
  assert.match(identity.account.id, /^[a-f0-9-]{36}$/, 'registration returned no Account ID');
  assert.equal(identity.account.email_verified, false);
  assert.deepEqual(identity.account.roles, ['VIEWER']);
  assert.ok(process.env.RCC_ACCOUNT_MAINTAIN, 'supply the maintenance tool for this isolated writer fixture');
  execFileSync(process.env.RCC_ACCOUNT_MAINTAIN, ['grant-admin', '--id', identity.account.id], {stdio:'pipe'});
  const roles = options.roles || ['ADMIN'];
  const roleAccount = roles.length === 1 && roles[0] === 'ADMIN'
    ? await fixtureRoleAccount(context, origin, identity.account.id)
    : await setFixtureRoles(context, origin, identity.account.id, roles);

  return {
    accountID: identity.account.id,
    roles: roleAccount.roles,
    credentials: { username, email, password },
    async assertMemoryOnly(page, draftValues = []) {
      const storage = await page.evaluate(() => ({ local: { ...localStorage }, session: { ...sessionStorage }, cookie: document.cookie }));
      // Coordination persists timestamps/events only; account/session/draft data
      // must remain in memory. Unknown storage keys still fail this acceptance.
      assert.ok(Object.keys(storage.local).every((key) => ['rcc:last-activity-report', 'rcc:session-event'].includes(key)), 'unexpected persistent storage key');
      assert.deepEqual(Object.keys(storage.session), []);
      if (storage.local['rcc:last-activity-report'] !== undefined) {
        assert.ok(Number.isFinite(Number(storage.local['rcc:last-activity-report'])), 'activity coordination must contain only a timestamp');
      }
      if (storage.local['rcc:session-event'] !== undefined) {
        const event = JSON.parse(storage.local['rcc:session-event']);
        assert.deepEqual(Object.keys(event).sort(), ['at', 'nonce', 'type']);
        assert.ok(['changed', 'ended'].includes(event.type));
        assert.equal(typeof event.at, 'number');
        assert.equal(typeof event.nonce, 'number');
      }
      const cookies = await context.cookies();
      const serialized = JSON.stringify(storage);
      for (const value of [username, email, password, csrf, identity.csrf_token, identity.account.id, ...cookies.map((cookie) => cookie.value), ...draftValues]) {
        if (value) assert.equal(serialized.includes(value), false, 'account credential or draft material leaked into browser storage');
      }
    },
  };
}

async function authenticatedDelete(context, baseURL, resourcePath) {
  const origin = new URL(baseURL).origin;
  assert.ok(resourcePath.startsWith('/api/v1/query-policies/'), 'cleanup must target the disposable Query draft');
  const session = await context.request.get(`${origin}/api/v1/auth/session`);
  assert.equal(session.status(), 200, 'cleanup requires the current local account');
  const { csrf_token: csrf } = await session.json();
  return context.request.delete(`${origin}${resourcePath}`, {
    headers: { Origin: origin, 'X-CSRF-Token': csrf },
  });
}

async function authenticatedRequest(context, baseURL, resourcePath, options = {}) {
  const origin = new URL(baseURL).origin;
  assert.ok(resourcePath.startsWith('/api/v1/') && !resourcePath.startsWith('/api/v1/auth/'));
  const session = await context.request.get(`${origin}/api/v1/auth/session`);
  assert.equal(session.status(), 200, 'fixture request requires the current local account');
  const { csrf_token: csrf } = await session.json();
  return context.request.fetch(`${origin}${resourcePath}`, {
    ...options, headers: { ...options.headers, Origin: origin, 'X-CSRF-Token': csrf },
  });
}

module.exports = { browserOptions, selectedBrowser, registerFixtureAccount, setFixtureRoles, authenticatedDelete, authenticatedRequest };
