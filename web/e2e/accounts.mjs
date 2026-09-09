import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { chromium, firefox, webkit, request } from 'playwright';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const origin = process.env.RCC_E2E_ORIGIN || process.env.RCC_WEB_URL;
assert.ok(origin, 'Run make test-browser-acceptance to create isolated real services');
const engineName = process.env.RCC_E2E_ENGINE || 'chromium';
const engines = { chromium, firefox, webkit };
assert.ok(engines[engineName], `unknown browser engine: ${engineName}`);
const browserEngine = engines[engineName];
const profile = await mkdtemp(join(process.env.RCC_E2E_OUTPUT || tmpdir(), 'account-browser-profile-'));
const runSuffix = randomUUID().replaceAll('-', '').slice(0, 12);
const adminUsername = `browser.user.${runSuffix}`;
const adminEmail = `browser.${runSuffix}@example.invalid`;
const memberUsername = `roles.member.${runSuffix}`;
const memberEmail = `roles.${runSuffix}@example.invalid`;
const templateKey = `browser_system_${runSuffix}`;
const launchOptions = {
  ...(engineName === 'chromium' && process.env.RCC_BROWSER_EXECUTABLE
    ? { executablePath: process.env.RCC_BROWSER_EXECUTABLE, headless: true }
    : engineName === 'chromium' && !process.env.RCC_E2E_ENGINE
      ? { channel: 'chrome', headless: true }
      : { headless: true }),
  ...(engineName === 'firefox' ? { firefoxUserPrefs: { 'network.proxy.type': 0 } } : {}),
};

async function session(api) {
  const response = await api.get(`${origin}/api/v1/auth/session`);
  assert.equal(response.status(), 200);
  return response.json();
}

async function releaseState(page, state) {
  await page.getByRole('heading', { name: 'notification_templates 配置变更', exact: true }).waitFor();
  await page.getByText(`notification_templates · ${state}`, { exact: true }).waitFor();
}

async function login(api, username, password) {
  const prepared = await api.get(`${origin}/api/v1/auth/csrf`);
  assert.equal(prepared.status(), 200);
  const { csrf_token: csrfToken } = await prepared.json();
  const response = await api.post(`${origin}/api/v1/auth/login`, {
    headers: { Origin: origin, 'X-CSRF-Token': csrfToken },
    data: { username, password },
  });
  assert.equal(response.status(), 200);
  return response.json();
}

async function releaseWrite(api, path, data, key = randomUUID()) {
  const current = await session(api);
  return api.post(`${origin}${path}`, {
    headers: {
      Origin: origin,
      'X-CSRF-Token': current.csrf_token,
      'Idempotency-Key': key,
    },
    data,
  });
}

async function publishSingle({ applicant, approver, publisher, item, keyPrefix }) {
  const createdResponse = await releaseWrite(applicant, '/api/v1/release-orders', {
    title: '通知模板账号验收变更',
    table_name: 'notification_templates',
    items: [item],
  }, `${keyPrefix}-create`);
  assert.equal(createdResponse.status(), 201);
  const created = await createdResponse.json();

  const submittedResponse = await releaseWrite(applicant, `/api/v1/release-orders/${created.id}/submit`, {
    expected_version: created.version,
  }, `${keyPrefix}-submit`);
  assert.equal(submittedResponse.status(), 200);
  const submitted = await submittedResponse.json();

  const approvedResponse = await releaseWrite(approver, `/api/v1/release-orders/${created.id}/approve`, {
    expected_version: submitted.version,
    reason: 'Independent concurrent browser baseline review',
  }, `${keyPrefix}-approve`);
  assert.equal(approvedResponse.status(), 200);
  const approved = await approvedResponse.json();

  const executedResponse = await releaseWrite(publisher, `/api/v1/release-orders/${created.id}/execute`, {
    expected_version: approved.version,
  }, `${keyPrefix}-execute`);
  assert.equal(executedResponse.status(), 200);
  const executed = await executedResponse.json();
  assert.equal(executed.state, 'SUCCEEDED');
  const completed = await releaseWrite(publisher, `/api/v1/release-orders/${created.id}/complete`, { expected_version: executed.version }, `${keyPrefix}-complete`);
  assert.equal(completed.status(), 200);
  assert.equal((await completed.json()).state, 'COMPLETED');
  return executed;
}

