// Public Cookie/CSRF authentication on the runner's disposable deployment.
// Persist only status evidence; credentials stay in the API context's memory.
const assert = require('node:assert/strict');
const { request } = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');

(async () => {
  const base = process.env.RCC_WEB_URL;
  const origin = new URL(base).origin;
  const client = await request.newContext();
  const context = { request: client };
  try {
    const anonymous = await client.get(`${base}/api/v1/query-policies`);
    assert.equal(anonymous.status(), 401);
    await registerFixtureAccount(context, base);
    const authenticated = await client.get(`${base}/api/v1/query-policies`);
    assert.equal(authenticated.status(), 200);
    const queryPath = '/api/v1/tables/stage1_acceptance_items/query';
    const missingCSRF = await client.post(`${base}${queryPath}`, {
      headers: { Origin: origin }, data: { page_number: 1 },
    });
    assert.equal(missingCSRF.status(), 403);
    const queried = await authenticatedRequest(context, base, queryPath, {
      method: 'POST', data: { page_number: 1 },
    });
    assert.equal(queried.status(), 200);
    const identity = await (await client.get(`${base}/api/v1/auth/session`)).json();
    const logout = await client.post(`${base}/api/v1/auth/logout`, {
      headers: { Origin: origin, 'X-CSRF-Token': identity.csrf_token },
    });
    assert.equal(logout.status(), 204);
    const afterLogout = await client.get(`${base}/api/v1/query-policies`);
    assert.equal(afterLogout.status(), 401);
    console.log(JSON.stringify({ ok: true, anonymous: 401, registered: 201,
      authenticatedRead: 200, missingCSRF: 403, authenticatedQuery: 200,
      logout: 204, afterLogout: 401, credentialsPersisted: false }));
  } finally {
    await client.dispose();
  }
})().catch((error) => { console.error(error); process.exitCode = 1; });
