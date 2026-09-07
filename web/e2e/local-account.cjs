// Shared support for the standalone management acceptance scripts.
// From web/: RCC_WEB_URL=http://127.0.0.1:15173 node e2e/unsaved-changes.cjs
//            RCC_WEB_URL=http://127.0.0.1:15173 node e2e/rule-clarity.cjs
// Optional: RCC_BROWSER_EXECUTABLE, RCC_PLAYWRIGHT_MODULE and RCC_E2E_OUTPUT.
// Run against an isolated Admin/Web/MySQL fixture with the notification rules,
// stage1_acceptance_items, and assigned Deprecated stage1_mutation_v1 loaded.
// Each run registers a random disposable account through the public auth API.
// Accounts have no public delete endpoint: remove them by tearing down that
// isolated database, never by deleting accounts from a shared deployment.
const assert = require('node:assert/strict');
const { randomBytes } = require('node:crypto');

function browserOptions() {
  if (process.env.RCC_E2E_ENGINE && !process.env.RCC_BROWSER_EXECUTABLE) return { headless: true };
  return process.env.RCC_BROWSER_EXECUTABLE
    ? { executablePath: process.env.RCC_BROWSER_EXECUTABLE, headless: true }
    : { channel: 'chrome', headless: true };
}

async function registerFixtureAccount(context, baseURL) {
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

  return {
    accountID: identity.account.id,
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

module.exports = { browserOptions, registerFixtureAccount, authenticatedDelete, authenticatedRequest };
