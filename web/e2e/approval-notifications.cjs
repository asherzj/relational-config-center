// AC-013..017: formal Web → Admin HTTP → disposable MySQL. Use the same
// table/query fixture as notification-center.cjs, on a fresh isolated database.
const assert = require('node:assert/strict');
const { join } = require('node:path');
const { mkdirSync, writeFileSync } = require('node:fs');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { selectedBrowser, browserOptions, registerFixtureAccount } = require('./local-account.cjs');
const { approvalFixtureRequest: api, createFixtureApprovalRole: createRole, bindFixtureApprovalRoles: bind, fixtureApprovalInput } = require('./table-approval-fixture.cjs');
const base = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
const tables = ['multitable_browser_a', 'multitable_browser_b'];
const button = (page, name) => page.getByRole('button', { name, exact: true });
(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const checks = [], errors = [], pages = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  if (output) mkdirSync(output, { recursive: true });
  const shot = async (page, name) => {
    await page.evaluate(() => window.scrollTo(0, 0));
    if (output) await page.screenshot({ path: join(output, name), fullPage: true, animations: 'disabled' });
  };
  const person = async roles => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const identity = await registerFixtureAccount(context, base, { roles });
    const page = await context.newPage(); page.setDefaultTimeout(15000); pages.push(page);
    page.on('pageerror', error => errors.push(error.message));
    return { context, identity, page };
  };
  const counts = async (person, unread, pending) => {
    await person.page.getByRole('link', { name: `个人未读 ${unread}`, exact: true }).waitFor();
    await person.page.getByLabel('个人待审批数', { exact: true }).filter({ hasText: new RegExp(`^待审批 ${pending}$`) }).waitFor();
  };
  const readCounts = person => api(person.context, base, 'GET', '/api/v1/approval-notifications');
  const approve = async (person, orderID) => {
    const order = await api(person.context, base, 'GET', `/api/v1/release-orders/${orderID}`);
    return api(person.context, base, 'POST', `/api/v1/release-orders/${orderID}/approve`, await fixtureApprovalInput(person.context, base, orderID, { expected_version: order.version, reason: '核对本人的表' }));
  };
  try {
    const admin = await person(['ADMIN']), applicant = await person(['EDITOR']), reviewer = await person(['VIEWER']), second = await person(['VIEWER']), unrelated = await person(['VIEWER']);
    const mutation = 'approval_notifications_browser_v1';
    await api(admin.context, base, 'POST', '/api/v1/mutation-policies', { code: mutation, name: '个人审批通知验收', description: '', type_code: 'single_table_mutation', allow_add: true, allow_modify: true, allow_delete: true }, 201);
    await api(admin.context, base, 'POST', `/api/v1/mutation-policies/${mutation}/activate`, {});
    for (const table of tables) {
      const policy = await api(admin.context, base, 'POST', '/api/v1/table-policies', { table_name: table, query_policy_code: 'notification_page_query_v1', mutation_policy_code: mutation }, 201);
      await api(admin.context, base, 'POST', `/api/v1/table-policies/${table}/enable`, { expected_version: policy.version });
      const associations = (await api(admin.context, base, 'GET', `/api/v1/table-policies/${table}/release-templates`)).associations;
      const standard = associations.find(association => association.type === 'STANDARD');
      await api(admin.context, base, 'PUT', `/api/v1/table-policies/${table}/release-templates/STANDARD`, { template_code: 'default_standard_v1', enabled: true, expected_version: standard?.version ?? '0' });
    }
    const firstRole = await createRole(admin.context, base, '同单聚合的商品审批长名称', [reviewer.identity.accountID]);
    const duplicateRole = await createRole(admin.context, base, '同一人员的第二个审批角色', [reviewer.identity.accountID]);
    const secondRole = await createRole(admin.context, base, '价格审批', [second.identity.accountID]);
    await bind(admin.context, base, tables[0], [firstRole.id, duplicateRole.id]);
    await bind(admin.context, base, tables[1], [secondRole.id]);
    const draft = await api(applicant.context, base, 'POST', '/api/v1/release-orders', { title: '个人未读与仍待审批的多表申请：长标题保留完整审批进度和返回位置', items: tables.map(table_name => ({ table_name, operation: 'ADD', content: { id: '9601', label: 'approval-notification' } })) }, 201);
    await api(applicant.context, base, 'POST', `/api/v1/release-orders/${draft.id}/submit`, { expected_version: draft.version });
    assert.deepEqual(await readCounts(reviewer), { unread_count: 1, pending_count: 1 });
    assert.deepEqual(await readCounts(unrelated), { unread_count: 0, pending_count: 0 });
    assert.deepEqual(await readCounts(applicant), { unread_count: 0, pending_count: 0 });
    await reviewer.page.goto(`${base}/configuration/notifications`);
    await counts(reviewer, 1, 1);
    await reviewer.page.getByRole('row').filter({ hasText: draft.title }).getByText('未读', { exact: true }).waitFor();
    check('同账号多角色聚合一条未读；申请人自身动作及无关查看者没有提醒');
    await shot(reviewer.page, 'notifications-desktop.png');

    const readPath = `**/api/v1/release-orders/${draft.id}/notification-read`;
    const ackBodies = [];
    const failRead = async route => {
      ackBodies.push(route.request().postDataJSON());
      await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'release_unavailable', message: 'injected ack failure', request_id: 'notification-read-browser-failure' } }) });
    };
    await reviewer.page.route(readPath, failRead);
    const detail = reviewer.page.getByRole('link', { name: `查看详情：${draft.title}`, exact: true });
    await detail.focus(); await reviewer.page.keyboard.press('Enter');
    await reviewer.page.getByRole('heading', { name: draft.title, exact: true }).waitFor();
    await reviewer.page.getByText('标记已读失败，未读提醒已保留；详情仍可继续查看。', { exact: true }).waitFor();
    assert.equal(ackBodies.length, 1);
    await counts(reviewer, 1, 1);
    await shot(reviewer.page, 'notification-read-failure.png');
    await reviewer.page.unroute(readPath, failRead);
    await button(reviewer.page, '重试标记已读').click();
    await reviewer.page.getByText('已读；本单仍待你审批。', { exact: true }).waitFor();
    await counts(reviewer, 0, 1);
    check('键盘打开真实详情，已读失败可继续查看并显式重试；读后待审批仍为一');
    await button(reviewer.page, '批准发布单').click();
    await reviewer.page.getByLabel('审批意见', { exact: true }).fill('先核对商品表');
    await button(reviewer.page, '确认批准').click();
    await reviewer.page.getByRole('heading', { name: '已通过 1 / 2 表', exact: true }).waitFor();
    await reviewer.page.getByRole('dialog', { name: '批准发布单', exact: true }).waitFor({ state: 'detached' });
    await counts(reviewer, 0, 0);
    assert.deepEqual(await readCounts(applicant), { unread_count: 1, pending_count: 0 });

    // A second genuine approval lands after the applicant's header has been
    // read and before its acknowledgement reaches Admin.
    let displayedSequence, concurrentAck;
    await applicant.page.route(`**/api/v1/release-orders/${draft.id}`, async route => {
      const response = await route.fetch(); displayedSequence = (await response.json()).notification.sequence;
      await route.fulfill({ response });
    });
    await applicant.page.route(readPath, async route => {
      concurrentAck = route.request().postDataJSON();
      await approve(second, draft.id);
      await route.continue();
    });
    await applicant.page.goto(`${base}/configuration/notifications/${draft.id}?view=mine&unread=true`);
    await applicant.page.getByText('本单还有新的未读变化。', { exact: true }).waitFor();
    assert.deepEqual(concurrentAck, { sequence: displayedSequence });
    assert.deepEqual(await readCounts(applicant), { unread_count: 1, pending_count: 0 });
    await applicant.page.unroute(readPath);
    await button(applicant.page, '读取最新进展').click();
    await applicant.page.getByRole('heading', { name: '已通过 2 / 2 表', exact: true }).waitFor();
    await counts(applicant, 0, 0);
    check('申请人收到部分审批进展；并发全量批准保留新未读，确认序号只来自已展示详情');

    await applicant.page.getByRole('link', { name: '返回通知中心列表', exact: true }).click();
    assert.equal(new URL(applicant.page.url()).searchParams.get('unread'), 'true');
    await applicant.page.getByText('没有符合筛选条件的发布单。', { exact: true }).waitFor();
    await applicant.page.getByRole('checkbox', { name: '仅看未读', exact: true }).uncheck();
    await button(applicant.page, '查询审批').click();
    await applicant.page.getByRole('link', { name: `查看详情：${draft.title}`, exact: true }).waitFor();
    const failCounts = '**/api/v1/approval-notifications';
    await applicant.page.route(failCounts, route => route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'release_unavailable', message: 'injected counts failure', request_id: 'notification-counts-browser-failure' } }) }));
    await button(applicant.page, '刷新通知').click();
    await applicant.page.getByText('未读与待审批数刷新失败，保留上次读取的计数。', { exact: true }).waitFor();
    await counts(applicant, 0, 0);
    await applicant.page.unroute(failCounts);
    await button(applicant.page, '刷新通知').click();
    await applicant.page.getByText('未读与待审批数刷新失败，保留上次读取的计数。', { exact: true }).waitFor({ state: 'detached' });
    check('未读筛选和返回位置保留；计数接口失败显示最后已知数据并可恢复');

    const membershipOrder = await api(applicant.context, base, 'POST', '/api/v1/release-orders', { title: '资格变化保留仍可审表', items: [{ table_name: tables[0], operation: 'ADD', content: { id: '9602', label: 'membership-notification' } }] }, 201);
    await api(applicant.context, base, 'POST', `/api/v1/release-orders/${membershipOrder.id}/submit`, { expected_version: membershipOrder.version });
    const members = async (roleID, memberIDs) => {
      const current = await api(admin.context, base, 'GET', `/api/v1/approval-roles/${roleID}`);
      return api(admin.context, base, 'PUT', `/api/v1/approval-roles/${roleID}`, { name: current.name, description: current.description, enabled: current.enabled, expected_version: current.version, member_ids: memberIDs });
    };
    await reviewer.page.goto(`${base}/configuration/notifications`); await counts(reviewer, 1, 1);
    await members(firstRole.id, []);
    await button(reviewer.page, '刷新通知').click(); await counts(reviewer, 1, 1);
    await members(duplicateRole.id, []);
    await button(reviewer.page, '刷新通知').click(); await counts(reviewer, 0, 0);
    await button(reviewer.page, '刷新列表').click();
    await reviewer.page.getByText('暂无待你审批的发布单。', { exact: true }).waitFor();
    await members(firstRole.id, [reviewer.identity.accountID]);
    await button(reviewer.page, '刷新通知').click(); await counts(reviewer, 1, 1);
    check('失去一个角色仍保留可审表，全部失去移除待办与纯待办未读，恢复资格补充新提醒');

    await applicant.page.setViewportSize({ width: 390, height: 844 });
    const geometry = await applicant.page.evaluate(() => ({ viewport: innerWidth, width: document.documentElement.scrollWidth }));
    assert.ok(geometry.width <= geometry.viewport, JSON.stringify(geometry));
    await shot(applicant.page, 'notifications-390.png');
    await button(applicant.page, '打开导航').click();
    await counts(applicant, 0, 0);
    await shot(applicant.page, 'notifications-390-navigation.png');
    await button(applicant.page, '关闭导航').first().click();
    const session = await (await applicant.context.request.get(`${base}/api/v1/auth/session`)).json();
    const logout = await applicant.context.request.post(`${base}/api/v1/auth/logout`, { headers: { Origin: new URL(base).origin, 'X-CSRF-Token': session.csrf_token } });
    assert.equal(logout.status(), 204);
    await button(applicant.page, '刷新列表').click();
    await applicant.page.getByRole('heading', { name: '登录本地账号', exact: true }).waitFor();
    await applicant.page.getByLabel('用户名', { exact: true }).fill(applicant.identity.credentials.username);
    await applicant.page.getByLabel('密码', { exact: true }).fill(applicant.identity.credentials.password);
    await button(applicant.page, '登录').click();
    await applicant.page.getByRole('link', { name: `查看详情：${draft.title}`, exact: true }).waitFor();
    check('390px 页面无横向溢出、未读入口可达；真实会话失效并同账号恢复列表');
    let releaseHeld, capturedHeld;
    const held = new Promise(resolve => { capturedHeld = resolve; });
    const gate = new Promise(resolve => { releaseHeld = resolve; });
    let delayed = false;
    await reviewer.page.route('**/api/v1/approval-notifications', async route => {
      if (delayed) { await route.continue(); return; }
      delayed = true;
      const response = await route.fetch();
      assert.equal((await response.json()).unread_count, 1);
      capturedHeld(); await gate; await route.fulfill({ response });
    });
    await button(reviewer.page, '刷新通知').click(); await held;
    const reviewerSession = await (await reviewer.context.request.get(`${base}/api/v1/auth/session`)).json();
    assert.equal((await reviewer.context.request.post(`${base}/api/v1/auth/logout`, { headers: { Origin: new URL(base).origin, 'X-CSRF-Token': reviewerSession.csrf_token } })).status(), 204);
    const otherTab = await reviewer.context.newPage();
    await otherTab.goto(`${base}/login?returnTo=/configuration/notifications`);
    await otherTab.getByLabel('用户名', { exact: true }).fill(unrelated.identity.credentials.username);
    await otherTab.getByLabel('密码', { exact: true }).fill(unrelated.identity.credentials.password);
    await button(otherTab, '登录').click();
    await counts(reviewer, 0, 0);
    const oldResponse = reviewer.page.waitForResponse(response => response.url().endsWith('/api/v1/approval-notifications') && response.headers()['x-rcc-account-id'] === reviewer.identity.accountID);
    releaseHeld(); await oldResponse;
    await counts(reviewer, 0, 0);
    check('另一标签真实登录不同账号，旧账号延迟计数响应不能覆盖新账号');
    await otherTab.close();
    assert.deepEqual(errors, []);
    if (output) writeFileSync(join(output, 'result.json'), JSON.stringify({ checks, errors }, null, 2));
  } catch (error) {
    if (output) for (let index = 0; index < pages.length; index++) await pages[index].screenshot({ path: join(output, `failure-${index}.png`), fullPage: true }).catch(() => {});
    throw error;
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
