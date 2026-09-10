// AC-012: formal Web → Admin HTTP → one disposable MySQL.
const assert = require('node:assert/strict');
const { join } = require('node:path');
const { mkdirSync, writeFileSync } = require('node:fs');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { selectedBrowser, browserOptions, registerFixtureAccount } = require('./local-account.cjs');
const { approvalFixtureRequest: api, createFixtureApprovalRole: createRole, bindFixtureApprovalRoles: bind } = require('./table-approval-fixture.cjs');
const base = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
const tables = ['multitable_browser_a', 'multitable_browser_b'];
const button = (page, name) => page.getByRole('button', { name, exact: true });
(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const checks = [], errors = [], surfaces = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  if (output) mkdirSync(output, { recursive: true });
  const shot = async (page, name) => {
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.evaluate(() => Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {}))));
    if (output) await page.screenshot({ path: join(output, name), fullPage: true, animations: 'disabled' });
  };
  const person = async roles => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const identity = await registerFixtureAccount(context, base, { roles });
    const page = await context.newPage(); page.setDefaultTimeout(15000); surfaces.push(page);
    page.on('pageerror', error => errors.push(error.message));
    return { context, identity, page };
  };
  const noOverflow = async (page, name) => {
    const geometry = await page.evaluate(() => ({ viewport: innerWidth, documentWidth: document.documentElement.scrollWidth,
      tables: [...document.querySelectorAll('[data-slot="table-container"]')].map(node => ({ width: node.clientWidth, scrollWidth: node.scrollWidth })) }));
    if (output) writeFileSync(join(output, `${name}.json`), JSON.stringify(geometry, null, 2));
    assert.ok(geometry.documentWidth <= geometry.viewport, JSON.stringify(geometry));
  };
  const changeView = async (page, name) => {
    await page.getByRole('navigation', { name: '审批视图' }).getByRole('link', { name, exact: true }).click();
    await page.getByRole('region', { name: `${name}列表`, exact: true }).waitFor();
  };
  try {
    const admin = await person(['ADMIN']), applicant = await person(['EDITOR']), reviewer = await person(['VIEWER']);
    const mutation = 'notification_center_browser_v1';
    await api(admin.context, base, 'POST', '/api/v1/mutation-policies', { code: mutation, name: '通知中心验收', description: '', type_code: 'single_table_mutation', allow_add: true, allow_modify: true, allow_delete: true }, 201);
    await api(admin.context, base, 'POST', `/api/v1/mutation-policies/${mutation}/activate`, {});
    for (const table of tables) {
      await api(admin.context, base, 'POST', '/api/v1/table-policies', { table_name: table, query_policy_code: 'notification_page_query_v1', mutation_policy_code: mutation }, 201);
      await api(admin.context, base, 'POST', `/api/v1/table-policies/${table}/enable`, {});
    }
    const roles = [];
    for (const name of ['商品审批', '共同复核']) roles.push(await createRole(admin.context, base, name, [reviewer.identity.accountID]));
    for (const table of tables) await bind(admin.context, base, table, roles.map(role => role.id));
    const orders = [];
    for (let index = 0; index < 23; index++) {
      const title = `${String(index).padStart(2, '0')} 通知中心长标题：商品配置与价格配置由同一人员按两张表完整审阅，详情返回保留筛选和当前分页位置`;
      const order = await api(applicant.context, base, 'POST', '/api/v1/release-orders', { title, items: tables.map(table_name => ({ table_name, operation: 'ADD', content: { id: String(1000 + index), label: `notice-${index}` } })) }, 201);
      if (index < 22) {
        await api(applicant.context, base, 'POST', `/api/v1/release-orders/${order.id}/submit`, { expected_version: order.version });
        orders.push(order);
      } else {
        await api(applicant.context, base, 'POST', `/api/v1/release-orders/${order.id}/cancel`, { expected_version: order.version, reason: '未提交草稿不进入通知中心' });
      }
    }
    orders.sort((a, b) => a.id.localeCompare(b.id));
    await reviewer.page.goto(`${base}/configuration/release-orders`);
    await reviewer.page.getByRole('navigation').getByRole('link', { name: '通知中心', exact: true }).click();
    await reviewer.page.getByRole('region', { name: '待我审批发布单', exact: true }).waitFor();
    assert.equal(await reviewer.page.getByRole('navigation', { name: '审批视图' }).getByRole('link', { name: '待我审批', exact: true }).getAttribute('aria-current'), 'page');
    assert.equal(await reviewer.page.locator('tbody tr').count(), 20);
    assert.equal(await reviewer.page.getByText(/未读/).count(), 0);
    await noOverflow(reviewer.page, 'desktop-geometry'); await shot(reviewer.page, 'center-desktop.png');
    check('正式导航默认待我审批；多角色、多表一单一行，首屏20项且不展示伪造未读');

    await reviewer.page.getByLabel('表名', { exact: true }).fill(tables[0]);
    await reviewer.page.getByLabel('状态', { exact: true }).selectOption('PENDING_APPROVAL');
    await button(reviewer.page, '查询审批').click();
    await button(reviewer.page, '下一页').click();
    await reviewer.page.getByRole('link', { name: `查看详情：${orders[20].title}`, exact: true }).waitFor();
    assert.equal(await reviewer.page.locator('tbody tr').count(), 2);
    assert.ok(await button(reviewer.page, '下一页').isDisabled());
    const originalURL = reviewer.page.url();
    const position = new URL(originalURL).searchParams;
    assert.equal(position.get('after'), orders[19].id);
    assert.equal(position.get('table_name'), tables[0]); assert.equal(position.get('state'), 'PENDING_APPROVAL');
    await reviewer.page.getByRole('link', { name: `查看详情：${orders[20].title}`, exact: true }).focus();
    await reviewer.page.keyboard.press('Enter');
    await reviewer.page.getByRole('region', { name: '逐表审批进度', exact: true }).waitFor();
    await button(reviewer.page, '批准发布单').click();
    await reviewer.page.getByLabel('审批意见', { exact: true }).fill('从通知中心核对两张表');
    await button(reviewer.page, '确认批准').click();
    await reviewer.page.getByRole('heading', { name: '已通过 2 / 2 表', exact: true }).waitFor();
    await reviewer.page.getByRole('link', { name: '返回通知中心列表', exact: true }).click();
    assert.equal(reviewer.page.url(), originalURL);
    await reviewer.page.getByRole('link', { name: `查看详情：${orders[20].title}`, exact: true }).waitFor({ state: 'detached' });
    await reviewer.page.getByRole('link', { name: `查看详情：${orders[21].title}`, exact: true }).waitFor();
    assert.equal(await reviewer.page.locator('tbody tr').count(), 1);
    check('第二页键盘进入正式详情并真实批准两表，返回保留视图/筛选/游标且已处理单退出待办');

    await reviewer.page.getByLabel('状态', { exact: true }).selectOption('');
    await button(reviewer.page, '查询审批').click();
    await changeView(reviewer.page, '我已处理');
    await reviewer.page.getByRole('link', { name: `查看详情：${orders[20].title}`, exact: true }).waitFor();
    assert.equal(await reviewer.page.locator('tbody tr').count(), 1);
    await changeView(reviewer.page, '我发起的');
    await reviewer.page.getByText('没有符合筛选条件的发布单。', { exact: true }).waitFor();
    await changeView(reviewer.page, '全部审批');
    await reviewer.page.getByRole('link', { name: `查看详情：${orders[0].title}`, exact: true }).waitFor();
    await applicant.page.goto(`${base}/configuration/notifications?view=mine`);
    await applicant.page.getByRole('region', { name: '我发起的发布单', exact: true }).waitFor();
    assert.equal(await applicant.page.locator('tbody tr').count(), 20);
    await admin.page.goto(`${base}/configuration/notifications`);
    await admin.page.getByText('暂无待你审批的发布单。', { exact: true }).waitFor();
    await changeView(admin.page, '全部审批');
    await admin.page.getByRole('link', { name: `查看详情：${orders[0].title}`, exact: true }).click();
    await admin.page.getByRole('region', { name: '逐表审批进度', exact: true }).waitFor();
    assert.equal(await button(admin.page, '批准发布单').count(), 0);
    check('四视图归属与真实处理一致；ADMIN可查看全单仍无角色审批权，未提交取消草稿不混入');

    await reviewer.page.setViewportSize({ width: 390, height: 844 });
    await reviewer.page.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' });
    await noOverflow(reviewer.page, 'mobile-geometry');
    const table = reviewer.page.getByRole('region', { name: '全部审批发布单', exact: true });
    await table.focus(); assert.equal(await table.evaluate(node => document.activeElement === node), true);
    const entry = reviewer.page.getByRole('link', { name: `查看详情：${orders[0].title}`, exact: true });
    const box = await entry.boundingBox(); assert.ok(box.x >= 0 && box.x + box.width <= 390, JSON.stringify(box));
    await shot(reviewer.page, 'center-mobile.png');
    await entry.focus(); await reviewer.page.keyboard.press('Enter');
    await reviewer.page.getByRole('region', { name: '逐表审批进度', exact: true }).waitFor();
    await noOverflow(reviewer.page, 'detail-mobile-geometry'); await shot(reviewer.page, 'detail-mobile.png');
    await reviewer.page.getByRole('link', { name: '返回通知中心列表', exact: true }).focus(); await reviewer.page.keyboard.press('Enter');
    await reviewer.page.getByRole('region', { name: '全部审批发布单', exact: true }).waitFor();
    check('390px长标题/多表无页面溢出，固定详情入口与列表返回可键盘操作');

    const before = await reviewer.page.locator('tbody').innerText();
    const route = '**/api/v1/release-orders?*';
    await reviewer.page.route(route, handler => handler.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'release_unavailable', message: 'injected read failure', request_id: 'center-browser-read-failure' } }) }));
    await button(reviewer.page, '刷新列表').click();
    await reviewer.page.getByRole('alert').filter({ hasText: 'center-browser-read-failure' }).waitFor();
    assert.ok((await reviewer.page.getByRole('alert').innerText()).includes('审批列表读取失败，请重试。'));
    assert.ok((await reviewer.page.getByRole('alert').innerText()).includes('release_unavailable'));
    assert.ok(!(await reviewer.page.getByRole('alert').innerText()).includes('请保留原请求'));
    assert.equal(await reviewer.page.locator('tbody').innerText(), before);
    assert.ok(await button(reviewer.page, '下一页').isDisabled());
    await shot(reviewer.page, 'read-failure-mobile.png');
    await reviewer.page.unroute(route);
    await button(reviewer.page, '重试').click();
    await reviewer.page.getByRole('alert').filter({ hasText: 'center-browser-read-failure' }).waitFor({ state: 'detached' });
    check('读取失败保留最后列表并显示可重试错误，恢复后真实读取成功');
    assert.deepEqual(errors, []);
    if (output) writeFileSync(join(output, 'result.json'), JSON.stringify({ checks, errors, orders: orders.map(order => order.id) }, null, 2));
  } catch (error) {
    if (output) {
      writeFileSync(join(output, 'failure.json'), JSON.stringify({ checks, errors, message: error.message, stack: error.stack }, null, 2));
      for (const [index, page] of surfaces.entries()) {
        await shot(page, `failure-${index}.png`).catch(() => {});
        writeFileSync(join(output, `failure-${index}.txt`), await page.locator('body').innerText().catch(() => 'unavailable'));
      }
    }
    throw error;
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
