// Real browser -> production Web proxy -> Bearer Admin -> disposable MySQL 8.4.
// RCC_E2E_ENGINE chooses one Playwright engine; the runner records each separately.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const { execFileSync } = require('node:child_process');

const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const table = process.env.RCC_E2E_TABLE || 'stage1_acceptance_items';
const container = process.env.RCC_E2E_MYSQL_CONTAINER;
const engineName = process.env.RCC_E2E_ENGINE || 'chromium';
const engine = playwright[engineName];
assert.ok(['chromium', 'firefox', 'webkit'].includes(engineName), `unknown browser engine: ${engineName}`);
assert.ok(engine, `Playwright engine is unavailable: ${engineName}`);
assert.match(container || '', /^rcc-browser-\d+-\d+-mysql$/);

const sql = (statement) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c',
  'MYSQL_PWD="$MYSQL_PASSWORD" mysql --default-character-set=utf8mb4 --raw --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE"'],
{ input: statement, encoding: 'utf8', timeout: 20000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
const literal = (value) => `'${String(value).replaceAll("'", "''")}'`;

(async () => {
  await fs.mkdir(output, { recursive: true });
  const checks = [];
  const pageErrors = [];
  const http = [];
  const requests = [];
  const cleanupNames = new Set();
  let browser;
  let page;
  let failure = null;
  let browserVersion = null;
  let faultEvidence = null;
  const pass = (name, evidence = {}) => {
    checks.push({ name, evidence });
    console.log('PASS', name, JSON.stringify(evidence));
  };
  const button = (name) => page.getByRole('button', { name, exact: true });
  const dialogContainsFocus = (locator) => locator.evaluate((node) => node.contains(document.activeElement));
  const pageOverflow = () => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  const visibleInViewport = (locator) => locator.evaluate((node) => {
    const rect = node.getBoundingClientRect();
    return rect.left >= -1 && rect.right <= innerWidth + 1 && rect.top >= -1 && rect.bottom <= innerHeight + 1;
  });
  const viewportRect = (locator) => locator.evaluate((node) => {
    const { left, right, top, bottom, width, height } = node.getBoundingClientRect();
    return { left, right, top, bottom, width, height, innerWidth, innerHeight };
  });
  const boundedSurface = (locator) => locator.evaluate((node) => {
    const { left, right, width } = node.getBoundingClientRect();
    return { left, right, width, clientWidth: node.clientWidth, scrollWidth: node.scrollWidth, innerWidth };
  });
  async function open(pathname, viewport = { width: 1440, height: 1000 }) {
    if (page) await page.close();
    page = await browser.newPage({ viewport });
    page.setDefaultTimeout(15000);
    page.setDefaultNavigationTimeout(20000);
    page.on('pageerror', (error) => pageErrors.push({ url: page.url(), message: error.message }));
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith('/api/')) requests.push({ method: request.method(), path: url.pathname });
    });
    page.on('response', (response) => {
      const request = response.request();
      const url = new URL(response.url());
      if (!url.pathname.startsWith('/api/')) return;
      let body;
      if (!['GET', 'HEAD'].includes(request.method())) {
        try { body = request.postDataJSON(); } catch { body = request.postData() || undefined; }
      }
      http.push({ method: request.method(), path: url.pathname, status: response.status(), ...(body === undefined ? {} : { body }) });
    });
    await page.goto(`${base}${pathname}`);
  }
  async function managed(viewport) {
    await open('/configuration/managed-data', viewport);
    await page.getByRole('combobox', { name: 'Managed Table', exact: true }).selectOption(table);
    await button('新增记录').waitFor();
  }
  async function include(field, value) {
    await page.getByRole('checkbox', { name: `包含 ${field}`, exact: true }).check();
    if (value !== undefined) await page.getByRole('textbox', { name: `${field} 值`, exact: true }).fill(value);
  }
  async function pasteRaw(control, raw) {
    await control.evaluate((node, text) => {
      node.focus();
      const clipboardData = new DataTransfer();
      clipboardData.setData('text/plain', text);
      const paste = new Event('paste', { bubbles: true, cancelable: true });
      Object.defineProperty(paste, 'clipboardData', { value: clipboardData });
      node.dispatchEvent(paste);
    }, raw);
  }

  try {
    browser = await engine.launch({ headless: true });
    browserVersion = browser.version();

    // Rule editing, native browser history and modal ownership.
    await open('/platform/query-policies');
    await button('新建草稿').click();
    const ruleDrawer = page.getByRole('dialog', { name: '新建查询规则草稿', exact: true });
    await ruleDrawer.waitFor();
    assert.equal(await ruleDrawer.evaluate((node) => node === document.activeElement), true, 'opening the drawer must place focus without a test fixture');
    await ruleDrawer.evaluate((node) => {
      const hidden = document.createElement('button');
      hidden.textContent = 'Stage 5 hidden focus fixture';
      hidden.style.display = 'none';
      const disabled = document.createElement('button');
      disabled.textContent = 'Stage 5 disabled focus fixture';
      disabled.disabled = true;
      node.append(hidden, disabled);
    });
    await page.keyboard.press('Shift+Tab');
    assert.equal(await button('取消').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    assert.equal(await ruleDrawer.getByRole('button', { name: '关闭', exact: true }).evaluate((node) => node === document.activeElement), true);
    await page.getByRole('textbox', { name: '显示名称', exact: true }).fill(`Stage 5 ${engineName} draft`);
    const inertProbe = await page.getByRole('link', { name: '配置内容管理', exact: true }).evaluate((background) => {
      const inertAncestor = background.closest('[inert]');
      background.focus(); // Programmatic probe for native inert; not a Tab path.
      return {
        inert: Boolean(inertAncestor),
        focusStayedInTopModal: Boolean(document.activeElement?.closest('[data-modal-surface="true"]')),
        bodyClasses: [...document.body.classList],
        rootLocked: document.documentElement.classList.contains('modal-open'),
      };
    });
    assert.deepEqual(inertProbe, { inert: true, focusStayedInTopModal: true, bodyClasses: ['modal-open'], rootLocked: true });
    await page.evaluate(() => (document.activeElement instanceof HTMLElement ? document.activeElement.blur() : undefined));
    await page.keyboard.press('Tab');
    assert.equal(await ruleDrawer.getByRole('button', { name: '关闭', exact: true }).evaluate((node) => node === document.activeElement), true);
    pass('drawer background is inert and actual Shift+Tab/Tab loops inside the drawer', {
      ...inertProbe,
      focusProbe: 'programmatic HTMLElement.focus used only for native inert',
      keyboardPath: ['Shift+Tab skips appended hidden/disabled fixtures -> 取消', 'Tab -> 关闭', 'focus-loss fixture then real Tab -> 关闭'],
    });

    const unloadProbe = await page.evaluate(() => {
      const event = new Event('beforeunload', { cancelable: true });
      const dispatched = window.dispatchEvent(event);
      return { defaultPrevented: event.defaultPrevented, dispatchReturned: dispatched, fixture: 'cancelable browser event; no user-agent prompt' };
    });
    assert.equal(unloadProbe.defaultPrevented, true);
    assert.equal(unloadProbe.dispatchReturned, false);
    await page.goBack();
    const leave = page.getByRole('alertdialog', { name: '放弃未保存的修改？', exact: true });
    await leave.waitFor();
    assert.equal(await ruleDrawer.evaluate((node) => Boolean(node.closest('[inert]'))), true);
    assert.equal(await button('继续编辑').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Shift+Tab');
    assert.equal(await button('放弃修改并离开').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    assert.equal(await button('继续编辑').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Escape');
    await leave.waitFor({ state: 'detached' });
    assert.equal(await dialogContainsFocus(ruleDrawer), true);
    await page.goBack();
    await leave.waitFor();
    await button('放弃修改并离开').click();
    await page.waitForURL('**/platform/query-policies');
    await page.goForward();
    await page.getByRole('textbox', { name: '显示名称', exact: true }).waitFor();
    assert.equal(await page.getByRole('textbox', { name: '显示名称', exact: true }).inputValue(), '');
    await button('关闭').click();
    await page.waitForURL('**/platform/query-policies');
    pass('native back cancel/confirm and forward preserve the intended history entry', { unloadProbe, lowerDrawerInertWhilePromptOpen: true, focusRestoredToDrawer: true });

    // Narrow drawer, raw CR boundary and a real MySQL validation failure.
    await managed({ width: 320, height: 568 });
    await button('新增记录').click();
    await include('name', `stage5_${engineName}_invalid`);
    await include('state', 'invalid-state');
    await include('note');
    const optionalId = page.getByRole('checkbox', { name: '包含 id', exact: true });
    assert.equal(await optionalId.isChecked(), false);
    const rawCR = `prefix-${engineName}\r\nsecond\rtail`;
    const note = page.getByRole('textbox', { name: 'note 值', exact: true });
    await pasteRaw(note, rawCR);
    assert.equal(await note.getAttribute('readonly'), '');
    const drawerBody = page.locator('.drawer-body');
    const drawerFooter = page.locator('.drawer-footer');
    assert.ok(await drawerBody.evaluate((node) => node.scrollHeight > node.clientHeight));
    const drawerScrollBefore = await drawerBody.evaluate((node) => node.scrollTop);
    await drawerBody.hover();
    await page.mouse.wheel(0, 360);
    await page.waitForFunction(({ selector, before }) => document.querySelector(selector).scrollTop > before, { selector: '.drawer-body', before: drawerScrollBefore });
    const drawerScrollAfter = await drawerBody.evaluate((node) => node.scrollTop);
    const drawerSurfaces = [];
    for (const selector of ['.drawer', '.drawer-header', '.drawer-body', '.drawer-footer']) {
      const metrics = await boundedSurface(page.locator(selector));
      drawerSurfaces.push({ selector, metrics });
      assert.ok(metrics.left >= -1 && metrics.right <= metrics.innerWidth + 1, `${selector} must stay inside the 320px viewport: ${JSON.stringify(metrics)}`);
      assert.ok(metrics.scrollWidth <= metrics.clientWidth + 1, `${selector} must not own unintended horizontal overflow: ${JSON.stringify(metrics)}`);
    }
    const noteRect = await viewportRect(note);
    assert.ok(noteRect.left >= -1 && noteRect.right <= noteRect.innerWidth + 1, `note textarea must stay inside the 320px viewport: ${JSON.stringify(noteRect)}`);
    const drawerFooterActions = [];
    for (const action of ['取消', '查看 Change Set']) {
      const actionButton = drawerFooter.getByRole('button', { name: action, exact: true });
      await actionButton.scrollIntoViewIfNeeded();
      const rect = await viewportRect(actionButton);
      drawerFooterActions.push({ action, rect });
      assert.equal(await visibleInViewport(actionButton), true, `${action} must be fully visible at 320px: ${JSON.stringify(rect)}`);
      assert.ok((await pageOverflow()) <= 1, `${action} must not require document horizontal scrolling`);
    }
    await page.screenshot({ path: `${output}/drawer-320.png`, fullPage: true });
    pass('320px long drawer scroll keeps the action footer reachable', { viewport: '320x568 CSS pixels', documentOverflow: await pageOverflow(), optionalIdIncluded: false, actualMouseWheel: { before: drawerScrollBefore, after: drawerScrollAfter }, surfaces: drawerSurfaces, noteRect, footerActions: drawerFooterActions });

    await button('查看 Change Set').click();
    const changeSet = page.getByRole('dialog', { name: 'ADD Change Set', exact: true });
    await changeSet.waitFor();
    assert.equal(await changeSet.evaluate((node) => node === document.activeElement), true, 'opening Change Set must place focus without a test fixture');
    await page.keyboard.press('Shift+Tab');
    assert.equal(await button('确认并执行').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    assert.equal(await button('放弃本次编辑').evaluate((node) => node === document.activeElement), true);
    const changeScroll = page.locator('.change-set-scroll');
    assert.ok(await changeScroll.evaluate((node) => node.scrollWidth > node.clientWidth));
    await changeScroll.evaluate((node) => { node.scrollLeft = node.scrollWidth; node.scrollTop = node.scrollHeight; });
    const changeSetActions = [];
    for (const action of ['放弃本次编辑', '返回修改', '确认并执行']) {
      const actionButton = button(action);
      await actionButton.scrollIntoViewIfNeeded();
      const rect = await viewportRect(actionButton);
      changeSetActions.push({ action, rect });
      assert.equal(await visibleInViewport(actionButton), true, `${action} must remain reachable at 320px: ${JSON.stringify(rect)}`);
      assert.ok((await pageOverflow()) <= 1, `${action} must not require document horizontal scrolling`);
    }
    assert.ok((await pageOverflow()) <= 1);
    await page.screenshot({ path: `${output}/change-set-320.png` });
    pass('320px Change Set owns horizontal/vertical overflow and keeps actions reachable', { keyboardPath: ['Shift+Tab -> 确认并执行', 'Tab -> 放弃本次编辑'], documentOverflow: await pageOverflow(), footerActions: changeSetActions });

    await page.keyboard.press('Escape');
    const nestedLeave = page.getByRole('alertdialog', { name: '放弃未保存的修改？', exact: true });
    await nestedLeave.waitFor();
    assert.equal(await page.locator('.change-set-dialog').evaluate((node) => Boolean(node.closest('[inert]'))), true);
    await page.keyboard.press('Shift+Tab');
    assert.equal(await button('放弃修改并离开').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    assert.equal(await button('继续编辑').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Escape');
    await nestedLeave.waitFor({ state: 'detached' });
    assert.equal(await dialogContainsFocus(changeSet), true);
    assert.equal(await page.evaluate(() => document.body.classList.contains('modal-open')), true);
    pass('only the top nested leave dialog handles Tab/Escape and focus returns to Change Set', { lowerChangeSetInert: true, bodyRemainedLocked: true });

    const failureResponse = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === `/api/v1/tables/${table}/rows`);
    await button('确认并执行').click();
    const failed = await failureResponse;
    assert.equal(failed.status(), 400);
    const failureBody = failed.request().postDataJSON();
    assert.equal(failureBody.content.note, rawCR);
    assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(`stage5_${engineName}_invalid`)};`), '0');
    await page.getByRole('alert').filter({ hasText: '写入内容不符合实时字段 Schema' }).waitFor();
    await button('返回修改').click();
    assert.equal(await note.getAttribute('readonly'), '');
    await button('note 值：转换为 LF 再编辑').click();
    assert.equal(await note.getAttribute('readonly'), null);
    assert.equal(await note.inputValue(), rawCR.replace(/\r\n?/g, '\n'));
    await button('取消').click();
    await nestedLeave.waitFor();
    await button('放弃修改并离开').click();
    assert.equal(await page.locator('[data-modal-surface="true"]').count(), 0);
    assert.equal(await page.evaluate(() => document.body.classList.contains('modal-open')), false);
    pass('real MySQL rejection retains raw CR and explicit LF conversion unlocks editing', {
      status: failed.status(),
      rawHex: Buffer.from(rawCR).toString('hex'),
      submittedHex: Buffer.from(failureBody.content.note).toString('hex'),
      clipboardBoundary: 'synthetic paste Event with injected DataTransfer; no OS clipboard access',
    });

    // API fault injection happens after a real database commit.
    await managed({ width: 390, height: 640 });
    await button('新增记录').click();
    const unknownName = `stage5_${engineName}_unknown`;
    cleanupNames.add(unknownName);
    await include('name', unknownName);
    assert.equal(await page.getByRole('checkbox', { name: '包含 id', exact: true }).isChecked(), false);
    const writePath = `/api/v1/tables/${table}/rows`;
    const unknownRequestStart = requests.length;
    await page.route(`**${writePath}`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      const response = await route.fetch();
      faultEvidence = { actualStatus: response.status(), requestId: response.headers()['x-request-id'], injectedStatus: 503 };
      await route.fulfill({ status: 503, json: { error: { code: 'mutation_unavailable', message: 'Injected after real commit', request_id: faultEvidence.requestId } } });
    });
    await button('查看 Change Set').click();
    await button('确认并执行').click();
    const uncertain = page.getByRole('alert', { name: '提交结果尚未确认', exact: true });
    await uncertain.waitFor();
    assert.deepEqual(faultEvidence && { actualStatus: faultEvidence.actualStatus, injectedStatus: faultEvidence.injectedStatus }, { actualStatus: 201, injectedStatus: 503 });
    assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(unknownName)};`), '1');
    assert.equal(await button('确认并执行').isDisabled(), true);
    await button('只读核对当前状态').scrollIntoViewIfNeeded();
    assert.equal(await visibleInViewport(button('只读核对当前状态')), true);
    assert.ok((await pageOverflow()) <= 1);
    await button('只读核对当前状态').click();
    await page.getByText('当前查询结果（仅供核对）', { exact: true }).waitFor();
    const snapshot = page.locator('.write-recovery-snapshot');
    assert.ok(await snapshot.evaluate((node) => node.scrollWidth > node.clientWidth || node.scrollHeight > node.clientHeight));
    await snapshot.evaluate((node) => { node.scrollLeft = node.scrollWidth; node.scrollTop = node.scrollHeight; });
    await button('我已核对，返回修改').scrollIntoViewIfNeeded();
    assert.equal(await visibleInViewport(button('我已核对，返回修改')), true);
    for (const action of ['关闭本次预览', '返回修改', '确认并执行']) {
      assert.equal(await visibleInViewport(button(action)), true, `${action} must remain reachable during recovery`);
    }
    const rowWrites = requests.slice(unknownRequestStart).filter((entry) => entry.method === 'POST' && entry.path === writePath);
    assert.equal(rowWrites.length, 1);
    assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(unknownName)};`), '1');
    await page.screenshot({ path: `${output}/write-recovery-390.png`, fullPage: true });
    pass('390px unknown-write recovery stays locked and performs one real write plus read-only verification', {
      faultEvidence,
      rowWriteRequests: rowWrites.length,
      sqlRows: 1,
      longSnapshotOwnsOverflow: true,
      faultBoundary: 'Playwright API response fault injection after route.fetch committed to real MySQL 8.4',
      documentOverflow: await pageOverflow(),
    });

    assert.deepEqual(pageErrors, []);
  } catch (error) {
    failure = { name: error.name, message: error.message, stack: error.stack };
    if (page) {
      await page.screenshot({ path: `${output}/failure.png`, fullPage: true }).catch(() => {});
      await fs.writeFile(`${output}/failure-body.txt`, await page.locator('body').innerText().catch(() => 'page unavailable')).catch(() => {});
    }
  } finally {
    if (browser) await browser.close().catch(() => {});
    let cleanup = null;
    try {
      if (cleanupNames.size) sql(`DELETE FROM ${table} WHERE name IN (${[...cleanupNames].map(literal).join(',')});`);
      cleanup = { remainingRows: cleanupNames.size ? Number(sql(`SELECT COUNT(*) FROM ${table} WHERE name IN (${[...cleanupNames].map(literal).join(',')});`)) : 0 };
      assert.deepEqual(cleanup, { remainingRows: 0 });
    } catch (error) {
      cleanup = { error: error.message };
      if (!failure) failure = { name: error.name, message: error.message, stack: error.stack };
    }
    await fs.writeFile(`${output}/http-evidence.json`, JSON.stringify(http, null, 2) + '\n');
    await fs.writeFile(`${output}/result.json`, JSON.stringify({
      ok: failure === null,
      engine: engineName,
      browserVersion,
      checks,
      pageErrors,
      http,
      requests,
      faultEvidence,
      cleanup,
      coverageBoundaries: {
        safari: 'Playwright WebKit engine; not an installed Safari release',
        clipboard: 'synthetic paste Event with injected DataTransfer; not the OS clipboard',
        unknownWrite: 'Playwright API response fault injection after a real MySQL commit',
        beforeUnload: engineName === 'chromium'
          ? 'This suite checks a cancelable browser event; the existing Chromium unsaved-changes suite separately observes the native user-agent prompt.'
          : 'Cancelable browser event only; Playwright did not claim native user-agent prompt coverage for this engine.',
      },
      failure,
    }, null, 2) + '\n');
  }
  if (failure) {
    console.error(JSON.stringify(failure, null, 2));
    process.exitCode = 1;
  }
})();
