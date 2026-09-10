// Current field display rules over immutable release request/publication history.
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { join } = require('node:path');
const { mkdir, writeFile } = require('node:fs/promises');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { browserOptions, selectedBrowser, registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');

const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const table = 'stage1_acceptance_items';

(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const checks = [], errors = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  const account = async roles => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await registerFixtureAccount(context, base, { roles });
    return context;
  };
  const api = async (context, method, path, data, expected = 200) => {
    const response = await authenticatedRequest(context, base, path, {
      method,
      headers: { 'Idempotency-Key': randomUUID() },
      ...(data === undefined ? {} : { data }),
    });
    assert.equal(response.status(), expected, `${method} ${path}: ${await response.text()}`);
    return response.json();
  };
  const policy = (displayName, oldValue, oldLabel, newValue, newLabel) => ({ policies: [{
    field_name: 'name',
    display_name: displayName,
    description: '历史页按当前规则展示，原始字段和值始终可核对',
    display_order: 20,
    is_visible: false,
    is_queryable: true,
    query_operators: ['exact'],
    ui_type: 'select',
    ui_options: { options: [{ label: oldLabel, value: oldValue }, { label: newLabel, value: newValue }] },
    editable_on_add: true,
    editable_on_modify: true,
    is_required: true,
    enabled: true,
  }] });
  let page;
  try {
    if (output) await mkdir(output, { recursive: true });
    const applicant = await account(['EDITOR', 'PUBLISHER']);
    const reviewer = await account(['APPROVER']);
    const administrator = await account(['ADMIN']);
    page = await applicant.newPage();
    page.setDefaultTimeout(15000);
    page.on('pageerror', error => errors.push(error.message));

    const queried = await api(applicant, 'POST', `/api/v1/tables/${table}/query`, {
      conditions: [{ field: 'id', operator: 'exact', value: '5' }],
    });
    assert.equal(queried.rows.length, 1);
    const oldValue = queried.rows[0].name;
    const newValue = `field-display-${randomUUID().slice(0, 8)}`;
    await api(administrator, 'PUT', `/api/v1/table-field-policies/${table}`, policy('当前名称', oldValue, '同名选项', newValue, '同名选项'));

    const draft = await api(applicant, 'POST', '/api/v1/release-orders', {
      title: '字段实时展示验收',
      table_name: table,
      items: [{ operation: 'MODIFY', id: '5', expected_record_version: queried.record_versions[0], content: { name: newValue } }],
    }, 201);
    const pending = await api(applicant, 'POST', `/api/v1/release-orders/${draft.id}/submit`, { expected_version: draft.version });
    const approved = await api(reviewer, 'POST', `/api/v1/release-orders/${draft.id}/approve`, { expected_version: pending.version, reason: '独立核对字段显示与真实值' });
    const published = await api(applicant, 'POST', `/api/v1/release-orders/${draft.id}/execute`, { expected_version: approved.version });
    assert.equal(published.state, 'SUCCEEDED');
    const persistedBefore = JSON.stringify(published.publication.commands[0].before);
    const persistedFinal = JSON.stringify(published.publication.commands[0].final);

    await page.goto(`${base}/configuration/release-orders/${draft.id}`);
    await page.getByText(`${table} · 已发布待完结`, { exact: true }).waitFor();
    const result = page.getByRole('region', { name: '发布结果', exact: true });
    await result.getByRole('rowheader', { name: /当前名称\s+name/ }).waitFor();
    assert.ok(await result.getByText('同名选项', { exact: true }).count() >= 2);
    await result.getByLabel(`真实值：${oldValue}`, { exact: true }).waitFor();
    await result.getByLabel(`真实值：${newValue}`, { exact: true }).waitFor();
    check('a hidden configured field remains in persisted actual before/final and duplicate labels retain both raw values');

    await api(administrator, 'PUT', `/api/v1/table-field-policies/${table}`, policy('最新名称', oldValue, '原始名称', newValue, '目标名称'));
    await page.reload();
    await result.getByRole('rowheader', { name: /最新名称\s+name/ }).waitFor();
    await result.getByText('原始名称', { exact: true }).waitFor();
    await result.getByText('目标名称', { exact: true }).waitFor();
    await result.getByLabel(`真实值：${oldValue}`, { exact: true }).waitFor();
    await result.getByLabel(`真实值：${newValue}`, { exact: true }).waitFor();
    const afterPolicyChange = await api(applicant, 'GET', `/api/v1/release-orders/${draft.id}`);
    assert.equal(JSON.stringify(afterPolicyChange.publication.commands[0].before), persistedBefore);
    assert.equal(JSON.stringify(afterPolicyChange.publication.commands[0].final), persistedFinal);
    check('reopening history reads new names and option labels without changing persisted before/final');

    await page.getByRole('button', { name: '申请差异', exact: true }).click();
    const request = page.getByRole('region', { name: '变更内容', exact: true });
    await request.getByRole('rowheader', { name: /最新名称\s+name/ }).waitFor();
    await request.getByText('原始名称', { exact: true }).waitFor();
    await request.getByText('目标名称', { exact: true }).waitFor();
    await request.getByLabel(`真实值：${oldValue}`, { exact: true }).waitFor();
    await request.getByLabel(`真实值：${newValue}`, { exact: true }).waitFor();
    check('request review ignores list visibility while showing current labels and immutable requested values');

    if (output) await page.screenshot({ path: join(output, 'field-display-request-desktop.png'), fullPage: true, animations: 'disabled' });
    const policyRoute = `**/api/v1/table-field-policies/${table}`;
    await page.route(policyRoute, route => route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'field_policy_unavailable', message: 'temporary field display failure', request_id: 'field-display-browser-request' } }),
    }));
    await page.reload();
    const alert = page.getByRole('alert').filter({ hasText: '字段配置暂时不可用' });
    await alert.waitFor();
    await alert.getByText('请求编号：field-display-browser-request', { exact: true }).waitFor();
    await page.getByText(`值：${oldValue}`, { exact: true }).first().waitFor();
    await page.getByText(`值：${newValue}`, { exact: true }).first().waitFor();
    await page.unroute(policyRoute);
    await alert.getByRole('button', { name: '重试', exact: true }).click();
    await page.getByText('最新名称', { exact: true }).first().waitFor();
    await page.getByLabel(`真实值：${newValue}`, { exact: true }).first().waitFor();
    check('metadata failure keeps raw request history, diagnostics and an independent retry');

    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    if (output) await page.screenshot({ path: join(output, 'field-display-request-mobile.png'), fullPage: true, animations: 'disabled' });
    assert.deepEqual(errors, []);
    if (output) await writeFile(join(output, 'result.json'), JSON.stringify({ browser: browser.version(), checks, errors, order_id: draft.id }, null, 2));
    console.log(JSON.stringify({ checks, errors, order_id: draft.id }));
  } catch (error) {
    if (page && output) {
      await page.screenshot({ path: join(output, 'failure.png'), fullPage: true }).catch(() => {});
      await writeFile(join(output, 'failure.txt'), await page.locator('body').innerText().catch(() => 'unavailable'));
    }
    throw error;
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
