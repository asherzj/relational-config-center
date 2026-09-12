const { createFixtureApprovalRole, fixtureApprovalInput } = require('./table-approval-fixture.cjs');
const {readAllReleaseDetailPages,executionCommands,applicationItems}=require('./release-detail-pages.cjs');
// Real browser -> production Web proxy -> Cookie-authenticated Admin -> disposable MySQL 8.4.
// RCC_E2E_ENGINE chooses one Playwright engine; the runner records each separately.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { browserOptions, registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');
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
  let context;
  let approvalContext;
  let account;
  let approverAccount;
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
    page = await context.newPage();
    await page.setViewportSize(viewport);
    page.setDefaultTimeout(15000);
    page.setDefaultNavigationTimeout(20000);
    page.on('pageerror', (error) => pageErrors.push({ url: page.url(), message: error.message }));
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith('/api/') && !url.pathname.startsWith('/api/v1/auth/')) requests.push({
        method: request.method(),
        path: url.pathname,
        body: request.postData(),
        key: request.headers()['idempotency-key'],
      });
    });
    page.on('response', (response) => {
      const request = response.request();
      const url = new URL(response.url());
      if (!url.pathname.startsWith('/api/') || url.pathname.startsWith('/api/v1/auth/')) return;
      let body;
      if (!['GET', 'HEAD'].includes(request.method())) {
        try { body = request.postDataJSON(); } catch { body = request.postData() || undefined; }
      }
      http.push({ method: request.method(), path: url.pathname, status: response.status(), ...(body === undefined ? {} : { body }) });
    });
    await page.goto(`${base}${pathname}`);
  }
  async function managed(viewport) {
    // Start on this suite's fixture instead of briefly loading the first table.
    // The supported deep link gives the initial editor and reads one table identity.
    await open(`/configuration/managed-data?table_name=${encodeURIComponent(table)}`, viewport);
    const selectedTable = page.getByRole('combobox', { name: 'Managed Table', exact: true });
    await selectedTable.waitFor();
    assert.equal(await selectedTable.inputValue(), table);
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
  async function releaseWrite(actor, pathname, data, expected = 200) {
    const response = await authenticatedRequest(actor, base, pathname, {
      method: 'POST', headers: { 'Idempotency-Key': randomUUID() }, data,
    });
    assert.equal(response.status(), expected, `POST ${pathname}: ${await response.text()}`);
    return response.json();
  }
  async function publishRelease(draft) {
    let order = await releaseWrite(context, `/api/v1/release-orders/${draft.id}/submit`, { expected_version: draft.version });
    order = await releaseWrite(approvalContext, `/api/v1/release-orders/${draft.id}/approve`, await fixtureApprovalInput(approvalContext, base, draft.id, { expected_version: order.version, reason: 'Accessibility publication review' }));
    assert.equal(order.history.find((event) => event.action === 'APPROVE')?.actor_id, approverAccount.accountID);
    assert.notEqual(order.applicant_id, approverAccount.accountID, 'approval must use a separate permanent account');
    const result = await releaseWrite(context, `/api/v1/release-orders/${draft.id}/execute`, { expected_version: order.version });
    await releaseWrite(context, `/api/v1/release-orders/${draft.id}/complete`, { expected_version: result.version });
    return result;
  }

  try {
    browser = await engine.launch(browserOptions());
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    account = await registerFixtureAccount(context, base);
    approvalContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    approverAccount = await registerFixtureAccount(approvalContext, base, { roles: ['VIEWER'] });
    await createFixtureApprovalRole(context, base, `Accessibility review ${randomUUID()}`, [approverAccount.accountID], [table]);
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
    const inertProbe = await page.getByRole('link', { name: '统一变更入口', exact: true }).evaluate((background) => {
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
    await page.getByRole('dialog', { name: `新增 ${table} 记录`, exact: true }).waitFor();
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
    assert.equal(await button('确认并保存草稿').evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    const changeTableScroll = changeSet.getByRole('region', { name: '变更字段对比，可横向滚动', exact: true });
    assert.equal(await changeTableScroll.evaluate((node) => node === document.activeElement), true, 'Tab wraps to the keyboard-scrollable field comparison');
    await page.keyboard.press('Tab');
    const releaseTitle = changeSet.getByRole('textbox', { name: '发布单标题', exact: true });
    assert.equal(await releaseTitle.evaluate((node) => node === document.activeElement), true);
    await page.keyboard.press('Tab');
    const tabAfterReleaseTitle = await changeSet.evaluate((dialog) => {
      const active = document.activeElement;
      return {
        insideTopModal: active instanceof Element && dialog.contains(active),
        tagName: active?.tagName,
        type: active instanceof HTMLInputElement || active instanceof HTMLButtonElement ? active.type : undefined,
        id: active?.id,
        ariaLabel: active?.getAttribute('aria-label'),
        text: active?.textContent?.trim(),
        tabIndex: active instanceof HTMLElement ? active.tabIndex : undefined,
      };
    });
    assert.equal(tabAfterReleaseTitle.insideTopModal, true, `Tab from release title escaped the top Change Set: ${JSON.stringify(tabAfterReleaseTitle)}`);
    assert.equal(tabAfterReleaseTitle.text, '选择已有草稿', `unexpected Tab target after release title: ${JSON.stringify(tabAfterReleaseTitle)}`);
    const changeScroll = changeTableScroll;
    assert.ok(await changeTableScroll.evaluate((node) => node.scrollWidth > node.clientWidth));
    assert.ok(await changeScroll.evaluate((node) => node.scrollHeight > node.clientHeight));
    await changeTableScroll.evaluate((node) => { node.scrollLeft = node.scrollWidth; });
    await changeScroll.evaluate((node) => { node.scrollTop = node.scrollHeight; });
    const changeScrollEvidence = {
      horizontal: await changeTableScroll.evaluate((node) => ({ client: node.clientWidth, total: node.scrollWidth, offset: node.scrollLeft })),
      vertical: await changeScroll.evaluate((node) => ({ client: node.clientHeight, total: node.scrollHeight, offset: node.scrollTop })),
    };
    assert.ok(changeScrollEvidence.horizontal.offset > 0);
    assert.ok(changeScrollEvidence.vertical.offset > 0);
    const changeSetActions = [];
    for (const action of ['放弃本次编辑', '返回修改', '确认并保存草稿']) {
      const actionButton = button(action);
      await actionButton.scrollIntoViewIfNeeded();
      const rect = await viewportRect(actionButton);
      changeSetActions.push({ action, rect });
      assert.equal(await visibleInViewport(actionButton), true, `${action} must remain reachable at 320px: ${JSON.stringify(rect)}`);
      assert.ok((await pageOverflow()) <= 1, `${action} must not require document horizontal scrolling`);
    }
    assert.ok((await pageOverflow()) <= 1);
    await page.screenshot({ path: `${output}/change-set-320.png` });
    pass('320px Change Set owns horizontal/vertical overflow and keeps actions reachable', { keyboardPath: ['Shift+Tab -> 确认并保存草稿', 'Tab -> 变更字段对比，可横向滚动', 'Tab -> 发布单标题', 'Tab -> 选择已有草稿'], documentOverflow: await pageOverflow(), scrolling: changeScrollEvidence, footerActions: changeSetActions });

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

    const draftResponse = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/release-orders');
    await button('确认并保存草稿').click();
    const created = await draftResponse;
    assert.equal(created.status(), 201);
    const failureBody = created.request().postDataJSON();
    assert.equal(failureBody.items[0].table_name, table);
    assert.equal(failureBody.items[0].content.note, rawCR);
    await page.waitForURL('**/configuration/release-orders/*');
    let invalidOrder = await (await authenticatedRequest(context, base, new URL(page.url()).pathname.replace('/configuration', '/api/v1'))).json();
    invalidOrder = await releaseWrite(context, `/api/v1/release-orders/${invalidOrder.id}/submit`, { expected_version: invalidOrder.version });
    assert.deepEqual(invalidOrder.table_names, [table]);
    assert.equal(invalidOrder.applicant_id, account.accountID);
    invalidOrder = await releaseWrite(approvalContext, `/api/v1/release-orders/${invalidOrder.id}/approve`, await fixtureApprovalInput(approvalContext, base, invalidOrder.id, { expected_version: invalidOrder.version, reason: 'Independent accessibility validation review' }));
    assert.equal(invalidOrder.history.find((event) => event.action === 'APPROVE')?.actor_id, approverAccount.accountID);
    const rejected = await releaseWrite(context, `/api/v1/release-orders/${invalidOrder.id}/execute`, { expected_version: invalidOrder.version }, 422);
    assert.equal(rejected.error.code, 'invalid_mutation_content');
    const retainedResponse = await authenticatedRequest(context, base, `/api/v1/release-orders/${invalidOrder.id}`);
    assert.equal(retainedResponse.status(), 200);
    const retained = await readAllReleaseDetailPages(context, base, await retainedResponse.json());
    assert.equal(retained.state, 'APPROVED');
    assert.equal(retained.items[0].content.note, rawCR);
    assert.deepEqual(retained.executions, []);
    assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(`stage5_${engineName}_invalid`)};`), '0');
    await page.reload();
    await page.getByRole('heading', { name: `${table} 配置变更`, exact: true }).waitFor();
    await button('更多操作').click();
    await page.getByRole('menuitem', { name: '取消发布单', exact: true }).click();
    await page.getByRole('textbox', { name: '取消原因', exact: true }).fill('Correct the rejected value without changing the frozen intent');
    await button('确认取消发布单').click();
    await page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^已取消$/ }).waitFor();
    await button('复制新草稿').click();
    await button('读取最新配置').click();
    await button('确认最新基线并复制').click();
    await page.getByRole('heading', { name: `${table} 配置变更`, exact: true }).waitFor();
    await page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^草稿$/ }).waitFor();
    await button('编辑草稿').click();
    const copiedNote = page.getByRole('textbox', { name: 'note 申请值', exact: true });
    assert.equal(await copiedNote.getAttribute('readonly'), '');
    assert.equal(await copiedNote.inputValue(), rawCR.replace(/\r\n?/g, '\n'));
    await button('note 申请值：转换为 LF 再编辑').click();
    assert.equal(await copiedNote.getAttribute('readonly'), null);
    assert.equal(await copiedNote.inputValue(), rawCR.replace(/\r\n?/g, '\n'));
    await page.getByRole('dialog', { name: `编辑多表草稿`, exact: true }).locator('.drawer-footer').getByRole('button', { name: '关闭', exact: true }).click();
    await nestedLeave.waitFor();
    await button('放弃修改并离开').click();
    assert.equal(await page.locator('[data-modal-surface="true"]').count(), 0);
    assert.equal(await page.evaluate(() => document.body.classList.contains('modal-open')), false);
    pass('real MySQL rejection freezes raw CR; a copied draft requires explicit LF conversion before editing', {
      status: 422,
      rejectedOrder: invalidOrder.id,
      rawHex: Buffer.from(rawCR).toString('hex'),
      submittedHex: Buffer.from(failureBody.items[0].content.note).toString('hex'),
      clipboardBoundary: 'synthetic paste Event with injected DataTransfer; no OS clipboard access',
    });

    // API fault injection happens after the release draft is durably created.
    await managed({ width: 390, height: 640 });
    await button('新增记录').click();
    await page.getByRole('dialog', { name: `新增 ${table} 记录`, exact: true }).waitFor();
    const unknownName = `stage5_${engineName}_unknown`;
    cleanupNames.add(unknownName);
    await include('name', unknownName);
    assert.equal(await page.getByRole('checkbox', { name: '包含 id', exact: true }).isChecked(), false);
    const writePath = '/api/v1/release-orders';
    const unknownRequestStart = requests.length;
    await page.route(`**${writePath}`, async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      const response = await route.fetch();
      faultEvidence = { actualStatus: response.status(), requestId: response.headers()['x-request-id'], injectedStatus: 503 };
      await route.fulfill({ status: 503, json: { error: { code: 'release_order_unavailable', message: 'Injected after durable draft creation', request_id: faultEvidence.requestId } } });
    });
    await button('查看 Change Set').click();
    await button('确认并保存草稿').click();
    const uncertain = page.getByRole('alert').filter({ hasText: '原请求已保留' });
    await uncertain.waitFor();
    assert.deepEqual(faultEvidence && { actualStatus: faultEvidence.actualStatus, injectedStatus: faultEvidence.injectedStatus }, { actualStatus: 201, injectedStatus: 503 });
    assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(unknownName)};`), '0');
    assert.equal(await button('确认并保存草稿').isEnabled(), true);
    assert.equal(await visibleInViewport(button('确认并保存草稿')), true);
    assert.ok((await pageOverflow()) <= 1);
    await page.unroute(`**${writePath}`);
    page.once('dialog', dialog => dialog.accept());
    await page.reload();
    // Continue through the restored mobile workspace without a second document
    // navigation interrupting its initial session and field-configuration reads.
    await page.getByRole('combobox', { name: 'Managed Table', exact: true }).waitFor();
    await button('打开导航').click();
    await page.getByRole('link', { name: '发布单', exact: true }).click();
    await page.getByRole('alertdialog', { name: '放弃未保存的修改？', exact: true }).waitFor();
    await button('放弃修改并离开').click();
    await button('新建草稿').click();
    await button('确认并保存草稿').click();
    await page.waitForURL('**/configuration/release-orders/*');
    const draftWrites = requests.slice(unknownRequestStart).filter((entry) => entry.method === 'POST' && entry.path === writePath);
    assert.equal(draftWrites.length, 2);
    assert.deepEqual(draftWrites[0], draftWrites[1], 'recovery must replay the original release body and idempotency key');
    const orderID = new URL(page.url()).pathname.split('/').pop();
    const recoveredDraft = await (await authenticatedRequest(context, base, `/api/v1/release-orders/${orderID}`)).json();
    assert.deepEqual(recoveredDraft.table_names, [table]);
    assert.equal(recoveredDraft.applicant_id, account.accountID);
    const published = await publishRelease(recoveredDraft);
    assert.equal(published.state, 'SUCCEEDED');
    assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(unknownName)};`), '1');
    await page.screenshot({ path: `${output}/write-recovery-390.png`, fullPage: true });
    pass('390px unknown release-draft recovery replays one intent and then publishes it', {
      faultEvidence,
      releaseDraftRequests: draftWrites.length,
      replayedOriginalBodyAndKey: true,
      releaseOrder: published.id,
      sqlRows: 1,
      faultBoundary: 'Playwright API response fault injection after route.fetch durably created the release draft',
      documentOverflow: await pageOverflow(),
    });

    assert.equal(requests.some((entry) => /^\/api\/v1\/tables\/[^/]+\/rows(?:\/|$)/.test(entry.path) && !['GET', 'HEAD'].includes(entry.method)), false, 'browser must not use removed direct record write routes');

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
