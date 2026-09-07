// Standalone acceptance against a running, isolated Admin + Web + MySQL fixture.
// RCC_PLAYWRIGHT_MODULE may point to an existing Playwright package; no install required.
const { chromium } = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { browserOptions, registerFixtureAccount, authenticatedDelete } = require('./local-account.cjs');
const fs = require('node:fs/promises');
const base = process.env.RCC_WEB_URL || 'http://127.0.0.1:15173';
const output = process.env.RCC_E2E_OUTPUT || '/tmp/rcc-stage2-browser';
const table = process.env.RCC_E2E_TABLE || 'stage1_acceptance_items';

(async () => {
  await fs.mkdir(output, { recursive: true });
  const browser = await chromium.launch(browserOptions());
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  page.setDefaultTimeout(8000);
  let cleanupCode = null;
  const errors = [];
  const passed = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const check = (name) => { passed.push(name); console.log('PASS', name); };
  const button = (name) => page.getByRole('button', { name, exact: true });
  const discard = () => page.getByRole('alertdialog', { name: '放弃未保存的修改？' });
  const nameField = () => page.getByRole('textbox', { name: '显示名称', exact: true });
  try {
    const account = await registerFixtureAccount(context, base);
    await page.goto(`${base}/platform/query-policies`);
    await button('新建草稿').click();
    await nameField().waitFor();
    await page.keyboard.press('Escape');
    await page.waitForURL('**/platform/query-policies');
    assert.equal(await page.getByRole('alertdialog').count(), 0);
    check('untouched Query create closes without a warning');

    await button('新建草稿').click();
    await nameField().fill('Stage 2 draft');
    await page.goBack();
    await discard().waitFor();
    await button('继续编辑').click();
    assert.equal(await nameField().inputValue(), 'Stage 2 draft');
    assert.match(page.url(), /query-policies\/new$/);
    check('actual browser POP back cancel retains input and route');
    await page.goBack();
    await discard().waitFor();
    await button('放弃修改并离开').click();
    await page.waitForURL('**/platform/query-policies');
    await page.goForward();
    await nameField().waitFor();
    assert.equal(await nameField().inputValue(), '');
    check('actual browser POP back confirm and forward restore the intended route');

    await nameField().fill('reload-protected');
    const dialogEvent = page.waitForEvent('dialog');
    const reload = page.reload().catch(() => null);
    const native = await dialogEvent;
    assert.equal(native.type(), 'beforeunload');
    await native.dismiss();
    await reload;
    assert.equal(await nameField().inputValue(), 'reload-protected');
    check('native beforeunload reload cancellation preserves input');
    const closeEvent = page.waitForEvent('dialog');
    await page.close({ runBeforeUnload: true });
    const nativeClose = await closeEvent;
    assert.equal(nativeClose.type(), 'beforeunload');
    await nativeClose.dismiss();
    assert.equal(page.isClosed(), false);
    assert.equal(await nameField().inputValue(), 'reload-protected');
    check('native tab-close cancellation preserves the editing document');

    await button('关闭').click();
    await discard().waitFor();
    assert.equal(await button('继续编辑').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Shift+Tab');
    assert.equal(await button('放弃修改并离开').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    assert.equal(await button('继续编辑').evaluate((node) => node === document.activeElement), true);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: `${output}/discard-390.png`, fullPage: true, mask: [page.locator('.operator')] });
    const bounds = await discard().boundingBox();
    assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 390 && bounds.y >= 0 && bounds.y + bounds.height <= 844);
    check('discard confirmation owns keyboard focus and fits a 390px viewport');
    await page.keyboard.press('Escape');
    assert.equal(await nameField().inputValue(), 'reload-protected');
    await nameField().fill('');
    await button('取消').click();
    await page.waitForURL('**/platform/query-policies');
    check('restored original Query values clear protection');
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goBack();
    await nameField().fill('forward protected');
    await page.goForward();
    await discard().waitFor();
    await button('继续编辑').click();
    assert.equal(await nameField().inputValue(), 'forward protected');
    await page.goForward();
    await discard().waitFor();
    await button('放弃修改并离开').click();
    await page.waitForURL('**/platform/query-policies');
    check('actual browser forward navigation is canceled or confirmed at its original target');

    await button('新建草稿').click();
    const codeField = page.getByRole('textbox', { name: /规则编码/ });
    await codeField.fill('notification_page_query_v1');
    await nameField().fill('Stage 2 save outcome');
    await button('创建草稿').click();
    await page.getByRole('alert').waitFor();
    assert.equal(await nameField().inputValue(), 'Stage 2 save outcome');
    assert.equal(await codeField.inputValue(), 'notification_page_query_v1');
    check('real duplicate Policy Code rejection preserves Query form input');
    const createdCode = `stage2_unsaved_query_${Date.now()}_v1`;
    cleanupCode = createdCode;
    await codeField.fill(createdCode);
    let releaseResponse;
    const responseGate = new Promise((resolve) => { releaseResponse = resolve; });
    let responseArrived;
    const received = new Promise((resolve) => { responseArrived = resolve; });
    let writeCount = 0;
    await page.route('**/api/v1/query-policies', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      writeCount += 1;
      const response = await route.fetch(); // Real write completes; hold only delivery to the UI.
      responseArrived();
      await responseGate;
      await route.fulfill({ response });
    });
    await button('创建草稿').click();
    await received;
    // The real server receipt is a network checkpoint, not a React DOM commit.
    // Keep delivery gated while waiting for the form's inherited disabled state.
    await nameField().and(page.locator(':disabled')).waitFor({ state: 'attached', timeout: 8000 });
    assert.equal(await nameField().isDisabled(), true);
    await page.goBack();
    await page.getByRole('alertdialog', { name: '正在提交，请稍候' }).waitFor();
    assert.equal(await button('放弃修改并离开').count(), 0);
    assert.equal(await nameField().isDisabled(), true);
    releaseResponse();
    await page.waitForURL(`**/query-policies/${createdCode}`);
    assert.equal(await page.getByRole('alertdialog').count(), 0);
    assert.equal(writeCount, 1);
    assert.equal(await nameField().inputValue(), 'Stage 2 save outcome');
    check('real completed write with delayed response blocks POP, then shows success once');
    await page.unroute('**/api/v1/query-policies');
    const cleanup = await authenticatedDelete(context, base, `/api/v1/query-policies/${createdCode}`);
    assert.equal(cleanup.status(), 204);
    cleanupCode = null;
    check('disposable Query draft cleaned up through the Admin API');

    await page.goto(`${base}/configuration/managed-data`);
    await page.getByRole('combobox', { name: 'Managed Table', exact: true }).selectOption(table);
    await button('新增记录').click();
    await page.getByRole('checkbox', { name: '包含 name', exact: true }).check();
    await page.getByRole('textbox', { name: 'name 值', exact: true }).fill('stage2-invalid-row');
    await page.getByRole('checkbox', { name: '包含 state', exact: true }).check();
    await page.getByRole('textbox', { name: 'state 值', exact: true }).fill('not-valid-state');
    await page.getByRole('checkbox', { name: '包含 category', exact: true }).check();
    await page.getByRole('checkbox', { name: 'category 使用 NULL', exact: true }).check();
    await page.getByRole('checkbox', { name: '包含 note', exact: true }).check();
    await button('查看 Change Set').click();
    await button('放弃本次编辑').click();
    await discard().waitFor();
    await button('继续编辑').click();
    await button('返回修改').click();
    assert.equal(await page.getByRole('textbox', { name: 'name 值', exact: true }).inputValue(), 'stage2-invalid-row');
    assert.equal(await page.getByRole('checkbox', { name: 'category 使用 NULL', exact: true }).isChecked(), true);
    assert.equal(await page.getByRole('checkbox', { name: '包含 priority', exact: true }).isChecked(), false);
    check('Change Set cancel and return retain values, NULL and omitted fields');
    // Saving a draft does not execute the ENUM constraint. Omit the required
    // name instead so real draft preparation rejects this request.
    await page.getByRole('checkbox', { name: '包含 name', exact: true }).uncheck();
    await button('查看 Change Set').click();
    await button('确认并保存草稿').click();
    await page.getByRole('alert').filter({ hasText: '新增内容缺少实时 Schema 要求的字段' }).waitFor();
    await button('返回修改').click();
    assert.equal(await page.getByRole('textbox', { name: 'state 值', exact: true }).inputValue(), 'not-valid-state');
    await page.getByRole('alert').filter({ hasText: '新增内容缺少实时 Schema 要求的字段' }).waitFor();
    await page.screenshot({ path: `${output}/failed-row-retained.png`, fullPage: true, mask: [page.locator('.operator')] });
    check('real Admin/MySQL validation rejection retains the row draft and request error');
    await button('取消').click();
    await button('放弃修改并离开').click();
    assert.equal(await page.getByRole('dialog').count(), 0);
    await account.assertMemoryOnly(page, ['Stage 2 draft', 'reload-protected', 'forward protected', 'Stage 2 save outcome', createdCode, 'stage2-invalid-row', 'not-valid-state']);
    check('confirmed discard closes the editor with no browser storage draft');
    assert.deepEqual(errors, []);
    await fs.writeFile(`${output}/result.json`, JSON.stringify({ browser: browser.version(), passed, errors }, null, 2));
  } catch (error) {
    await page.screenshot({ path: `${output}/failure.png`, fullPage: true, mask: [page.locator('.operator')] }).catch(() => {});
    throw error;
  } finally {
    if (cleanupCode) await authenticatedDelete(context, base, `/api/v1/query-policies/${cleanupCode}`).catch(() => {});
    await browser.close();
  }
})().catch((error) => { console.error(error); process.exitCode = 1; });
