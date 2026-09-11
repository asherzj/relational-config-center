// #94: real browser → public Admin HTTP → disposable MySQL, no business mocks.
const assert = require('node:assert/strict');
const { join } = require('node:path');
const { mkdirSync, writeFileSync } = require('node:fs');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { selectedBrowser, browserOptions, registerFixtureAccount } = require('./local-account.cjs');
const { approvalFixtureRequest: api, bindFixtureApprovalRoles: bind, createFixtureApprovalRole: createRole } = require('./table-approval-fixture.cjs');
const base = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
const engine = process.env.RCC_E2E_ENGINE || 'chromium';
const suffix = engine === 'chromium' ? '' : `_${engine}`;
const tables = ['multitable_browser_a', 'multitable_browser_b'].map(table => table + suffix);
const button = (page, name) => page.getByRole('button', { name, exact: true });
(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const checks = [], errors = [], surfaces = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  if (output) mkdirSync(output, { recursive: true });
  const shot = async (page, name) => { await page.evaluate(() => Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {})))); if (output) await page.screenshot({ path: join(output, name), fullPage: true, animations: 'disabled' }); };
  const checkReviewGeometry = async page => {
    const geometry = await page.evaluate(() => {
      const root = document.querySelector('.release-conflict-review');
      const bounds = element => {
        const rect = element.getBoundingClientRect(), style = getComputedStyle(element);
        return { tag: element.tagName, label: element.getAttribute('aria-label'), class: element.className, left: rect.left, right: rect.right, width: rect.width, clientWidth: element.clientWidth, scrollWidth: element.scrollWidth, display: style.display, minWidth: style.minWidth, overflowX: style.overflowX };
      };
      return { viewport: innerWidth, documentWidth: document.documentElement.scrollWidth, review: root ? bounds(root) : null, children: root ? [...root.children].map(bounds) : [], content: root ? [...root.querySelectorAll('section,details,.table-scroll,[data-slot="table-container"]')].map(bounds) : [] };
    });
    if (output) writeFileSync(join(output, 'scope-review-geometry-390.json'), JSON.stringify(geometry, null, 2));
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), JSON.stringify(geometry));
  };
  const person = async roles => { const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } }); const identity = await registerFixtureAccount(context, base, { roles }); const page = await context.newPage(); surfaces.push(page); page.setDefaultTimeout(15000); page.on('pageerror', error => errors.push(error.message)); page.on('dialog', dialog => dialog.accept()); return { context, identity, page }; };
  try {
    const admin = await person(['ADMIN']), applicant = await person(['EDITOR', 'PUBLISHER']), ops = await person(['VIEWER']), finance = await person(['VIEWER']);
    const mutation = `approval_browser_${engine}_v1`;
    await api(admin.context, base, 'POST', '/api/v1/mutation-policies', { code: mutation, name: '审批角色多表验收', description: '', type_code: 'single_table_mutation', allow_add: true, allow_modify: true, allow_delete: true }, 201);
    await api(admin.context, base, 'POST', `/api/v1/mutation-policies/${mutation}/activate`, {});
    for (const table of tables) {
      await api(admin.context, base, 'POST', '/api/v1/table-policies', { table_name: table, query_policy_code: 'notification_page_query_v1', mutation_policy_code: mutation }, 201);
      await api(admin.context, base, 'POST', `/api/v1/table-policies/${table}/enable`, { expected_version: "1" });
      await api(admin.context, base, 'PUT', `/api/v1/table-policies/${table}/release-templates/STANDARD`, {template_code:'default_standard_v1',enabled:true,expected_version:'0'});
    }
    const opsRole = await createRole(admin.context, base, '商品运营审批角色 · 长名称验证多人分工', [ops.identity.accountID]);
    const priceRole = await createRole(admin.context, base, '财务价格审批', [finance.identity.accountID]);
    // The only ADMIN is this application's author, so an empty assignment has
    // no independent fallback. The UI must explain both affected tables.
    const unavailableInput = { title: '无人可审批时保留申请与进度', items: tables.map(table_name => ({ table_name, operation: 'MODIFY', id: '4', expected_record_version: '0', content: { label: 'preserved-unavailable-approval' } })) };
    const unavailableDraft = await api(admin.context, base, 'POST', '/api/v1/release-orders', unavailableInput, 201);
    const unavailablePath = `/api/v1/release-orders/${unavailableDraft.id}`;
    await admin.page.goto(`${base}/configuration/release-orders/${unavailableDraft.id}`);
    const unavailablePlan = admin.page.getByRole('region', { name: '提交审批安排', exact: true });
    for (const table of tables) await unavailablePlan.getByRole('heading', { name: table, exact: true }).waitFor();
    assert.equal(await unavailablePlan.getByText('暂无独立审批人', { exact: true }).count(), 2);
    await button(admin.page, '提交审批').click();
    const unavailableResponse = admin.page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith(`/${unavailableDraft.id}/submit`));
    await button(admin.page, '确认提交审批').click();
    const unavailableResult = await unavailableResponse;
    assert.equal(unavailableResult.status(), 422);
    assert.equal((await unavailableResult.json()).error.code, 'release_approver_unavailable');
    await admin.page.getByText('有表缺少独立审批人，请查看各表审批安排并补充合格人员后重新提交。', { exact: true }).waitFor();
    assert.equal(await admin.page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。', { exact: true }).count(), 0);
    const retainedDraft = await api(admin.context, base, 'GET', unavailablePath);
    assert.equal(retainedDraft.state, 'DRAFT');
    assert.equal(retainedDraft.version, unavailableDraft.version);
    assert.deepEqual(retainedDraft.approval_context.tables.map(table => [table.table_name, table.mode]), tables.map(table => [table, 'UNAVAILABLE']));
    await shot(admin.page, 'table-approval-unavailable-submit.png');
    await button(admin.page, '关闭').last().click();
    for (const table of tables) await bind(admin.context, base, table, [opsRole.id]);
    await admin.page.reload(); await button(admin.page, '提交审批').click(); await button(admin.page, '确认提交审批').click(); await admin.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^待审批$/ }).waitFor();
    const pendingUnavailable = await api(admin.context, base, 'GET', unavailablePath);
    const assignedRole = await api(admin.context, base, 'GET', `/api/v1/approval-roles/${opsRole.id}`);
    await api(admin.context, base, 'PUT', `/api/v1/approval-roles/${opsRole.id}`, { name: assignedRole.name, description: assignedRole.description, enabled: true, member_ids: [], expected_version: assignedRole.version });
    await admin.page.reload();
    const unavailableProgress = admin.page.getByRole('region', { name: '逐表审批进度', exact: true });
    await unavailableProgress.getByRole('heading', { name: '已通过 0 / 2 表', exact: true }).waitFor();
    assert.equal(await unavailableProgress.getByText('暂无独立审批人', { exact: true }).count(), 2);
    assert.equal(await button(admin.page, '批准发布单').count(), 0);
    await ops.page.goto(`${base}/configuration/release-orders/${unavailableDraft.id}`);
    assert.equal(await button(ops.page, '批准发布单').count(), 0);
    const paused = await api(admin.context, base, 'GET', unavailablePath);
    assert.equal(paused.state, 'PENDING_APPROVAL'); assert.equal(paused.version, pendingUnavailable.version);
    assert.deepEqual(paused.approvals, pendingUnavailable.approvals);
    assert.deepEqual(paused.approval_context.tables.map(table => [table.table_name, table.mode]), tables.map(table => [table, 'UNAVAILABLE']));
    await shot(admin.page, 'table-approval-unavailable-pending.png');
    await button(admin.page, '更多操作').click(); await admin.page.getByRole('menuitem', { name: '取消发布单', exact: true }).click(); await admin.page.getByLabel('取消原因', { exact: true }).fill('保留无人审批证据后释放测试目标'); await button(admin.page, '确认取消发布单').click(); await admin.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^已取消$/ }).waitFor();
    const releasedTarget = await api(admin.context, base, 'POST', '/api/v1/release-orders', unavailableInput, 201);
    await api(admin.context, base, 'POST', `/api/v1/release-orders/${releasedTarget.id}/cancel`, { expected_version: releasedTarget.version, reason: '目标释放已核对' });
    const emptyRole = await api(admin.context, base, 'GET', `/api/v1/approval-roles/${opsRole.id}`);
    await api(admin.context, base, 'PUT', `/api/v1/approval-roles/${opsRole.id}`, { name: emptyRole.name, description: emptyRole.description, enabled: true, member_ids: [ops.identity.accountID], expected_version: emptyRole.version });
    for (const table of tables) await bind(admin.context, base, table, []);
    check('无独立审批人显示问题表，提交422保留草稿；在途成员离开保留进度，取消释放目标');
    for (const [index, table] of tables.entries()) {
      await admin.page.goto(`${base}/platform/table-policies`);
      const row = admin.page.getByRole('row').filter({ has: admin.page.getByRole('cell', { name: table, exact: true }) });
      await row.getByRole('button', { name: '审批角色', exact: true }).click();
      await admin.page.getByText('未选择角色时，由已启用且不是申请人的 ADMIN 默认审批。无人符合时不能提交。', { exact: true }).waitFor();
      const choice = admin.page.getByRole('checkbox', { name: `选择审批角色 ${index ? priceRole.name : opsRole.name}`, exact: true });
      await choice.focus(); await admin.page.keyboard.press('Space');
      await button(admin.page, '保存表审批角色').click();
      await admin.page.getByRole('dialog', { name: '表审批角色', exact: true }).waitFor({ state: 'detached' });
    }
    check('管理员从正式表目录以键盘配置两个表的独立审批角色');
    // Keep local empty choice while another administrator changes the server version.
    await admin.page.goto(`${base}/platform/table-policies/${tables[0]}?mode=approvals`);
    await button(admin.page, `移除审批角色 ${opsRole.name}`).click();
    await bind(admin.context, base, tables[0], [priceRole.id]);
    await button(admin.page, '保存表审批角色').click();
    await button(admin.page, '查看最新审批分配').click();
    await admin.page.getByRole('region', { name: '服务器最新审批分配', exact: true }).getByText(priceRole.name, { exact: true }).waitFor();
    await admin.page.getByRole('region', { name: '已选择审批角色', exact: true }).getByRole('heading', { name: '已选择 0 个审批角色', exact: true }).waitFor();
    await admin.page.setViewportSize({ width: 390, height: 844 });
    await admin.page.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' });
    assert.ok(await admin.page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    await shot(admin.page, 'table-assignment-conflict-390.png');
    await admin.page.getByRole('checkbox', { name: `选择审批角色 ${opsRole.name}`, exact: true }).check();
    await button(admin.page, '保存表审批角色').click();
    await admin.page.getByRole('dialog', { name: '表审批角色', exact: true }).waitFor({ state: 'detached' });
    check('390px 表分配冲突保留空选择，查看最新后明确重选保存');
    const newOrder = async (record, title) => api(applicant.context, base, 'POST', '/api/v1/release-orders', { title, items: tables.map(table_name => ({ table_name, operation: 'MODIFY', id: String(record), expected_record_version: '0', content: { label: `approval-${record}` } })) }, 201);
    const submit = async order => { await applicant.page.goto(`${base}/configuration/release-orders/${order.id}`); await button(applicant.page, '提交审批').click(); await applicant.page.getByRole('region', { name: '提交审批安排', exact: true }).last().waitFor(); await button(applicant.page, '确认提交审批').click(); await applicant.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^待审批$/ }).waitFor(); };
    const order = await newOrder(1, '商品与价格分别审批 · 长标题检查审批范围');
    await submit(order);
    await ops.page.goto(`${base}/configuration/release-orders/${order.id}`);
    await button(ops.page, '批准发布单').click();
    const scope = ops.page.getByRole('region', { name: '本次审批范围', exact: true });
    assert.ok((await scope.innerText()).includes(tables[0])); assert.ok(!(await scope.innerText()).includes(tables[1]));
    await ops.page.getByLabel('审批意见', { exact: true }).fill('商品表已核对，价格表交给财务');
    await button(ops.page, '确认批准').click();
    await ops.page.getByRole('region', { name: '逐表审批进度', exact: true }).getByRole('heading', { name: '已通过 1 / 2 表', exact: true }).waitFor();
    assert.equal(await ops.page.getByRole('list', { name: '发布阶段', exact: true }).getByText('已批准', { exact: true }).count(), 0);
    await shot(ops.page, 'table-approval-partial-desktop.png');
    await applicant.page.reload(); assert.equal(await button(applicant.page, '执行发布').count(), 0);
    await admin.page.goto(`${base}/configuration/release-orders/${order.id}`); assert.equal(await button(admin.page, '批准发布单').count(), 0);
    check('VIEWER 成员只批准商品表，页面显示真实部分进度，ADMIN 无通用越权');
    await finance.page.goto(`${base}/configuration/release-orders/${order.id}`);
    await button(finance.page, '批准发布单').click(); await finance.page.getByLabel('审批意见', { exact: true }).fill('财务确认价格范围');
    await button(finance.page, '确认批准').click(); await finance.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^已批准$/ }).waitFor();
    await applicant.page.reload(); await button(applicant.page, '执行发布').click(); await button(applicant.page, '确认发布到数据库').click(); await applicant.page.getByRole('heading', { name: '数据库发布结果', exact: true }).waitFor();
    check('两个独立角色都通过后，申请人执行真实多表发布');
    // Change membership after the first reviewer has confirmed only one table.
    const conflictOrder = await newOrder(2, '成员变化后必须重审范围'); await submit(conflictOrder);
    await ops.page.goto(`${base}/configuration/release-orders/${conflictOrder.id}`); await button(ops.page, '批准发布单').click(); await ops.page.getByLabel('审批意见', { exact: true }).fill('保留原意见再审阅新增价格表');
    const role = await api(admin.context, base, 'GET', `/api/v1/approval-roles/${priceRole.id}`);
    await api(admin.context, base, 'PUT', `/api/v1/approval-roles/${priceRole.id}`, { name: role.name, description: role.description, enabled: true, member_ids: [finance.identity.accountID, ops.identity.accountID], expected_version: role.version });
    const conflictedResponse = ops.page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith(`/${conflictOrder.id}/approve`));
    await button(ops.page, '确认批准').click();
    const conflictResult = await conflictedResponse;
    assert.equal(conflictResult.status(), 409);
    assert.equal((await conflictResult.json()).error.code, 'release_approval_conflict');
    await ops.page.getByText('审批资格或确认表范围已变化，原意见保留，请重新审阅最新范围。', { exact: true }).waitFor();
    await ops.page.reload(); await ops.page.getByText('查看原申请内容', { exact: true }).click(); await ops.page.getByText('审批意见：保留原意见再审阅新增价格表', { exact: true }).waitFor();
    await button(ops.page, '查看最新状态与配置').click();
    await ops.page.getByRole('region', { name: '本次审批范围', exact: true }).getByText(tables[1], { exact: true }).waitFor();
    await ops.page.setViewportSize({ width: 390, height: 844 }); await ops.page.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' }); await checkReviewGeometry(ops.page);
    await shot(ops.page, 'table-approval-scope-review-390.png');
    await button(ops.page, '确认按最新状态批准发布单').focus(); await ops.page.keyboard.press('Enter'); await ops.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^已批准$/ }).waitFor();
    check('成员变化返回真实范围冲突，刷新保留意见和原范围，390px 键盘明确批准最新两表');
    // An empty snapshot remains empty after later re-assignment, with a visible default source.
    for (const table of tables) await bind(admin.context, base, table, []);
    const fallback = await newOrder(3, '无角色由独立管理员默认审批'); await submit(fallback);
    for (const table of tables) await bind(admin.context, base, table, [opsRole.id]);
    await admin.page.goto(`${base}/configuration/release-orders/${fallback.id}`);
    await admin.page.getByRole('region', { name: '逐表审批进度', exact: true }).getByText('默认 ADMIN 审批', { exact: false }).first().waitFor();
    await button(admin.page, '批准发布单').click(); await admin.page.getByLabel('审批意见', { exact: true }).fill('根据空分配快照独立接手'); await button(admin.page, '确认批准').click(); await admin.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^已批准$/ }).waitFor();
    await admin.page.getByRole('region', { name: '逐表审批进度', exact: true }).getByText('资格来源：默认 ADMIN', { exact: true }).first().waitFor();
    await shot(admin.page, 'table-approval-default-390.png');
    check('空快照不受后来改绑影响，独立 ADMIN 默认接手并保留真实资格来源');
    assert.deepEqual(errors, []);
    if (output) writeFileSync(join(output, 'result.json'), JSON.stringify({ engine, checks, errors, orders: [unavailableDraft.id, order.id, conflictOrder.id, fallback.id] }, null, 2));
    console.log(JSON.stringify({ engine, checks, errors }));
  } catch (error) {
    if (output) {
      writeFileSync(join(output, 'failure.json'), JSON.stringify({ engine, checks, errors, message: error.message, stack: error.stack }, null, 2));
      for (const [index, page] of surfaces.entries()) {
        await shot(page, `failure-surface-${index}.png`).catch(() => {});
        writeFileSync(join(output, `failure-surface-${index}.txt`), await page.locator('body').innerText().catch(() => 'Page unavailable'));
      }
    }
    throw error;
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
