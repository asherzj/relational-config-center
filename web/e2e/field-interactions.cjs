// AC-022: one public UI journey per viewport, backed by isolated Admin/MySQL.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { randomUUID } = require('node:crypto');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { browserOptions, selectedBrowser, registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');

const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const table = process.env.RCC_E2E_TABLE || 'stage1_acceptance_items';
const policyPath = `/api/v1/table-field-policies/${table}`;
const engine = process.env.RCC_E2E_ENGINE || 'chromium';

(async () => {
  await fs.mkdir(output, { recursive: true });
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' });
  const checks = [], errors = [], drafts = [];
  let originalPolicies, page, completed = false;
  const api = async (method, resource, data) => {
    const response = await authenticatedRequest(context, base, resource, {
      method, headers: { 'Idempotency-Key': randomUUID() }, ...(data === undefined ? {} : { data }),
    });
    assert.equal(response.status(), 200, `${method} ${resource}: ${await response.text()}`);
    return response.json();
  };
  const pass = (viewport, name, evidence = {}) => {
    checks.push({ viewport, name, ...evidence });
    console.log('PASS', viewport, name);
  };
  const active = locator => locator.evaluate(node => node === document.activeElement);
  const enter = async locator => { await locator.focus(); await page.keyboard.press('Enter'); };
  const noOverflow = async () => assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'document must not scroll horizontally');
  const reachable = async locator => {
    await locator.focus();
    await page.waitForFunction(node => { const r = node.getBoundingClientRect(); return r.left >= -1 && r.right <= innerWidth + 1 && r.top >= -1 && r.bottom <= innerHeight + 1; }, await locator.elementHandle());
    await noOverflow();
  };
  const keyboardScroll = async (region, narrow) => {
    await region.focus();
    // Start immediately before the region, then reach it using keyboard navigation.
    // Engines may add an implicit tab stop for an overflowing parent container.
    await page.keyboard.press('Shift+Tab');
    const tabPath = [];
    for (let attempt = 0; attempt < 12; attempt++) {
      await page.keyboard.press('Tab');
      tabPath.push(await page.evaluate(() => ({ tag: document.activeElement.tagName, label: document.activeElement.getAttribute('aria-label') })));
      if (await active(region)) break;
    }
    assert.equal(await active(region), true, `scroll region must be keyboard reachable: ${JSON.stringify(tabPath)}`);
    const metrics = await region.evaluate(node => {
      const style = getComputedStyle(node);
      return { client: node.clientWidth, total: node.scrollWidth, outlineWidth: style.outlineWidth, outlineStyle: style.outlineStyle };
    });
    assert.equal(metrics.outlineStyle, 'solid'); assert.ok(parseFloat(metrics.outlineWidth) >= 2);
    if (narrow) {
      assert.ok(metrics.total > metrics.client, 'long comparison must overflow its local region');
      await page.keyboard.press('ArrowRight');
      await page.waitForFunction(label => document.querySelector(`[aria-label="${label}"]`).scrollLeft > 0, await region.getAttribute('aria-label'));
    }
    await noOverflow();
    return { ...metrics, keyboardScrollLeft: await region.evaluate(node => node.scrollLeft) };
  };
  try {
    await registerFixtureAccount(context, base);
    originalPolicies = (await api('GET', policyPath)).fields.flatMap(field => field.policy ? [field.policy] : []);
    page = await context.newPage(); page.setDefaultTimeout(15000);
    page.on('pageerror', error => errors.push(error.message));
    for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
      const label = `${viewport.width}x${viewport.height}`;
      await page.setViewportSize(viewport);
      // Restore only our field configuration between journeys; each begins at the real administrator UI.
      await api('PUT', policyPath, { policies: originalPolicies });
      await page.goto(`${base}/platform/table-policies`);
      const row = page.getByRole('row').filter({ has: page.getByText(table, { exact: true }) });
      const entry = row.getByRole('button', { name: '字段配置', exact: true });
      await enter(entry);
      const fields = page.getByRole('combobox', { name: '真实字段', exact: true });
      await fields.selectOption('name');
      await enter(page.getByRole('button', { name: '配置此字段', exact: true }));
      await page.getByLabel('显示名称', { exact: true }).fill('配置名称');
      await page.getByLabel('录入时必填', { exact: true }).check();
      await fields.selectOption('category');
      await enter(page.getByRole('button', { name: '配置此字段', exact: true }));
      await page.getByLabel('显示名称', { exact: true }).fill('通知渠道');
      await page.getByLabel('字段说明', { exact: true }).fill('可选择静态渠道名称，也可以输入自定义值；保存时保留真实渠道编码。');
      await page.getByLabel('录入控件', { exact: true }).selectOption('select');
      await page.getByLabel('属于集合', { exact: true }).check();
      await enter(page.getByRole('button', { name: '添加选项', exact: true }));
      await page.getByLabel('选项名称 1', { exact: true }).fill('通知');
      await page.getByLabel('选项名称 1', { exact: true }).focus(); await page.keyboard.press('Tab');
      assert.equal(await active(page.getByLabel('实际值 1', { exact: true })), true);
      await page.keyboard.type('notice');
      await page.keyboard.press('Escape');
      const leave = page.getByRole('alertdialog', { name: '放弃未保存的修改？', exact: true });
      await leave.waitFor();
      assert.equal(await active(leave.getByRole('button', { name: '继续编辑', exact: true })), true);
      await page.keyboard.press('Enter'); await leave.waitFor({ state: 'hidden' });
      assert.equal(await page.getByLabel('实际值 1', { exact: true }).inputValue(), 'notice');
      await page.getByLabel('实际值 1', { exact: true }).focus();
      await page.screenshot({ path: path.join(output, `configuration-${viewport.width}.png`), animations: 'disabled' });
      const save = page.getByRole('button', { name: '保存全部字段配置', exact: true });
      await reachable(save);
      const saved = page.waitForResponse(r => new URL(r.url()).pathname === policyPath && r.request().method() === 'PUT');
      await page.keyboard.press('Enter'); assert.equal((await saved).status(), 200);
      await page.getByText('字段配置已保存', { exact: true }).waitFor();
      await page.keyboard.press('Escape');
      await fields.waitFor({ state: 'hidden' }); assert.equal(await active(entry), true);
      pass(label, 'administrator configures named required text and static/custom select; option Tab labels, unsaved Escape and entry focus restoration');

      await page.goto(`${base}/configuration/managed-data?table_name=${table}`);
      const filters = page.getByRole('region', { name: '查询条件', exact: true });
      await filters.getByLabel('通知渠道 运算符', { exact: true }).selectOption('in');
      await filters.getByLabel('筛选 通知渠道 集合值 1', { exact: true }).selectOption('0');
      await enter(filters.getByRole('button', { name: '通知渠道 添加集合值', exact: true }));
      await filters.getByLabel('筛选 通知渠道 集合值 2 自定义值', { exact: true }).fill('digest');
      await filters.getByLabel('筛选 priority 值', { exact: true }).fill('10');
      const filterScroll = filters.locator('fieldset').first().locator('..');
      if (viewport.width === 390) {
        assert.ok(await filterScroll.evaluate(node => node.scrollHeight > node.clientHeight));
        await filters.getByLabel('筛选 updated_at 值', { exact: true }).focus();
        await page.waitForFunction(node => node.scrollTop > 0, await filterScroll.elementHandle());
      }
      await enter(filters.getByRole('button', { name: '收起筛选', exact: true }));
      const query = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/tables/${table}/query` && r.request().method() === 'POST');
      await reachable(filters.getByRole('button', { name: '查询', exact: true })); await page.keyboard.press('Enter');
      const queried = await query; assert.equal(queried.status(), 200);
      assert.deepEqual(queried.request().postDataJSON().conditions, [{ field: 'category', operator: 'in', values: ['notice', 'digest'] }, { field: 'priority', operator: 'exact', value: '10' }]);
      assert.deepEqual((await queried.json()).rows.map(row => row.name), ['Alpha']);
      await enter(filters.getByRole('button', { name: '展开筛选', exact: true }));
      assert.equal(await filters.getByLabel('筛选 通知渠道 集合值 2 自定义值', { exact: true }).inputValue(), 'digest');
      await noOverflow();
      pass(label, 'keyboard submits static plus custom IN with a second AND field; collapse retains inputs and long mobile filters scroll locally');

      await enter(page.getByRole('button', { name: '修改记录 1', exact: true }));
      const name = page.getByLabel('name 值', { exact: true });
      await name.fill('');
      const review = page.getByRole('button', { name: '查看 Change Set', exact: true });
      await enter(review);
      await page.getByRole('alert').filter({ hasText: '请填写此字段' }).waitFor();
      assert.equal(await name.getAttribute('aria-invalid'), 'true');
      assert.ok(await name.evaluate(node => (node.getAttribute('aria-describedby') || '').split(' ').some(id => document.getElementById(id)?.textContent.includes('请填写此字段'))));
      assert.equal(await active(name), true, 'invalid field must receive focus when review fails');
      const draftName = `field-flow-${engine}-${viewport.width}`;
      await name.fill(draftName); assert.equal(await name.getAttribute('aria-invalid'), 'false');
      const category = page.getByLabel('category 值', { exact: true });
      await category.selectOption('custom');
      const custom = page.getByLabel('category 值 自定义值', { exact: true });
      await custom.fill(`custom-${viewport.width}`);
      await custom.focus(); await page.keyboard.press('Tab'); assert.notEqual(await page.evaluate(() => document.activeElement.tagName), 'BODY');
      await page.keyboard.press('Escape'); await leave.waitFor(); await page.keyboard.press('Escape');
      await leave.waitFor({ state: 'hidden' }); assert.equal(await custom.inputValue(), `custom-${viewport.width}`);
      const body = page.locator('.drawer-body');
      assert.ok(await body.evaluate(node => node.scrollHeight > node.clientHeight));
      await page.getByLabel('priority 值', { exact: true }).focus();
      await page.waitForFunction(node => node.scrollTop > 0, await body.elementHandle());
      await custom.focus();
      await page.screenshot({ path: path.join(output, `editor-${viewport.width}.png`), animations: 'disabled' });
      await reachable(review); await page.keyboard.press('Enter');
      const change = page.getByRole('dialog', { name: 'MODIFY Change Set', exact: true }); await change.waitFor();
      const changeMetrics = await keyboardScroll(change.getByRole('region', { name: '变更字段对比，可横向滚动', exact: true }), viewport.width === 390);
      const title = page.getByLabel('发布单标题', { exact: true }); await title.fill(`完整字段流程 ${engine} ${viewport.width}`);
      await page.screenshot({ path: path.join(output, `change-set-${viewport.width}.png`), animations: 'disabled' });
      const created = page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/release-orders' && r.request().method() === 'POST');
      await reachable(page.getByRole('button', { name: '确认并保存草稿', exact: true })); await page.keyboard.press('Enter');
      const response = await created; assert.equal(response.status(), 201); const draft = await response.json(); drafts.push(draft.id);
      assert.equal(draft.items[0].content.category, `custom-${viewport.width}`); assert.equal(draft.items[0].content.name, draftName);
      await page.waitForURL(`**/configuration/release-orders/${draft.id}`);
      await page.getByRole('heading', { name: draft.title, exact: true }).waitFor();
      const diff = page.getByRole('region', { name: '明细 1 字段对比，可横向滚动', exact: true });
      await diff.getByRole('rowheader', { name: /通知渠道\s+category/ }).waitFor();
      await diff.getByText(`custom-${viewport.width}`, { exact: true }).waitFor();
      const reviewMetrics = await keyboardScroll(diff, viewport.width === 390);
      await page.screenshot({ path: path.join(output, `review-${viewport.width}.png`), animations: 'disabled' });
      await reachable(page.getByRole('button', { name: '编辑草稿', exact: true }));
      pass(label, 'required error links and focuses its control; custom draft survives exit; Change Set and durable review retain real values with keyboard scroll/focus rings and reachable actions', { changeMetrics, reviewMetrics });
      // UI cancellation retires our draft without touching any business record.
      await enter(page.getByRole('button', { name: '取消草稿', exact: true }));
      await page.getByLabel('取消原因', { exact: true }).fill('完整字段流程验收结束');
      await enter(page.getByRole('button', { name: '确认取消草稿', exact: true }));
      await page.getByText(`${table} · 已取消`, { exact: true }).waitFor();
      drafts.pop();
    }
    assert.deepEqual(errors, []);
    completed = true;
  } catch (error) {
    if (page) {
      await page.screenshot({ path: path.join(output, 'failure.png'), fullPage: true }).catch(() => {});
      await fs.writeFile(path.join(output, 'failure.txt'), await page.locator('body').innerText().catch(() => 'unavailable'));
    }
    throw error;
  } finally {
    try {
      for (const id of drafts) {
        const draft = await api('GET', `/api/v1/release-orders/${id}`);
        if (draft.state === 'DRAFT') await api('POST', `/api/v1/release-orders/${id}/cancel`, { expected_version: draft.version, reason: '验收清理' });
      }
      if (originalPolicies) {
        await api('PUT', policyPath, { policies: originalPolicies });
        assert.deepEqual((await api('GET', policyPath)).fields.flatMap(field => field.policy ? [field.policy] : []), originalPolicies);
      }
      await fs.writeFile(path.join(output, 'result.json'), JSON.stringify({ engine, browser: browser.version(), checks, errors, policiesRestored: Boolean(originalPolicies), ok: completed && checks.length === 6 && errors.length === 0 }, null, 2) + '\n');
    } finally { await browser.close(); }
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