let context;
let member;
let adminAPI;
let reviewerBrowser;
let page;
try {
  context = await browserEngine.launchPersistentContext(profile, launchOptions);
  page = await context.newPage();
  const seenRequests = [];
  const rememberRequest = browserRequest => seenRequests.push({ method: browserRequest.method(), url: browserRequest.url() });
  context.on('request', rememberRequest);
  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  await page.getByRole('link', { name: '注册新账号' }).click();
  // Wait for initial session reconciliation before entering registration data.
  await page.getByRole('button', { name: '注册并登录', exact: true }).waitFor({ state: 'visible' });
  await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.textContent === '注册并登录' && !button.disabled));
  await page.getByLabel('用户名', { exact: true }).fill(adminUsername);
  await page.getByLabel('邮箱', { exact: true }).fill(adminEmail);
  await page.getByLabel('密码', { exact: true }).fill('browser password long enough');
  const registration = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/auth/register' && response.request().method() === 'POST');
  await page.getByRole('button', { name: '注册并登录' }).click();
  assert.equal((await registration).status(), 201, 'real browser registration failed');
  await page.waitForURL(`${origin}/configuration/managed-data`);
  const identity = await page.evaluate(async () => (await fetch('/api/v1/auth/session')).json());
  assert.match(identity.account.id, /^[a-f0-9-]{36}$/);
  assert.equal(identity.account.email_verified, false);
  assert.deepEqual(identity.account.roles, ['VIEWER']);
  await page.getByRole('button', { name: '新增记录', exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '新增记录', exact: true }).isDisabled(), true);
  assert.ok(process.env.RCC_ACCOUNT_MAINTAIN, 'isolated fixture must supply the maintenance executable');
  execFileSync(process.env.RCC_ACCOUNT_MAINTAIN, ['grant-admin', '--id', identity.account.id], { stdio: 'pipe' });

  // A separate registered account remains VIEWER until the administrator uses the UI.
  member = await request.newContext();
  const prepared = await member.get(`${origin}/api/v1/auth/csrf`);
  const preparation = await prepared.json();
  const response = await member.post(`${origin}/api/v1/auth/register`, {
    headers: { Origin: origin, 'X-CSRF-Token': preparation.csrf_token },
    data: { username: memberUsername, email: memberEmail, password: 'member password long enough' },
  });
  assert.equal(response.status(), 201);
  assert.deepEqual((await response.json()).account.roles, ['VIEWER']);
  await page.goto(`${origin}/platform/account-roles`);
  const initialRolePage = await page.evaluate(async () => (await fetch('/api/v1/account-roles?q=&after=')).json());
  await page.getByLabel('检索账号', { exact: true }).fill(memberUsername);
  await page.getByRole('button', { name: '查询', exact: true }).click();
  const memberRoleButton = page.getByRole('button', { name: `管理 ${memberUsername} 的角色` });
  await memberRoleButton.waitFor();
  assert.equal(await memberRoleButton.count(), 1, 'account search must return the exact registered member once');
  if (process.env.RCC_E2E_OUTPUT) await writeFile(join(process.env.RCC_E2E_OUTPUT, 'account-role-targeting.json'), JSON.stringify({
    first_page_count: initialRolePage.accounts.length,
    first_page_has_next: Boolean(initialRolePage.next_cursor),
    target_on_first_page: initialRolePage.accounts.some(account => account.username === memberUsername),
    target_visible_after_search: true,
    target_username: memberUsername,
  }, null, 2));
  if (process.env.RCC_E2E_OUTPUT) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'account-roles-desktop.png') });
  await page.setViewportSize({ width: 390, height: 844 });
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'role catalog overflows mobile viewport');
  await memberRoleButton.click();
  await page.getByRole('dialog', { name: `管理 ${memberUsername} 的角色` }).evaluate(async element => {
    // Resizing can replace an opening transition. Its cancelled promise is not
    // a failed drawer; wait for replacement animations before checking layout.
    for (;;) {
      const active = element.getAnimations({ subtree: true }).filter(animation => animation.pending || animation.playState === 'running');
      if (active.length === 0) break;
      await Promise.all(active.map(animation => animation.finished.catch(error => {
        if (error.name !== 'AbortError') throw error;
      })));
    }
  });
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'role drawer overflows mobile viewport');
  if (process.env.RCC_E2E_OUTPUT) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'account-roles-mobile.png') });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.getByRole('checkbox', { name: /查看者 VIEWER/ }).uncheck();
  await page.getByRole('checkbox', { name: /编辑者 EDITOR/ }).check();
  await page.getByRole('checkbox', { name: /审批人 APPROVER/ }).check();
  await page.getByRole('button', { name: '保存角色', exact: true }).click();
  await page.getByText('角色已保存，后续请求立即生效。').waitFor();
  const memberIdentity = await session(member);
  assert.deepEqual(memberIdentity.account.roles, ['EDITOR', 'APPROVER']);
  await page.getByRole('button', { name: `管理 ${memberUsername} 的角色` }).click();
  await page.getByRole('heading', { name: '角色变更历史' }).waitFor();
  await page.getByText(`操作者：${identity.account.id}`, { exact: true }).waitFor();
  await page.getByRole('button', { name: '关闭', exact: true }).last().click();

  adminAPI = await request.newContext();
  const adminIdentity = await login(adminAPI, adminUsername, 'browser password long enough');
  assert.ok(adminIdentity.account.roles.includes('ADMIN'));
  reviewerBrowser = await browserEngine.launch(launchOptions);
  const reviewerContext = await reviewerBrowser.newContext({
    storageState: await member.storageState(),
    viewport: { width: 1280, height: 900 },
  });
  const reviewerPage = await reviewerContext.newPage();

  await page.goto(`${origin}/configuration/managed-data`);
  await page.reload();
  await page.getByRole('heading', { name: '配置内容管理' }).waitFor();
  await context.close();
  context = await browserEngine.launchPersistentContext(profile, launchOptions);
  context.on('request', rememberRequest);
  page = await context.newPage();
  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByRole('heading', { name: '配置内容管理' }).waitFor();
  await page.getByLabel('Managed Table', { exact: true }).selectOption('notification_templates');
  await page.getByRole('button', { name: '新增记录', exact: true }).click();
  for (const [field, value] of Object.entries({ template_key: templateKey, channel: 'PUSH', body: 'browser initial configuration' })) {
    await page.getByLabel(`包含 ${field}`, { exact: true }).check();
    await page.getByLabel(`${field} 值`, { exact: true }).fill(value);
  }
  // Both branches meet here: a dirty drawer must protect ordinary navigation,
  // while session loss must suspend its prompt and keyboard trap for re-login.
  await page.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('alertdialog', { name: '放弃未保存的修改？' }).waitFor();
  await context.clearCookies();
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  assert.equal(await page.getByRole('alertdialog').count(), 0);
  await page.getByLabel('用户名', { exact: true }).fill(adminUsername);
  await page.keyboard.press('Tab');
  assert.equal(await page.getByLabel('密码', { exact: true }).evaluate(element => element === document.activeElement), true);
  await page.getByLabel('密码', { exact: true }).fill('browser password long enough');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor({ state: 'hidden' });
  assert.equal(await page.getByLabel('body 值', { exact: true }).inputValue(), 'browser initial configuration');
  await page.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('alertdialog', { name: '放弃未保存的修改？' }).waitFor();
  await page.getByRole('button', { name: '继续编辑', exact: true }).click();
  await page.getByRole('button', { name: '查看 Change Set' }).click();
  await page.getByRole('button', { name: '确认并保存草稿', exact: true }).click();
  await releaseState(page, '草稿');
  const addOrderID = new URL(page.url()).pathname.split('/').at(-1);
  assert.match(addOrderID, /^[a-f0-9]{32}$/);

  await page.getByRole('button', { name: '提交审批', exact: true }).click();
  await page.getByRole('button', { name: '确认提交审批', exact: true }).click();
  await releaseState(page, '待审批');
  assert.equal(await page.getByRole('button', { name: '批准发布单', exact: true }).count(), 0);

  await reviewerPage.goto(page.url());
  await reviewerPage.getByRole('button', { name: '批准发布单', exact: true }).click();
  await reviewerPage.getByLabel('审批意见', { exact: true }).fill('Independent browser publication review');
  await reviewerPage.getByRole('button', { name: '确认批准', exact: true }).click();
  await releaseState(reviewerPage, '已批准');
  assert.equal(await reviewerPage.getByRole('button', { name: '执行发布', exact: true }).count(), 0);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' });
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'approved release overflows before narrow execute recovery');
  await page.reload();
  await page.getByRole('button', { name: '执行发布', exact: true }).click();
  const executePath = `/api/v1/release-orders/${addOrderID}/execute`;
  const executeWrites = [];
  page.on('request', browserRequest => {
    if (browserRequest.method() === 'POST' && new URL(browserRequest.url()).pathname === executePath) {
      executeWrites.push({ body: browserRequest.postData(), key: browserRequest.headers()['idempotency-key'] });
    }
  });
  await page.route(`**${executePath}`, async route => {
    assert.equal(route.request().method(), 'POST');
    const result = await route.fetch();
    assert.equal(result.status(), 200);
    assert.equal((await result.json()).state, 'SUCCEEDED');
    await route.abort('failed');
  });
  await page.getByRole('button', { name: '确认发布到数据库', exact: true }).click();
  await page.getByText('结果待确认。原请求与意见已保留，请使用原请求重试。', { exact: true }).waitFor();
  await page.getByRole('button', { name: '使用原请求重试', exact: true }).waitFor();
  if (process.env.RCC_E2E_OUTPUT) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'publication-unknown.png'), fullPage: true });
  await page.unroute(`**${executePath}`);
  const acceptReload = dialog => dialog.accept();
  page.on('dialog', acceptReload);
  await page.reload();
  page.off('dialog', acceptReload);
  const recoveryPanel = page.getByRole('region', { name: '待处理发布请求' });
  const recoverExecute = page.getByRole('button', { name: '恢复原发布请求', exact: true });
  await recoverExecute.click();
  await recoveryPanel.waitFor({ state: 'detached' });
  await releaseState(page, '已发布待完结');
  await page.getByRole('heading', { name: '数据库发布结果', exact: true }).waitFor();
  await page.getByRole('region', { name: '发布结果' }).getByText(/分发尚未接入/, { exact: false }).waitFor();
  assert.equal(executeWrites.length, 2);
  assert.deepEqual(executeWrites[0], executeWrites[1]);
  if (process.env.RCC_E2E_OUTPUT) {
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'publication result overflows narrow viewport');
    await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'publication-result-mobile.png'), fullPage: true });
    await page.getByRole('region', { name: '发布结果' }).screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'publication-final-row-mobile.png') });
  }
  await page.setViewportSize({ width: 1280, height: 900 });
  if (process.env.RCC_E2E_OUTPUT) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'publication-result-desktop.png'), fullPage: true });

  const addPublication = await page.evaluate(async id => (await (await fetch(`/api/v1/release-orders/${id}`)).json()), addOrderID);
  assert.equal(addPublication.state, 'SUCCEEDED');
  assert.equal(addPublication.applicant_id, identity.account.id);
  assert.equal(addPublication.publication.publisher_id, identity.account.id);
  assert.equal(addPublication.publication.notification.status, 'NOT_CONNECTED');
  assert.equal(addPublication.publication.commands.length, 1);
  assert.equal(addPublication.publication.commands[0].operation, 'ADD');
  assert.equal(addPublication.publication.commands[0].record_version, '1');
  assert.equal(addPublication.publication.commands[0].final.deleted, false);
  assert.equal(addPublication.history.find(event => event.action === 'APPROVE')?.actor_id, memberIdentity.account.id);
  assert.equal(addPublication.history.find(event => event.action === 'EXECUTE')?.actor_id, identity.account.id);
  const addFinalFields = Object.fromEntries(addPublication.publication.commands[0].final.fields.map(field => [field.name, field.value]));
  assert.equal(addFinalFields.template_key, templateKey);
  assert.equal(addFinalFields.enabled, '1');
  assert.equal(addFinalFields.priority, '100');
  assert.equal(addFinalFields.creator, identity.account.id);
  assert.equal(addFinalFields.modifier, identity.account.id);
  await page.getByRole('button', { name: '完结发布单', exact: true }).click();
  await page.getByRole('button', { name: '确认完结', exact: true }).click();
  await releaseState(page, '已完结');

  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByLabel('Managed Table', { exact: true }).selectOption('notification_templates');
  const baseline = await page.evaluate(async key => {
    const auth = await (await fetch('/api/v1/auth/session')).json();
    const result = await (await fetch('/api/v1/tables/notification_templates/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': auth.csrf_token },
      body: JSON.stringify({ conditions: [{ field: 'template_key', operator: 'exact', value: key }] }),
    })).json();
    return { id: result.rows[0].id, version: result.record_versions[0] };
  }, templateKey);
  assert.equal(baseline.id, addPublication.publication.commands[0].id);
  assert.equal(baseline.version, '1');
  await page.getByRole('button', { name: `修改记录 ${baseline.id}`, exact: true }).click();
  await page.getByLabel('body 值', { exact: true }).fill('browser system configuration');
  await page.getByRole('button', { name: '查看 Change Set', exact: true }).click();

  const concurrent = await publishSingle({
    applicant: member,
    approver: adminAPI,
    publisher: adminAPI,
    item: {
      operation: 'MODIFY',
      id: baseline.id,
      expected_record_version: baseline.version,
      content: { body: 'concurrent browser update' },
    },
    keyPrefix: `browser-concurrent-${randomUUID()}`,
  });
  assert.equal(concurrent.publication.publisher_id, identity.account.id);
  assert.equal(concurrent.publication.commands[0].record_version, '2');

  await page.getByRole('button', { name: '确认并保存草稿', exact: true }).click();
  await page.getByText('明细 1：记录已被其他操作修改；你的输入已保留，请查看最新值并重新确认。', { exact: true }).waitFor();
  assert.equal(await page.getByRole('button', { name: '确认并保存草稿', exact: true }).isDisabled(), true);
  await page.getByRole('button', { name: '查看最新值', exact: true }).click();
  const conflictReview = page.getByRole('region', { name: '记录版本冲突' });
  await conflictReview.getByText('concurrent browser update', { exact: true }).waitFor();
  await page.locator('.change-set-dialog').getByText('browser system configuration', { exact: true }).last().waitFor();
  assert.equal(await page.getByRole('button', { name: '确认并保存草稿', exact: true }).isDisabled(), true);
  if (process.env.RCC_E2E_OUTPUT) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'record-version-conflict.png'), fullPage: true });
  assert.ok(await page.locator('.change-set-scroll').evaluate(element => element.clientHeight >= 100), 'latest values hid the pending difference');
  await page.setViewportSize({ width: 390, height: 844 });
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'record conflict overflows mobile viewport');
  assert.ok(await page.locator('.change-set-scroll').evaluate(element => element.clientHeight >= 50), 'mobile conflict hid pending difference');
  assert.ok(await page.getByRole('button', { name: '确认并保存草稿', exact: true }).evaluate(element => element.getBoundingClientRect().bottom <= innerHeight), 'mobile conflict hid draft action');
  if (process.env.RCC_E2E_OUTPUT) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'record-version-conflict-mobile.png') });
  await page.setViewportSize({ width: 1280, height: 900 });

  await page.getByRole('button', { name: '基于最新值重建差异', exact: true }).click();
  await page.getByRole('button', { name: '确认并保存草稿', exact: true }).click();
  await releaseState(page, '草稿');
  const modifyOrderID = new URL(page.url()).pathname.split('/').at(-1);
  assert.match(modifyOrderID, /^[a-f0-9]{32}$/);
  await page.getByRole('button', { name: '提交审批', exact: true }).click();
  await page.getByRole('button', { name: '确认提交审批', exact: true }).click();
  await releaseState(page, '待审批');
  await reviewerPage.goto(page.url());
  await reviewerPage.getByRole('button', { name: '批准发布单', exact: true }).click();
  await reviewerPage.getByLabel('审批意见', { exact: true }).fill('Approved after explicit latest baseline rebuild');
  await reviewerPage.getByRole('button', { name: '确认批准', exact: true }).click();
  await releaseState(reviewerPage, '已批准');
  await page.reload();
  await page.getByRole('button', { name: '执行发布', exact: true }).click();
  await page.getByRole('button', { name: '确认发布到数据库', exact: true }).click();
  await releaseState(page, '已发布待完结');
  await page.getByRole('heading', { name: '数据库发布结果', exact: true }).waitFor();
  await page.getByRole('region', { name: '发布结果' }).getByText(/分发尚未接入/, { exact: false }).waitFor();

  const modifyPublication = await page.evaluate(async id => (await (await fetch(`/api/v1/release-orders/${id}`)).json()), modifyOrderID);
  assert.equal(modifyPublication.state, 'SUCCEEDED');
  assert.equal(modifyPublication.publication.publisher_id, identity.account.id);
  assert.equal(modifyPublication.publication.notification.status, 'NOT_CONNECTED');
  assert.equal(modifyPublication.publication.commands[0].operation, 'MODIFY');
  assert.equal(modifyPublication.publication.commands[0].record_version, '3');
  const modifyFinalFields = Object.fromEntries(modifyPublication.publication.commands[0].final.fields.map(field => [field.name, field.value]));
  assert.equal(modifyFinalFields.body, 'browser system configuration');
  assert.equal(modifyFinalFields.creator, identity.account.id);
  assert.equal(modifyFinalFields.modifier, identity.account.id);

  const finalRow = await page.evaluate(async key => {
    const auth = await (await fetch('/api/v1/auth/session')).json();
    return (await fetch('/api/v1/tables/notification_templates/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': auth.csrf_token },
      body: JSON.stringify({ conditions: [{ field: 'template_key', operator: 'exact', value: key }] }),
    })).json();
  }, templateKey);
  assert.equal(finalRow.rows[0].body, 'browser system configuration');
  assert.equal(finalRow.record_versions[0], '3');

  const cookies = await context.cookies();
  const browserSession = cookies.find(cookie => cookie.name === 'rcc-session-dev');
  assert.ok(browserSession?.httpOnly && !browserSession.secure && browserSession.sameSite === 'Lax' && browserSession.path === '/');
  const storage = await page.evaluate(async () => ({ local: { ...localStorage }, session: { ...sessionStorage }, databases: await indexedDB.databases(), cookie: document.cookie }));
  const materials = [adminEmail, memberEmail, 'browser password long enough', 'browser initial configuration', 'browser system configuration', identity.csrf_token, browserSession.value];
  for (const secret of materials) {
    assert.ok(!JSON.stringify(storage).includes(secret), 'sensitive material persisted or script-readable');
    assert.ok(seenRequests.every(({ url }) => !decodeURIComponent(url).includes(secret)), 'sensitive material in URL');
  }
  assert.ok(seenRequests.filter(({ url }) => new URL(url).pathname.startsWith('/api/')).every(({ url }) => new URL(url).origin === origin));
  assert.ok(seenRequests.filter(({ method }) => ['POST', 'PATCH', 'DELETE'].includes(method)).every(({ url }) => !/^\/api\/v1\/tables\/[^/]+\/rows(?:\/|$)/.test(new URL(url).pathname)), 'browser used the removed direct record write route');
  await page.goto(`${origin}/account`);
  await page.getByRole('button', { name: '退出当前账号' }).click();
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  assert.equal(await page.evaluate(async () => (await fetch('/api/v1/table-policies')).status), 401);
  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  process.stdout.write(JSON.stringify({
    run_suffix: runSuffix,
    account_id: identity.account.id,
    username: adminUsername,
    member_username: memberUsername,
    template_key: templateKey,
    checks: [
      'record version conflict preserves input',
      'explicit latest read and baseline rebuild',
      'registration defaults VIEWER',
      'maintenance ADMIN bootstrap',
      'UI combination grant',
      '390px role catalog and drawer',
      'role history with permanent actor',
      'original member session refresh',
      'refresh',
      'reopen',
      'dirty navigation protection',
      'same-account Change Set recovery after session loss',
      'hidden drawer keyboard isolation',
      'draft submit independent approval and publisher execution',
      'unknown execute result reload and original-key recovery',
      'final row defaults and permanent actor fields',
      'refresh notification remains NOT_CONNECTED',
      'no direct record write request',
      'HTTP Cookie',
      'storage/URL secrecy',
      'logout rejection',
    ],
  }));
} catch (error) {
  if (process.env.RCC_E2E_OUTPUT && page && !page.isClosed()) await page.screenshot({ path: join(process.env.RCC_E2E_OUTPUT, 'accounts-failure.png'), fullPage: true }).catch(() => {});
  throw error;
} finally {
  await reviewerBrowser?.close();
  await adminAPI?.dispose();
  await member?.dispose();
  await context?.close();
  await rm(profile, { recursive: true, force: true });
}
