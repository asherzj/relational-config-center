const assert = require('node:assert/strict');
const { chromium, request } = require('playwright');
const { writeFile } = require('node:fs/promises');
const { join } = require('node:path');

const origin = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
assert.ok(origin && output, 'isolated browser origin and evidence directory are required');
const code = 'browser_standard_v1';
const checks = [];

(async () => {
  const launchOptions = process.env.RCC_BROWSER_EXECUTABLE
    ? { executablePath: process.env.RCC_BROWSER_EXECUTABLE, headless: true }
    : { channel: 'chrome', headless: true };
  const browser = await chromium.launch(launchOptions);
  try {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await context.newPage();
    await page.goto(`${origin}/login`);
    await page.getByLabel('用户名').fill('template.browser.admin');
    await page.getByLabel('密码').fill('template browser password long enough');
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await page.waitForURL(url => !url.pathname.endsWith('/login'));
    await page.goto(`${origin}/platform/release-templates`);
    await page.getByRole('heading', { name: '发布流程模板', exact: true }).waitFor();
    await page.getByRole('button', { name: '查看' }).first().waitFor();
    assert.equal(await page.getByRole('button', { name: '查看' }).count(), 2, 'new install exposes both default templates');
    await page.screenshot({ path: join(output, 'release-template-catalog-desktop.png'), fullPage: true });
    checks.push('desktop catalog and two defaults');

    await page.getByRole('button', { name: '新建模板' }).click();
    await page.getByLabel(/模板编码/).fill(code);
    await page.getByLabel('模板名称').fill('浏览器常规模板');
    await page.getByLabel('发布类型', { exact: true }).selectOption('STANDARD');
    assert.deepEqual(await page.locator('.template-node').evaluateAll(nodes => nodes.map(node => node.getAttribute('aria-label'))), ['1. 按表审批', '2. 发布', '3. 完结']);
    await page.getByRole('button', { name: '保存模板' }).click();
    await page.getByText('发布流程模板已创建', { exact: true }).waitFor();
    await page.getByText(code, { exact: true }).waitFor();
    assert.equal(await page.getByRole('button', { name: '查看' }).count(), 3, 'same release type accepts more than one template');
    checks.push('constrained create and multiple STANDARD templates');

    await page.goto(`${origin}/platform/release-templates/${code}?mode=edit`);
    await page.getByLabel('模板名称').fill('浏览器结果未知重试');
    const unknownWrites = [];
    let interruptOnce = true;
    await page.route(`**/api/v1/release-templates/${code}`, async route => {
      const request = route.request();
      if (request.method() !== 'PUT') return route.continue();
      unknownWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
      if (!interruptOnce) return route.continue();
      interruptOnce = false;
      const response = await route.fetch();
      assert.equal(response.status(), 200, 'server-side write must commit before response interruption');
      return route.abort('connectionreset');
    });
    await page.getByRole('button', { name: '保存模板' }).click();
    await page.getByText('保存结果未知；请保持当前页面，再次点击保存将原样重推同一请求。', { exact: true }).waitFor();
    assert.equal(await page.getByLabel('模板名称').isDisabled(), true, 'unknown write must freeze its original fields');
    await page.getByRole('button', { name: '重推原请求' }).click();
    await page.getByText('发布流程模板已更新', { exact: true }).waitFor();
    assert.equal(unknownWrites.length, 2);
    assert.deepEqual(unknownWrites[0], unknownWrites[1], 'manual retry changed original write identity or body');
    await page.unroute(`**/api/v1/release-templates/${code}`);
    const currentAfterRetry = await page.evaluate(async value => (await fetch(`/api/v1/release-templates/${value}`)).json(), code);
    assert.equal(currentAfterRetry.name, '浏览器结果未知重试');
    checks.push('unknown result manually replays exact committed request');

    const stale = await context.newPage();
    let staleWrite;
    stale.on('request', request => {
      if (request.method() === 'PUT' && new URL(request.url()).pathname === `/api/v1/release-templates/${code}`) staleWrite = JSON.parse(request.postData());
    });
    await stale.goto(`${origin}/platform/release-templates/${code}?mode=edit`);
    const staleName = stale.getByLabel('模板名称');
    await staleName.waitFor();
    await staleName.fill('陈旧管理员输入');
    await page.goto(`${origin}/platform/release-templates/${code}?mode=edit`);
    await page.getByLabel('模板名称').fill('浏览器常规模板已更新');
    await page.getByRole('button', { name: '保存模板' }).click();
    await page.getByText('发布流程模板已更新', { exact: true }).waitFor();
    const refreshed = stale.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === `/api/v1/release-templates/${code}`);
    await stale.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
    assert.equal((await (await refreshed).json()).version, '3', 'background detail did not refresh to the current version');
    await stale.getByRole('button', { name: '保存模板' }).click();
    await stale.getByText(/模板已被其他管理员修改/).waitFor();
    assert.equal(await staleName.inputValue(), '陈旧管理员输入');
    assert.equal(staleWrite.expected_version, '2', 'old form adopted the refreshed detail version');
    await stale.screenshot({ path: join(output, 'release-template-version-conflict.png') });
    await stale.close();
    checks.push('stale version rejected with input retained');

    await page.goto(`${origin}/platform/release-templates/default_emergency_v1`);
    assert.equal(await page.getByRole('button', { name: '停用' }).isDisabled(), true);
    assert.equal(await page.getByRole('button', { name: '删除' }).isDisabled(), true);
    checks.push('emergency lifecycle protected');

    await page.goto(`${origin}/platform/release-templates/${code}`);
    await page.getByRole('button', { name: '停用' }).click();
    const confirm = page.getByRole('alertdialog', { name: /停用发布流程模板/ });
    await confirm.waitFor();
    assert.equal(await page.getByRole('button', { name: '取消', exact: true }).evaluate(node => node === document.activeElement), true, 'cancel receives safe default focus');
    await page.keyboard.press('Escape');
    assert.equal(await confirm.count(), 0);
    checks.push('keyboard-safe destructive confirmation');

    const lifecycleWrites = [];
    let interruptLifecycleOnce = true;
    await page.route(`**/api/v1/release-templates/${code}/disable`, async route => {
      const request = route.request();
      lifecycleWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'], path: new URL(request.url()).pathname });
      if (!interruptLifecycleOnce) return route.continue();
      interruptLifecycleOnce = false;
      const response = await route.fetch();
      assert.equal(response.status(), 200);
      return route.abort('connectionreset');
    });
    await page.getByRole('button', { name: '停用' }).click();
    await page.getByRole('button', { name: '确认停用' }).click();
    await page.getByText('操作结果未知；再次确认将原样重推同一请求。', { exact: true }).waitFor();
    await page.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(await page.getByRole('button', { name: '删除' }).isDisabled(), true, 'unknown disable allowed a new delete intent');
    assert.equal(await page.getByRole('button', { name: '重推停用请求' }).isEnabled(), true);
    await page.getByRole('button', { name: '重推停用请求' }).click();
    await page.getByRole('button', { name: '重推原请求' }).click();
    await page.getByText('发布流程模板已停用', { exact: true }).waitFor();
    assert.deepEqual(lifecycleWrites[0], lifecycleWrites[1], 'lifecycle retry changed action, target, body, or key');
    const currentAfterDisable = await page.evaluate(async value => (await fetch(`/api/v1/release-templates/${value}`)).json(), code);
    assert.equal(currentAfterDisable.enabled, false);
    await page.unroute(`**/api/v1/release-templates/${code}/disable`);
    checks.push('unknown lifecycle remains bound to original action and target');

    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`${origin}/platform/release-templates`);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'catalog overflows 390px viewport');
    await page.getByRole('button', { name: '查看' }).last().click();
    const mobileDrawer = page.getByRole('dialog');
    await mobileDrawer.waitFor();
    await mobileDrawer.evaluate(async element => {
      for (;;) {
        const active = element.getAnimations({ subtree: true }).filter(animation => animation.pending || animation.playState === 'running');
        if (active.length === 0) break;
        await Promise.all(active.map(animation => animation.finished.catch(error => {
          if (error.name !== 'AbortError') throw error;
        })));
      }
    });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'drawer overflows 390px viewport');
    await page.screenshot({ path: join(output, 'release-template-mobile.png'), fullPage: true });
    checks.push('390px catalog and drawer');

    const viewerAPI = await request.newContext({ baseURL: origin });
    const prepared = await viewerAPI.get('/api/v1/auth/csrf');
    const { csrf_token: csrf } = await prepared.json();
    const registered = await viewerAPI.post('/api/v1/auth/register', {
      headers: { Origin: origin, 'X-CSRF-Token': csrf },
      data: { username: 'template.browser.viewer', email: 'template.browser.viewer@example.invalid', password: 'template viewer password long enough' },
    });
    assert.equal(registered.status(), 201);
    const viewerContext = await browser.newContext({ storageState: await viewerAPI.storageState() });
    const viewer = await viewerContext.newPage();
    await viewer.goto(`${origin}/platform/release-templates`);
    await viewer.getByRole('heading', { name: '发布流程模板', exact: true }).waitFor();
    assert.equal(await viewer.getByRole('link', { name: '发布流程模板', exact: true }).count(), 0, 'viewer sees admin navigation');
    assert.equal(await viewer.getByRole('button', { name: '新建模板' }).isDisabled(), true);
    await viewer.getByText('当前账号没有执行此操作的角色，请联系管理员授权。').waitFor();
    await page.goto(`${origin}/platform/release-templates`);
    await page.getByText(code, { exact: true }).waitFor();
    assert.equal(await page.getByText(code, { exact: true }).count(), 1, 'viewer session changed administrator page');
    await viewerContext.close();
    await viewerAPI.dispose();
    checks.push('viewer denied without disturbing admin session');

    await writeFile(join(output, 'release-template-browser-evidence.json'), JSON.stringify({ code, checks }, null, 2));
    process.stdout.write(JSON.stringify({ code, checks }));
    await context.close();
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
