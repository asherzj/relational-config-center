// #97 / AC-019..020: formal Web → Admin process → disposable MySQL.
// Each engine runs on a fresh runNotificationBrowserSystemPath fixture.
const assert = require('node:assert/strict');
const { join } = require('node:path');
const { mkdirSync, writeFileSync } = require('node:fs');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { selectedBrowser, browserOptions, registerFixtureAccount } = require('./local-account.cjs');
const { approvalFixtureRequest: api, createFixtureApprovalRole: createRole, bindFixtureApprovalRoles: bind, fixtureApprovalInput } = require('./table-approval-fixture.cjs');
const { readAllReleaseDetailPages } = require('./release-detail-pages.cjs');
const { repeatReleaseAction } = require('./release-original-action.cjs');
const base = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
const tables = ['multitable_browser_a', 'multitable_browser_b'];
const button = (page, name) => page.getByRole('button', { name, exact: true });
const stateLabels = { APPROVED: '已批准', SUCCEEDED: '已发布待完结', COMPLETED: '已完结', ROLLED_BACK: '已回滚', CANCELLED: '已取消', DRAFT: '草稿' };

(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const checks = [], errors = [], people = [], acknowledgements = [], replays = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  if (output) mkdirSync(output, { recursive: true });
  const person = async (name, roles) => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const identity = await registerFixtureAccount(context, base, { roles });
    const page = await context.newPage(); page.setDefaultTimeout(15000);
    page.on('pageerror', error => errors.push(`${name}: ${error.message}`));
    const result = { name, context, identity, page }; people.push(result); return result;
  };
  const read = (who, id) => api(who.context, base, 'GET', `/api/v1/release-orders/${id}`);
  const counts = who => api(who.context, base, 'GET', '/api/v1/approval-notifications');
  const notifications = async id => Object.fromEntries(await Promise.all(people.map(async who => [who.name, (await read(who, id)).notification])));
  const state = async (page, order, expected) => {
    await page.getByRole('heading', { name: order.title, exact: true }).waitFor();
    await page.getByLabel('发布单状态', { exact: true }).filter({ hasText: new RegExp(`^${stateLabels[expected]}$`) }).waitFor();
  };
  const screenshot = async (page, name) => {
    await page.evaluate(() => window.scrollTo(0, 0));
    const geometry = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth }));
    assert.ok(geometry.document <= geometry.viewport, `${name}: ${JSON.stringify(geometry)}`);
    if (output) {
      await page.screenshot({ path: join(output, `${name}.png`), fullPage: true, animations: 'disabled' });
      writeFileSync(join(output, `${name}-layout.json`), JSON.stringify(geometry, null, 2));
    }
  };
  const park = async who => {
    await who.page.goto(`${base}/configuration/notifications?view=all`);
    await button(who.page, '查询审批').waitFor();
  };
  const query = (who, table) => api(who.context, base, 'POST', `/api/v1/tables/${table}/query`, {
    conditions: [], order: { field: 'id', direction: 'ASC' }, page_size: 20, page_number: 1,
  });
  // Include actual per-detail publication/rollback commands, both table rows,
  // versions, both kinds of notifications and history when comparing a replay.
  const facts = async (who, id) => {
    const header = await read(who, id);
    const order = await readAllReleaseDetailPages(who.context, base, header);
    return {
      id: order.id, state: order.state, version: order.version, items: order.items,
      executions: order.executions, history: order.history,
      notifications: await notifications(id),
      tables: await Promise.all(tables.map(table => query(who, table))),
    };
  };
  const resultRecipients = async (id, before, recipients) => {
    const after = await notifications(id);
    for (const who of people) {
      const receives = recipients.includes(who);
      assert.equal(after[who.name].sequence, String(BigInt(before[who.name].sequence) + (receives ? 1n : 0n)), `${who.name}: one result per order`);
      assert.equal(after[who.name].unread, receives, `${who.name}: result recipient`);
      assert.equal(after[who.name].pending, false, `${who.name}: result is not pending approval`);
      assert.deepEqual(await counts(who), { unread_count: receives ? 1 : 0, pending_count: 0 }, `${who.name}: aggregated counts`);
    }
    return after;
  };
  // Open the real unread row by keyboard, acknowledge the exact displayed
  // sequence and return through the real notification-list navigation.
  const openNotification = async (who, order, expected, options = {}) => {
    const before = (await read(who, order.id)).notification;
    assert.equal(before.unread, true, `${who.name}: expected an unread result`);
    const view = who.identity.accountID === order.applicant_id ? 'mine' : 'handled';
    await who.page.goto(`${base}/configuration/notifications?view=${view}&unread=true&id=${order.id}`);
    const row = who.page.getByRole('row').filter({ hasText: order.title });
    await row.getByText('未读', { exact: true }).waitFor();
    assert.equal(await row.count(), 1, 'all table results aggregate to one order row');
    await row.getByText(stateLabels[expected], { exact: false }).first().waitFor();
    if (options.shot) await screenshot(who.page, `${options.shot}-list`);
    const ack = who.page.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith(`/${order.id}/notification-read`));
    const link = row.getByRole('link', { name: `查看详情：${order.title}`, exact: true });
    await link.focus(); await who.page.keyboard.press('Enter');
    const response = await ack;
    assert.equal(response.status(), 200, await response.text());
    assert.deepEqual(response.request().postDataJSON(), { sequence: before.sequence });
    assert.equal((await response.json()).unread, false);
    await state(who.page, order, expected);
    assert.equal(new URL(who.page.url()).pathname, `/configuration/notifications/${order.id}`);
    await who.page.getByText('已读。', { exact: true }).waitFor();
    assert.deepEqual(await counts(who), { unread_count: 0, pending_count: 0 });
    if (options.verify) await options.verify(who.page);
    if (options.shot) await screenshot(who.page, `${options.shot}-detail`);
    acknowledgements.push({ account: who.name, order_id: order.id, state: expected, sequence: before.sequence });
    await who.page.getByRole('link', { name: '返回通知中心列表', exact: true }).click();
    assert.equal(new URL(who.page.url()).searchParams.get('view'), view);
    assert.equal(new URL(who.page.url()).searchParams.get('unread'), 'true');
    assert.equal(new URL(who.page.url()).searchParams.get('id'), order.id);
    await who.page.getByText('没有符合筛选条件的发布单。', { exact: true }).waitFor();
  };
  const revealResultActor = async (result, actorID) => {
    const person = result.getByRole('button', { name: /^(查看|收起).+的账号信息$/ });
    await person.waitFor();
    if (await person.getAttribute('aria-expanded') === 'false') await person.click();
    assert.equal(await person.getAttribute('aria-expanded'), 'true');
    await result.getByRole('group', { name: /的账号信息$/ }).getByText(actorID, { exact: true }).waitFor();
  };
  const publishedResult = (labels, actorID) => async page => {
    const result = page.getByRole('region', { name: '发布结果', exact: true });
    await result.getByRole('heading', { name: '数据库发布结果', exact: true }).waitFor();
    await revealResultActor(result, actorID);
    for (const label of labels) await result.getByText(`值：${label}`, { exact: true }).waitFor();
    assert.equal(await result.getByText(/分发尚未接入/).count(), 2, 'refresh notifications do not imply consumer delivery');
  };
  // The route sends the real write to Admin first, proves the committed state,
  // then loses only its response. Recovery clicks the original action after
  // reload; no API helper issues or substitutes the retry.
  const lostResponse = async (who, order, { endpoint, action, confirmation, expected, status = 200, inspect, shot }) => {
    const path = `/api/v1/release-orders/${order.id}/${endpoint}`, routePath = `**${path}`;
    const requests = [];
    const listener = request => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === path)
        requests.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
    };
    who.context.on('request', listener);
    let committed;
    await who.page.route(routePath, async route => {
      const response = await route.fetch();
      assert.equal(response.status(), status, await response.text());
      committed = await response.json(); assert.equal(committed.state, expected);
      await route.abort('failed');
    });
    await button(who.page, confirmation).dblclick();
    await who.page.getByText('Admin 连接或响应传输中断。', { exact: true }).last().waitFor();
    assert.equal(requests.length, 1, 'double click commits one request');
    const beforeReplay = await inspect(committed);
    assert.equal(requests.length, 1, 'reading real facts does not trigger an automatic retry');
    if (shot) {
      await who.page.setViewportSize({ width: 390, height: 844 });
      await screenshot(who.page, shot);
      await who.page.setViewportSize({ width: 1440, height: 1000 });
    }
    await who.page.unroute(routePath);
    who.page.once('dialog', dialog => dialog.accept());
    await who.page.reload();
    const replay = who.page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === path);
    await repeatReleaseAction(who.page, action, confirmation);
    const response = await replay;
    assert.equal(response.status(), status, await response.text());
    assert.equal((await response.json()).id, committed.id);
    assert.equal(requests.length, 2, 'only the explicit original operation resends');
    assert.ok(requests[0].key, 'the original write has an idempotency key');
    assert.deepEqual(requests[1], requests[0], 'reload retains the original key and byte-identical body');
    assert.deepEqual(await inspect(committed), beforeReplay, 'replay changes no rows, versions, execution, history or notification sequence');
    who.context.off('request', listener);
    replays.push({ action, source_order_id: order.id, result_order_id: committed.id, requests, notifications: beforeReplay.notifications ?? beforeReplay.source.notifications });
    return committed;
  };
  try {
    const admin = await person('admin', ['ADMIN']);
    const applicant = await person('applicant', ['EDITOR']);
    const reviewer = await person('former-reviewer', ['VIEWER']);
    const reviewerPublisher = await person('reviewer-publisher', ['VIEWER', 'PUBLISHER']);
    const publisher = await person('publisher', ['PUBLISHER']);
    await person('unrelated-viewer', ['VIEWER']);
    const mutation = 'release_notifications_browser_v1';
    await api(admin.context, base, 'POST', '/api/v1/mutation-policies', { code: mutation, name: '发布全过程结果提醒验收', description: '', type_code: 'single_table_mutation', allow_add: true, allow_modify: true, allow_delete: true }, 201);
    await api(admin.context, base, 'POST', `/api/v1/mutation-policies/${mutation}/activate`, {});
    for (const table of tables) {
      const policy = await api(admin.context, base, 'POST', '/api/v1/table-policies', { table_name: table, query_policy_code: 'notification_page_query_v1', mutation_policy_code: mutation }, 201);
      await api(admin.context, base, 'POST', `/api/v1/table-policies/${table}/enable`, { expected_version: policy.version });
      const associations = (await api(admin.context, base, 'GET', `/api/v1/table-policies/${table}/release-templates`)).associations;
      const standard = associations.find(association => association.type === 'STANDARD');
      await api(admin.context, base, 'PUT', `/api/v1/table-policies/${table}/release-templates/STANDARD`, { template_code: 'default_standard_v1', enabled: true, expected_version: standard?.version ?? '0' });
    }
    const firstRole = await createRole(admin.context, base, '商品表真实审批参与者，审批完成后移除角色成员仍应收到发布结果', [reviewer.identity.accountID]);
    const secondRole = await createRole(admin.context, base, '价格表审批与发布兼任人员', [reviewerPublisher.identity.accountID]);
    await bind(admin.context, base, tables[0], [firstRole.id]);
    await bind(admin.context, base, tables[1], [secondRole.id]);
    const members = async (role, ids) => {
      const current = await api(admin.context, base, 'GET', `/api/v1/approval-roles/${role.id}`);
      await api(admin.context, base, 'PUT', `/api/v1/approval-roles/${role.id}`, { name: current.name, description: current.description, enabled: current.enabled, expected_version: current.version, member_ids: ids });
    };
    const approvedDraft = async (recordID, title, labels) => {
      await members(firstRole, [reviewer.identity.accountID]);
      await members(secondRole, [reviewerPublisher.identity.accountID]);
      let order = await api(applicant.context, base, 'POST', '/api/v1/release-orders', {
        title, items: tables.map((table_name, index) => ({ table_name, operation: 'MODIFY', id: recordID, expected_record_version: '0', content: { label: labels[index] } })),
      }, 201);
      order = await api(applicant.context, base, 'POST', `/api/v1/release-orders/${order.id}/submit`, { expected_version: order.version });
      for (const who of [reviewer, reviewerPublisher]) {
        order = await api(who.context, base, 'POST', `/api/v1/release-orders/${order.id}/approve`, await fixtureApprovalInput(who.context, base, order.id, { expected_version: order.version, reason: `${who.name} 独立核对本人负责的表` }));
      }
      assert.equal(order.state, 'APPROVED');
      assert.equal(order.history.filter(event => event.action === 'APPROVE').length, 2);
      await members(firstRole, []); await members(secondRole, []);
      await openNotification(applicant, order, 'APPROVED');
      for (const who of people) assert.deepEqual(await counts(who), { unread_count: 0, pending_count: 0 });
      return order;
    };
    const completionLabels = ['notification-publication-a', 'notification-publication-b'];
    const completeOrder = await approvedDraft('1', '多表发布与完结提醒：真实审批参与者撤权后继续查看完整结果和处理历史', completionLabels);
    const beforePublish = await notifications(completeOrder.id);
    await reviewerPublisher.page.goto(`${base}/configuration/release-orders/${completeOrder.id}`);
    await button(reviewerPublisher.page, '执行发布').click();
    const committedPublication = await lostResponse(reviewerPublisher, completeOrder, {
      endpoint: 'execute', action: '执行发布', confirmation: '确认发布到数据库', expected: 'SUCCEEDED', shot: 'publication-lost-response-390',
      inspect: async () => { await resultRecipients(completeOrder.id, beforePublish, [applicant, reviewer]); return facts(publisher, completeOrder.id); },
    });
    await state(reviewerPublisher.page, completeOrder, 'SUCCEEDED');
    assert.equal(committedPublication.executions.length, 1);
    assert.equal(committedPublication.executions[0].actor_id, reviewerPublisher.identity.accountID);
    await park(reviewerPublisher);
    await openNotification(applicant, completeOrder, 'SUCCEEDED', { shot: 'publication-applicant-desktop', verify: publishedResult(completionLabels, reviewerPublisher.identity.accountID) });
    await reviewer.page.setViewportSize({ width: 390, height: 844 });
    await openNotification(reviewer, completeOrder, 'SUCCEEDED', { shot: 'publication-former-reviewer-390', verify: publishedResult(completionLabels, reviewerPublisher.identity.accountID) });
    check('发布真实提交后丢响应，原 UI 原键原正文重推一次；申请人和撤权审批者各聚合一项，兼任发布的审批者不提醒自己，桌面与 390px 键盘跳转核对实际结果并 ACK');

    const beforeComplete = await notifications(completeOrder.id), publishedFacts = await facts(publisher, completeOrder.id);
    await publisher.page.goto(`${base}/configuration/release-orders/${completeOrder.id}`);
    await button(publisher.page, '完结发布单').click();
    await lostResponse(publisher, completeOrder, {
      endpoint: 'complete', action: '完结发布单', confirmation: '确认完结', expected: 'COMPLETED',
      inspect: async () => { await resultRecipients(completeOrder.id, beforeComplete, [applicant, reviewer, reviewerPublisher]); return facts(publisher, completeOrder.id); },
    });
    await state(publisher.page, completeOrder, 'COMPLETED');
    const completedFacts = await facts(publisher, completeOrder.id);
    assert.deepEqual(completedFacts.tables, publishedFacts.tables);
    assert.deepEqual(completedFacts.executions, publishedFacts.executions);
    assert.equal(completedFacts.history.filter(event => event.action === 'COMPLETE').length, 1);
    await park(publisher);
    for (const who of [applicant, reviewer, reviewerPublisher]) await openNotification(who, completeOrder, 'COMPLETED', {
      ...(who === reviewer ? { shot: 'completion-former-reviewer-390' } : {}), verify: publishedResult(completionLabels, reviewerPublisher.identity.accountID),
    });
    check('独立发布者完结并重推，申请人及两位历史审批者收到完结结果；不改配置、不重复执行或下游刷新记录，已处理视图与已读返回正确');

    const rollbackLabels = ['notification-rollback-a', 'notification-rollback-b'];
    const rollbackOrder = await approvedDraft('2', '原单快速回滚结果提醒：保留真实发布结果、数据库恢复值与审批人身份', rollbackLabels);
    const beforeRollbackPublication = await notifications(rollbackOrder.id);
    await publisher.page.goto(`${base}/configuration/release-orders/${rollbackOrder.id}`);
    await button(publisher.page, '执行发布').click(); await button(publisher.page, '确认发布到数据库').click();
    await state(publisher.page, rollbackOrder, 'SUCCEEDED');
    await resultRecipients(rollbackOrder.id, beforeRollbackPublication, [applicant, reviewer, reviewerPublisher]);
    await park(publisher);
    for (const who of [applicant, reviewer, reviewerPublisher]) await openNotification(who, rollbackOrder, 'SUCCEEDED');
    const beforeRollback = await notifications(rollbackOrder.id);
    await reviewerPublisher.page.goto(`${base}/configuration/release-orders/${rollbackOrder.id}`);
    await button(reviewerPublisher.page, '快速回滚').click();
    await reviewerPublisher.page.getByRole('region', { name: '整单恢复预览', exact: true }).waitFor();
    await reviewerPublisher.page.getByLabel('快速回滚原因（选填）', { exact: true }).fill('恢复两表已核对的原值');
    const rolledBack = await lostResponse(reviewerPublisher, rollbackOrder, {
      endpoint: 'quick-rollback', action: '快速回滚', confirmation: '确认整单快速回滚', expected: 'ROLLED_BACK', shot: 'rollback-lost-response-390',
      inspect: async () => { await resultRecipients(rollbackOrder.id, beforeRollback, [applicant, reviewer]); return facts(publisher, rollbackOrder.id); },
    });
    await state(reviewerPublisher.page, rollbackOrder, 'ROLLED_BACK');
    assert.equal(rolledBack.id, rollbackOrder.id, 'rollback finishes the original order');
    assert.deepEqual(rolledBack.executions.map(execution => execution.kind), ['PUBLICATION', 'ROLLBACK']);
    assert.equal(rolledBack.executions[1].actor_id, reviewerPublisher.identity.accountID);
    assert.equal(rolledBack.history.filter(event => event.action === 'APPROVE').length, 2);
    assert.equal(rolledBack.history.filter(event => event.action === 'QUICK_ROLLBACK').length, 1);
    for (const table of tables) {
      const rows = await query(publisher, table), index = rows.rows.findIndex(row => row.id === '2');
      assert.equal(rows.rows[index].label, 'before-2'); assert.equal(rows.record_versions[index], '2');
    }
    await park(reviewerPublisher);
    const restoredResult = async page => {
      await publishedResult(rollbackLabels, publisher.identity.accountID)(page);
      const switcher = button(page, '恢复结果'); await switcher.focus(); await page.keyboard.press('Enter');
      const result = page.getByRole('region', { name: '发布结果', exact: true });
      await result.getByRole('heading', { name: '数据库恢复结果', exact: true }).waitFor();
      await revealResultActor(result, reviewerPublisher.identity.accountID);
      assert.equal(await result.getByText('值：before-2', { exact: true }).count(), 2);
      assert.equal(await result.getByText(/分发尚未接入/).count(), 2);
      if (page.viewportSize().width === 390) {
        const table = result.getByRole('region', { name: `明细 1 ${tables[0]} 实际结果，可横向滚动`, exact: true });
        await table.focus();
        for (let step = 0; step < 8; step++) await page.keyboard.press('ArrowRight');
        assert.ok(await table.evaluate(element => element.scrollLeft > 0), 'keyboard reaches the mobile restoration columns');
      }
    };
    await openNotification(applicant, rollbackOrder, 'ROLLED_BACK', { verify: restoredResult, shot: 'rollback-applicant-desktop' });
    await openNotification(reviewer, rollbackOrder, 'ROLLED_BACK', { verify: restoredResult, shot: 'rollback-former-reviewer-390' });
    check('原单快速回滚丢响应后原操作重推，真实两表只恢复一次；申请人与撤权审批者收到原单结果，键盘切换持久化发布和恢复值，真实回滚操作者无自身提醒');

    const replacementLabels = ['notification-reprepare-a', 'notification-reprepare-b'];
    const source = await approvedDraft('3', '管理员重新准备多表原申请，申请人收到取消结果而新草稿尚未通知审批人', replacementLabels);
    const beforeReprepare = await notifications(source.id);
    await admin.page.goto(`${base}/configuration/release-orders/${source.id}`);
    await button(admin.page, '重新准备').click(); await button(admin.page, '读取最新配置').click();
    await admin.page.getByText(`明细 2 · ${tables[1]} · MODIFY · 记录 3`, { exact: true }).waitFor();
    await button(admin.page, '继续重新准备').click();
    const replacement = await lostResponse(admin, source, {
      endpoint: 'reprepare', action: '重新准备', confirmation: '取消旧单并创建新草稿', expected: 'DRAFT', status: 201, shot: 'reprepare-lost-response-390',
      inspect: async committed => {
        await resultRecipients(source.id, beforeReprepare, [applicant]);
        const draft = await facts(admin, committed.id);
        assert.equal(draft.state, 'DRAFT');
        for (const notification of Object.values(draft.notifications)) assert.deepEqual(notification, { sequence: '0', unread: false, pending: false });
        return { source: await facts(admin, source.id), replacement: draft };
      },
    });
    await admin.page.waitForURL(`**/configuration/release-orders/${replacement.id}`);
    await state(admin.page, source, 'DRAFT');
    assert.equal(replacement.applicant_id, admin.identity.accountID);
    assert.equal(replacement.copied_from_id, source.id);
    const cancelled = await read(applicant, source.id);
    assert.equal(cancelled.state, 'CANCELLED');
    assert.equal(cancelled.history.filter(event => event.action === 'REPREPARE').length, 1);
    assert.equal(replacement.history.filter(event => event.action === 'REPREPARE').length, 1);
    await admin.page.setViewportSize({ width: 390, height: 844 });
    await screenshot(admin.page, 'reprepare-new-draft-390');
    await applicant.page.setViewportSize({ width: 390, height: 844 });
    await openNotification(applicant, source, 'CANCELLED', { shot: 'reprepare-cancellation-applicant-390', verify: async page => {
      await page.getByRole('link', { name: replacement.id, exact: true }).waitFor();
      assert.equal(await page.getByRole('heading', { name: '数据库发布结果', exact: true }).count(), 0);
    } });
    await admin.page.goto(`${base}/configuration/notifications?view=all&id=${replacement.id}`);
    await admin.page.getByText('没有符合筛选条件的发布单。', { exact: true }).waitFor();
    assert.equal(await admin.page.getByRole('link', { name: `查看详情：${source.title}`, exact: true }).count(), 0, 'the unsubmitted replacement is absent from the notification center');
    for (const who of people) assert.deepEqual(await counts(who), { unread_count: 0, pending_count: 0 });
    check('管理员重新准备真实提交后丢响应，原键原正文重推只生成一个替代草稿；取消仅提醒原申请人，新草稿未提交零待办零提醒，手机可读旧单关联');
    assert.deepEqual(errors, []);
    if (output) writeFileSync(join(output, 'result.json'), JSON.stringify({ checks, engine: process.env.RCC_E2E_ENGINE || 'chromium', acknowledgements, replays, errors }, null, 2));
    console.log(JSON.stringify({ checks, acknowledgements: acknowledgements.length, replayed_operations: replays.map(replay => replay.action), errors }));
  } catch (error) {
    if (output) for (const who of people) {
      await who.page.screenshot({ path: join(output, `failure-${who.name}.png`), fullPage: true }).catch(() => {});
      writeFileSync(join(output, `failure-${who.name}.txt`), await who.page.locator('body').innerText().catch(() => 'page unavailable'));
    }
    throw error;
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
