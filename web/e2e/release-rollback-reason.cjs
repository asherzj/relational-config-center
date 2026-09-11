const { createFixtureApprovalRole, fixtureApprovalInput } = require('./table-approval-fixture.cjs');
const {readAllReleaseDetailPages,executionCommands,applicationItems}=require('./release-detail-pages.cjs');
// Real Chromium → same-origin Admin → isolated MySQL acceptance for #87.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { join } = require('node:path');
const { mkdir, writeFile } = require('node:fs/promises');
const { browserOptions, selectedBrowser, registerFixtureAccount, setFixtureRoles } = require('./local-account.cjs');

const base = process.env.RCC_WEB_URL;
const engineName = process.env.RCC_E2E_ENGINE || 'chromium';
const table = `stage1_rollback_${engineName}_items`;
const output = process.env.RCC_E2E_OUTPUT;

(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const errors = [], checks = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  const identity = async context => (await context.request.get(`${base}/api/v1/auth/session`)).json();
  const api = async (context, method, path, data, expected = 200, key = randomUUID()) => {
    const session = await identity(context);
    const response = await context.request.fetch(`${base}${path}`, {
      method,
      headers: { Origin: base, 'X-CSRF-Token': session.csrf_token, 'Idempotency-Key': key },
      ...(data === undefined ? {} : { data }),
      timeout: 20000,
    });
    assert.equal(response.status(), expected, `${method} ${path}: ${await response.text()}`);
    return response.json();
  };
  const account = async roles => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const fixture = await registerFixtureAccount(context, base, { roles });
    return { context, fixture };
  };
  const assertViewportLayout = async (page, name) => {
    const layout = await page.evaluate(() => ({
      viewport: innerWidth,
      document: document.documentElement.scrollWidth,
      overflowing: [...document.querySelectorAll('body *')].flatMap(element => {
        const rect = element.getBoundingClientRect();
        return rect.width > 0 && rect.right > innerWidth + 1 ? [{
          tag: element.tagName, role: element.getAttribute('role'), text: element.textContent.slice(0, 100), right: rect.right,
        }] : [];
      }).slice(0, 20),
    }));
    if (output) await writeFile(join(output, `${name}-layout.json`), JSON.stringify(layout, null, 2));
    assert.ok(layout.document <= layout.viewport, `${name} overflows mobile: ${JSON.stringify(layout)}`);
  };

  let executorPage, adminPage;
  try {
    if (output) await mkdir(output, { recursive: true });
    const applicant = await account(['EDITOR']);
    const reviewer = await account(['VIEWER']);
    const forwardPublisher = await account(['PUBLISHER']);
    const executor = await account(['PUBLISHER']);
    const unrelated = await account(['VIEWER']);
    const administrator = await account(['ADMIN']);
    await createFixtureApprovalRole(administrator.context, base, `Rollback reason review ${randomUUID()}`, [reviewer.fixture.accountID], [table]);

    const baseline = await api(applicant.context, 'POST', `/api/v1/tables/${table}/query`, {
      conditions: [{ field: 'id', operator: 'exact', value: '1' }],
    });
    assert.equal(baseline.rows.length, 1);

    const draft = await api(applicant.context, 'POST', '/api/v1/release-orders', {
      title: '浏览器回滚原因留痕', items:[{table_name:table,operation: 'MODIFY', id: '1', expected_record_version: baseline.record_versions[0], content: { name: 'Published before reason correction' } }],
    }, 201);
    const submitted = await api(applicant.context, 'POST', `/api/v1/release-orders/${draft.id}/submit`, { expected_version: draft.version });
    const approved = await api(reviewer.context, 'POST', `/api/v1/release-orders/${draft.id}/approve`, await fixtureApprovalInput(reviewer.context, base, draft.id, { expected_version: submitted.version, reason: 'Independent review' }));
    const published = await api(forwardPublisher.context, 'POST', `/api/v1/release-orders/${draft.id}/execute`, { expected_version: approved.version });
    const preview = await api(executor.context, 'POST', `/api/v1/release-orders/${draft.id}/quick-rollback/preview`, { expected_version: published.version });
    const rolled = await api(executor.context, 'POST', `/api/v1/release-orders/${draft.id}/quick-rollback`, { expected_version: published.version, preview_digest: preview.preview_digest, reason: '' });
    assert.equal(rolled.state, 'ROLLED_BACK');
    assert.notEqual(rolled.applicant_id, rolled.executions[0].actor_id);
    assert.notEqual(rolled.executions[0].actor_id, rolled.executions[1].actor_id);
    await setFixtureRoles(administrator.context, base, executor.fixture.accountID, ['VIEWER']);

    for (const denied of [applicant.context, forwardPublisher.context, unrelated.context]) {
      const deniedPage = await denied.newPage();
      await deniedPage.goto(`${base}/configuration/release-orders/${draft.id}`);
      await deniedPage.getByRole('region', { name: '回滚原因', exact: true }).waitFor();
      assert.equal(await deniedPage.getByRole('button', { name: /回滚原因/ }).count(), 0);
      await deniedPage.close();
    }
    check('applicant, forward publisher and unrelated viewer have no rollback reason edit entry');

    executorPage = await executor.context.newPage();
    executorPage.setDefaultTimeout(15000);
    executorPage.on('pageerror', error => errors.push(error.message));
    const reasonWrites = [];
    executor.context.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith(`/${draft.id}/rollback-reason`)) {
        reasonWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
      }
    });
    await executorPage.goto(`${base}/configuration/release-orders/${draft.id}`);
    const reasonPanel = executorPage.getByRole('region', { name: '回滚原因', exact: true });
    await reasonPanel.getByText('尚未填写回滚原因。', { exact: true }).waitFor();
    const openReason = reasonPanel.getByRole('button', { name: '补填回滚原因', exact: true });
    await openReason.focus();
    await executorPage.keyboard.press('Enter');
    const executorDialog = executorPage.getByRole('dialog', { name: '补填回滚原因 · 浏览器回滚原因留痕', exact: true });
    const executorInput = executorDialog.getByLabel('回滚原因（选填）', { exact: true });
    assert.equal(await executorInput.getAttribute('required'), null);
    assert.equal(await executorDialog.getByRole('button', { name: '取消', exact: true }).evaluate(element => element === document.activeElement), true);
    await executorInput.fill('不会横向溢出的长回滚原因'.repeat(20));
    await assertViewportLayout(executorPage, 'rollback-reason-editor-desktop');
    const desktopSave = await executorDialog.getByRole('button', { name: '保存回滚原因', exact: true }).boundingBox();
    assert.ok(desktopSave && desktopSave.x >= 0 && desktopSave.x + desktopSave.width <= 1441, 'desktop save action is outside the viewport');
    if (output) await executorPage.screenshot({ path: join(output, 'rollback-reason-editor-desktop.png'), fullPage: false, animations: 'disabled' });
    await executorPage.setViewportSize({ width: 390, height: 844 });
    await assertViewportLayout(executorPage, 'rollback-reason-editor');
    if (output) await executorPage.screenshot({ path: join(output, 'rollback-reason-editor-mobile.png'), fullPage: false, animations: 'disabled' });
    await executorInput.fill('数据库约束冲突，恢复上一版');
    await executorInput.focus();
    await executorPage.keyboard.press('Tab');
    await executorPage.keyboard.press('Tab');
    const mobileSave = executorDialog.getByRole('button', { name: '保存回滚原因', exact: true });
    assert.equal(await mobileSave.evaluate(element => element === document.activeElement), true);
    const mobileSaveBounds = await mobileSave.boundingBox();
    assert.ok(mobileSaveBounds && mobileSaveBounds.x >= 0 && mobileSaveBounds.x + mobileSaveBounds.width <= 391, 'mobile save action is outside the viewport');
    await executorPage.keyboard.press('Enter');
    await reasonPanel.getByText('数据库约束冲突，恢复上一版', { exact: true }).waitFor();
    assert.equal(reasonWrites.length, 1);
    assert.deepEqual(JSON.parse(reasonWrites[0].body), { reason: '数据库约束冲突，恢复上一版' });
    check('recorded rollback executor keeps the optional editor as VIEWER and saves through the keyboard at 390px');

    adminPage = await administrator.context.newPage();
    adminPage.setDefaultTimeout(15000);
    adminPage.on('pageerror', error => errors.push(error.message));
    const adminWrites = [];
    administrator.context.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith(`/${draft.id}/rollback-reason`)) {
        adminWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
      }
    });
    await adminPage.goto(`${base}/configuration/release-orders/${draft.id}`);
    const adminPanel = adminPage.getByRole('region', { name: '回滚原因', exact: true });
    await adminPanel.getByRole('button', { name: '修改回滚原因', exact: true }).click();
    const adminDialog = adminPage.getByRole('dialog', { name: '修改回滚原因 · 浏览器回滚原因留痕', exact: true });
    const adminInput = adminDialog.getByLabel('回滚原因（选填）', { exact: true });
    await adminInput.fill('确认是外键约束冲突，已恢复上一版');
    const reasonRoute = `**/api/v1/release-orders/${draft.id}/rollback-reason`;
    await adminPage.route(reasonRoute, async route => {
      const response = await route.fetch();
      assert.equal(response.status(), 200, await response.text());
      await route.abort('failed');
    });
    await adminDialog.getByRole('button', { name: '保存回滚原因', exact: true }).click();
    await adminDialog.getByText('Admin 连接或响应传输中断。', { exact: true }).waitFor();
    assert.equal(await adminInput.inputValue(), '确认是外键约束冲突，已恢复上一版');
    assert.equal(adminWrites.length, 1);
    await new Promise(resolve => setTimeout(resolve, 50));
    assert.equal(adminWrites.length, 1, 'unknown response must not auto retry');
    await adminPage.unroute(reasonRoute);
    const replay = adminPage.waitForResponse(response => response.request().method() === 'POST' && response.url().endsWith(`/${draft.id}/rollback-reason`));
    await adminDialog.getByRole('button', { name: '保存回滚原因', exact: true }).click();
    assert.equal((await replay).status(), 200);
    await adminPanel.getByText('确认是外键约束冲突，已恢复上一版', { exact: true }).waitFor();
    assert.equal(adminWrites.length, 2);
    assert.deepEqual(adminWrites[1], adminWrites[0]);

    const finalOrder = await api(administrator.context, 'GET', `/api/v1/release-orders/${draft.id}`);
    const revisions = finalOrder.history.filter(event => event.action === 'ROLLBACK_REASON');
    assert.equal(revisions.length, 2);
    assert.equal(revisions[0].actor_id, executor.fixture.accountID);
    assert.equal(revisions[1].actor_id, administrator.fixture.accountID);
    assert.equal(revisions[0].execution_id, rolled.executions[1].id);
    assert.equal(revisions[1].execution_id, rolled.executions[1].id);
    assert.equal(finalOrder.version, rolled.version);
    assert.equal(finalOrder.updated_at, rolled.updated_at);
    const history = adminPage.getByRole('region', { name: '操作历史', exact: true });
    assert.equal(await history.getByText('修改了回滚原因', { exact: true }).count(), 2);
    await history.getByText('数据库约束冲突，恢复上一版', { exact: true }).waitFor();
    await history.getByText('确认是外键约束冲突，已恢复上一版', { exact: true }).waitFor();
    check('administrator correction preserves input after a lost response and manually replays the same key and body with two actor-bound revisions');

    assert.deepEqual(errors, []);
    if (output) await writeFile(join(output, 'result.json'), JSON.stringify({
      ok: true,
      checks,
      order_id: draft.id,
      rollback_execution_id: rolled.executions[1].id,
      executor_account_id: executor.fixture.accountID,
      administrator_account_id: administrator.fixture.accountID,
      executor_request_count: reasonWrites.length,
      administrator_request_count: adminWrites.length,
      administrator_request_replayed_with_same_key_body: true,
      revisions: revisions.map(event => ({ actor_id: event.actor_id, at: event.at, reason: event.reason, execution_id: event.execution_id })),
      browser_errors: errors,
    }, null, 2));
    console.log(JSON.stringify({ checks }));
  } catch (error) {
    if (output && (adminPage || executorPage)) {
      const page = adminPage || executorPage;
      await page.screenshot({ path: join(output, 'failure.png'), fullPage: false }).catch(() => {});
      await writeFile(join(output, 'failure.txt'), await page.locator('body').innerText()).catch(() => {});
    }
    throw error;
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
