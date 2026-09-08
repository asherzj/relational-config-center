// Real Chrome → same-origin Admin process → isolated MySQL rollback acceptance.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { join } = require('node:path');
const { mkdir, writeFile } = require('node:fs/promises');
const { browserOptions, registerFixtureAccount } = require('./local-account.cjs');

const base = process.env.RCC_WEB_URL;
const table = 'stage1_acceptance_items';
const output = process.env.RCC_E2E_OUTPUT;

(async () => {
  const browser = await chromium.launch(browserOptions());
  const checks = [], errors = [];
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
  const roles = async (context, names) => {
    const session = await identity(context);
    await api(context, 'PUT', `/api/v1/account-roles/${session.account.id}`, { roles: names, expected_version: '2' });
  };
  const account = async names => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await registerFixtureAccount(context, base);
    await roles(context, names);
    return context;
  };
  const button = (page, name) => page.getByRole('button', { name, exact: true });
  const heading = (page, state) => page.getByRole('heading', { name: `${table} · ${state}`, exact: true });
  const read = (context, id) => api(context, 'GET', `/api/v1/release-orders/${id}`);
  const query = context => api(context, 'POST', `/api/v1/tables/${table}/query`, {
    conditions: [{ field: 'id', operator: 'exact', value: '4' }],
  });
  const assertMobileLayout = async (page, name) => {
    const layout = await page.evaluate(() => ({
      viewport: innerWidth,
      document: document.documentElement.scrollWidth,
      overflowing: [...document.querySelectorAll('body *')].flatMap(element => {
        const rect = element.getBoundingClientRect();
        return rect.width > 0 && rect.right > innerWidth + 1 ? [{
          tag: element.tagName, role: element.getAttribute('role'), classes: element.className,
          text: element.textContent.slice(0, 120), left: rect.left, right: rect.right, width: rect.width,
        }] : [];
      }).slice(0, 30),
    }));
    if (output) await writeFile(join(output, `${name}-layout.json`), JSON.stringify(layout, null, 2));
    assert.ok(layout.document <= layout.viewport, `${name} overflows mobile: ${JSON.stringify(layout)}`);
  };
  let applicantPage, reviewPage, publishPage;
  try {
    if (output) await mkdir(output, { recursive: true });
    const applicant = await account(['EDITOR']);
    const reviewer = await account(['APPROVER']);
    const publisher = await account(['PUBLISHER']);
    applicantPage = await applicant.newPage();
    reviewPage = await reviewer.newPage();
    publishPage = await publisher.newPage();
    for (const page of [applicantPage, reviewPage, publishPage]) {
      page.setDefaultTimeout(15000);
      page.on('pageerror', error => errors.push(error.message));
    }

    const before = await query(applicant);
    assert.equal(before.rows[0].name, 'Delta');
    assert.equal(before.record_versions[0], '0');
    const forward = await api(applicant, 'POST', '/api/v1/release-orders', {
      table_name: table,
      items: [{ operation: 'MODIFY', id: '4', expected_record_version: '0', content: { name: 'T7 browser published value' } }],
    }, 201);
    const forwardURL = `${base}/configuration/release-orders/${forward.id}`;
    await applicantPage.goto(forwardURL);
    await button(applicantPage, '提交审批').click();
    await button(applicantPage, '确认提交审批').click();
    await heading(applicantPage, '待审批').waitFor();
    await reviewPage.goto(forwardURL);
    await button(reviewPage, '批准发布单').click();
    await reviewPage.getByLabel('审批意见', { exact: true }).fill('Independent forward review');
    await button(reviewPage, '确认批准').click();
    await heading(reviewPage, '已批准').waitFor();
    await publishPage.goto(forwardURL);
    await button(publishPage, '执行发布').click();
    await button(publishPage, '确认发布到数据库').click();
    await heading(publishPage, '已发布').waitFor();
    const changed = await query(applicant);
    assert.equal(changed.rows[0].name, 'T7 browser published value');
    assert.equal(changed.record_versions[0], '1');
    check('separate EDITOR, APPROVER and PUBLISHER accounts complete the forward publication');

    await applicantPage.reload();
    const rollbackWrites = [];
    let committedReverse;
    const rollbackRoute = `**/api/v1/release-orders/${forward.id}/rollback`;
    applicant.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith(`/${forward.id}/rollback`)) {
        rollbackWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
      }
    });
    await applicantPage.route(rollbackRoute, async route => {
      const response = await route.fetch();
      assert.equal(response.status(), 201, await response.text());
      committedReverse = await response.json();
      await route.abort('failed');
    });
    await button(applicantPage, '申请回滚').click();
    await applicantPage.getByLabel('回滚原因', { exact: true }).fill('Restore the reviewed production value');
    await button(applicantPage, '创建回滚草稿').click();
    await button(applicantPage, '使用原请求重试').waitFor();
    await applicantPage.unroute(rollbackRoute);
    applicantPage.once('dialog', dialog => dialog.accept());
    await applicantPage.reload();
    await button(applicantPage, '恢复原发布请求').click();
    await heading(applicantPage, '草稿').waitFor();
    const reverseID = new URL(applicantPage.url()).pathname.split('/').pop();
    assert.equal(reverseID, committedReverse.id);
    assert.equal(rollbackWrites.length, 2);
    assert.deepEqual(rollbackWrites[0], rollbackWrites[1]);
    assert.equal(await button(applicantPage, '编辑草稿').count(), 0);
    assert.equal(await applicantPage.getByRole('link', { name: '添加明细', exact: true }).count(), 0);
    assert.equal(await button(applicantPage, '复制新草稿').count(), 0);
    await applicantPage.getByText('明细来自原发布的实际结果，不可编辑或复制。', { exact: false }).waitFor();
    let original = await read(applicant, forward.id);
    let reverse = await read(applicant, reverseID);
    assert.equal(original.rollback_order_id, reverseID);
    assert.equal(original.rollback_pending, true);
    assert.equal(reverse.rollback_of_id, forward.id);
    assert.ok(original.history.some(event => event.related_order_id === reverseID));
    assert.ok(reverse.history.some(event => event.related_order_id === forward.id));
    check('a lost rollback response recovers the same linked read-only draft with the original request key and reason');

    await button(applicantPage, '提交审批').click();
    await button(applicantPage, '确认提交审批').click();
    await heading(applicantPage, '待审批').waitFor();
    const reverseURL = applicantPage.url();
    await reviewPage.goto(reverseURL);
    await button(reviewPage, '批准发布单').click();
    await reviewPage.getByLabel('审批意见', { exact: true }).fill('Independent rollback review');
    await button(reviewPage, '确认批准').click();
    await heading(reviewPage, '已批准').waitFor();
    await publishPage.goto(reverseURL);
    await button(publishPage, '执行发布').click();
    await button(publishPage, '确认发布到数据库').click();
    await heading(publishPage, '已发布').waitFor();
    const restored = await query(applicant);
    assert.equal(restored.rows[0].name, 'Delta');
    assert.equal(restored.record_versions[0], '2');
    original = await read(applicant, forward.id);
    reverse = await read(applicant, reverseID);
    assert.equal(original.state, 'ROLLED_BACK');
    assert.equal(original.rollback_order_id, reverseID);
    assert.equal(original.rollback_pending ?? false, false);
    assert.equal(reverse.state, 'SUCCEEDED');
    assert.ok(original.history.some(event => event.related_order_id === reverseID));
    assert.ok(reverse.history.some(event => event.related_order_id === forward.id));
    check('the reverse order needs a fresh independent approval and publication before restoring the actual row and both links');

    await applicantPage.goto(forwardURL);
    await heading(applicantPage, '已回滚').waitFor();
    await applicantPage.getByText('最新回滚发布单', { exact: false }).waitFor();
    await applicantPage.setViewportSize({ width: 390, height: 844 });
    await applicantPage.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' });
    await assertMobileLayout(applicantPage, 'release-rollback-original');
    if (output) await applicantPage.screenshot({ path: join(output, 'release-rollback-original-mobile.png'), fullPage: true, animations: 'disabled' });
    await applicantPage.goto(reverseURL);
    await heading(applicantPage, '已发布').waitFor();
    await applicantPage.getByText('回滚原发布单', { exact: false }).waitFor();
    await assertMobileLayout(applicantPage, 'release-rollback-reverse');
    if (output) await applicantPage.screenshot({ path: join(output, 'release-rollback-reverse-mobile.png'), fullPage: true, animations: 'disabled' });
    check('original and reverse detail links remain visible at 390px without horizontal overflow');

    assert.deepEqual(errors, []);
    if (output) await writeFile(join(output, 'rollback-evidence.json'), JSON.stringify({
      checks,
      original_order_id: forward.id,
      reverse_order_id: reverseID,
      original_state: original.state,
      reverse_state: reverse.state,
      rollback_request_replayed_with_same_key: true,
      restored_record_version: restored.record_versions[0],
      browser_errors: errors,
    }, null, 2));
    console.log(JSON.stringify({ checks }));
  } catch (error) {
    if (applicantPage && output) await applicantPage.screenshot({ path: join(output, 'release-rollback-failure.png'), fullPage: false }).catch(() => {});
    throw error;
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
