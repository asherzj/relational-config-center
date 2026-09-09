// Real Chrome → same-origin Admin process → isolated MySQL rollback acceptance.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { join } = require('node:path');
const { mkdir, writeFile } = require('node:fs/promises');
const { browserOptions, selectedBrowser, registerFixtureAccount } = require('./local-account.cjs');

const base = process.env.RCC_WEB_URL;
const engineName = process.env.RCC_E2E_ENGINE || 'chromium';
const table = `stage1_rollback_${engineName}_items`;
const output = process.env.RCC_E2E_OUTPUT;

(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
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
  const account = async names => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await registerFixtureAccount(context, base, { roles: names });
    return context;
  };
  const button = (page, name) => page.getByRole('button', { name, exact: true });
  const heading = (page, state, reverse = false) => ({ waitFor: async () => {
    await page.getByRole('heading', { name: reverse ? '回滚：浏览器回滚验收变更' : '浏览器回滚验收变更', exact: true }).waitFor();
    await page.getByText(`${table} · ${state}`, { exact: true }).waitFor();
  } });
  const read = (context, id) => api(context, 'GET', `/api/v1/release-orders/${id}`);
  const query = context => api(context, 'POST', `/api/v1/tables/${table}/query`, {
    conditions: [{ field: 'id', operator: 'exact', value: '1' }],
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
    assert.equal(before.rows[0].name, 'Rollback seed');
    assert.equal(before.record_versions[0], '0');
    const forward = await api(applicant, 'POST', '/api/v1/release-orders', {
      title: '浏览器回滚验收变更',
      table_name: table,
      items: [{ operation: 'MODIFY', id: '1', expected_record_version: '0', content: { name: `T8 ${engineName} browser published value` } }],
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
    await heading(publishPage, '已发布待完结').waitFor();
    const changed = await query(applicant);
    assert.equal(changed.rows[0].name, `T8 ${engineName} browser published value`);
    assert.equal(changed.record_versions[0], '1');
    check('separate EDITOR, APPROVER and PUBLISHER accounts complete the forward publication');

    const completionWrites = [];
    const completionRoute = `**/api/v1/release-orders/${forward.id}/complete`;
    publisher.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith(`/${forward.id}/complete`))
        completionWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
    });
    await button(publishPage, '完结发布单').click();
    await publishPage.getByText(/释放全部目标记录的占用/).waitFor();
    await publishPage.getByText(/关闭快速回滚/).waitFor();
    assert.equal(await publishPage.getByRole('textbox').count(), 0);
    if (output) await publishPage.screenshot({ path: join(output, 'release-completion-confirm-desktop.png'), fullPage: false, animations: 'disabled' });
    await publishPage.setViewportSize({ width: 390, height: 844 });
    await publishPage.waitForFunction(() => {
      const dialog = document.querySelector('[role="dialog"][data-modal-surface="true"]');
      if (!dialog) return false;
      const rect = dialog.getBoundingClientRect();
      return rect.left >= -1 && rect.right <= innerWidth + 1 && rect.width > 0;
    });
    const confirmationBounds = await button(publishPage, '确认完结').boundingBox();
    assert.ok(confirmationBounds && confirmationBounds.x >= 0 && confirmationBounds.x + confirmationBounds.width <= 391);
    await assertMobileLayout(publishPage, 'release-completion-confirm');
    if (output) await publishPage.screenshot({ path: join(output, 'release-completion-confirm-mobile.png'), fullPage: false, animations: 'disabled' });
    await publishPage.keyboard.press('Escape');
    assert.equal(completionWrites.length, 0);
    assert.equal((await read(publisher, forward.id)).state, 'SUCCEEDED');
    await publishPage.setViewportSize({ width: 1440, height: 1000 });
    await button(publishPage, '完结发布单').click();
    await publishPage.route(completionRoute, async route => {
      const response = await route.fetch();
      assert.equal(response.status(), 200, await response.text());
      assert.equal((await response.json()).state, 'COMPLETED');
      await route.abort('failed');
    });
    await button(publishPage, '确认完结').dblclick();
    await button(publishPage, '使用原请求重试').waitFor();
    assert.equal(completionWrites.length, 1);
    await publishPage.unroute(completionRoute);
    publishPage.once('dialog', dialog => dialog.accept());
    await publishPage.reload();
    await button(publishPage, '恢复原发布请求').click();
    await heading(publishPage, '已完结').waitFor();
    assert.equal(completionWrites.length, 2);
    assert.deepEqual(completionWrites[0], completionWrites[1]);
    assert.deepEqual(await query(applicant), changed);
    const completed = await read(publisher, forward.id);
    assert.equal(completed.history.filter(event => event.action === 'COMPLETE').length, 1);
    assert.equal(completed.publication.notification.status, 'NOT_CONNECTED');
    check('completion confirms release and closing quick rollback, cancellation does not write, repeated clicks and lost response restore the original request without changing configuration');

    await applicantPage.reload();
    assert.equal(await button(applicantPage, '申请回滚').count(), 0);
    assert.equal(await button(publishPage, '快速回滚').count(), 0);
    check('completion permanently closes both rollback entry points');

    const quickTitle = '浏览器快速回滚验收';
    const quickHeading = async (page, state, reverse = false, title = quickTitle) => {
      await page.getByRole('heading', { name: reverse ? `回滚：${title}` : title, exact: true }).waitFor();
      await page.getByText(`${table} · ${state}`, { exact: true }).waitFor();
    };
    const quickDraft = await api(applicant, 'POST', '/api/v1/release-orders', {
      title: quickTitle, table_name: table,
      items: [{ operation: 'MODIFY', id: '1', expected_record_version: '1', content: { name: `Quick ${engineName} published value` } }],
    }, 201);
    const quickURL = `${base}/configuration/release-orders/${quickDraft.id}`;
    await applicantPage.setViewportSize({ width: 1440, height: 1000 });
    await applicantPage.goto(quickURL);
    await button(applicantPage, '提交审批').click();
    await button(applicantPage, '确认提交审批').click();
    await quickHeading(applicantPage, '待审批');
    await reviewPage.goto(quickURL);
    await button(reviewPage, '批准发布单').click();
    await reviewPage.getByLabel('审批意见', { exact: true }).fill('Review forward change before quick restoration');
    await button(reviewPage, '确认批准').click();
    await quickHeading(reviewPage, '已批准');
    await publishPage.goto(quickURL);
    await button(publishPage, '执行发布').click();
    await button(publishPage, '确认发布到数据库').click();
    await quickHeading(publishPage, '已发布待完结');
    await applicantPage.reload();
    assert.equal(await button(applicantPage, '快速回滚').count(), 0);
    const quickPublished = await query(applicant);
    assert.equal(quickPublished.rows[0].name, `Quick ${engineName} published value`);
    assert.equal(quickPublished.record_versions[0], '2');
    const quickWrites = [], quickPreviews = [];
    publisher.on('request', request => {
      if (request.method() !== 'POST') return;
      if (request.url().endsWith(`/${quickDraft.id}/quick-rollback`))
        quickWrites.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
      if (request.url().endsWith(`/${quickDraft.id}/quick-rollback/preview`)) quickPreviews.push(request.postData());
    });
    await button(publishPage, '快速回滚').click();
    const quickDialog = publishPage.getByRole('dialog', { name: `快速回滚 · ${quickTitle}`, exact: true });
    await quickDialog.getByRole('region', { name: '整单恢复预览', exact: true }).waitFor();
    await quickDialog.getByRole('columnheader', { name: '当前值', exact: true }).waitFor();
    await quickDialog.getByRole('columnheader', { name: '恢复值', exact: true }).waitFor();
    await quickDialog.getByText(`Quick ${engineName} published value`, { exact: true }).waitFor();
    await quickDialog.getByText(`T8 ${engineName} browser published value`, { exact: true }).waitFor();
    assert.equal(await quickDialog.getByLabel('快速回滚原因（选填）', { exact: true }).getAttribute('required'), null);
    assert.equal(await button(quickDialog, '确认整单快速回滚').isDisabled(), false);
    assert.equal(await button(quickDialog, '取消快速回滚').evaluate(element => element === document.activeElement), true);
    if (output) await publishPage.screenshot({ path: join(output, 'release-quick-rollback-preview-desktop.png'), fullPage: false, animations: 'disabled' });
    await publishPage.setViewportSize({ width: 390, height: 844 });
    await assertMobileLayout(publishPage, 'release-quick-rollback-preview');
    if (output) await publishPage.screenshot({ path: join(output, 'release-quick-rollback-preview-mobile.png'), fullPage: false, animations: 'disabled' });
    await button(quickDialog, '取消快速回滚').click();
    assert.equal(quickWrites.length, 0);
    assert.equal((await read(publisher, quickDraft.id)).state, 'SUCCEEDED');
    assert.deepEqual(await query(applicant), quickPublished);
    check('quick rollback is publisher-only, reviews actual current and restoration values, has no mandatory reason and cancels without writes on desktop and 390px');

    await publishPage.setViewportSize({ width: 1440, height: 1000 });
    await button(publishPage, '快速回滚').click();
    await quickDialog.getByRole('region', { name: '整单恢复预览', exact: true }).waitFor();
    const quickReason = ''; // AC-010: one confirmation with no mandatory reason.
    await quickDialog.getByLabel('快速回滚原因（选填）', { exact: true }).fill(quickReason);
    const previewCountBeforeWrite = quickPreviews.length;
    const quickRoute = `**/api/v1/release-orders/${quickDraft.id}/quick-rollback`;
    let committedQuick;
    await publishPage.route(quickRoute, async route => {
      const response = await route.fetch();
      assert.equal(response.status(), 200, await response.text());
      committedQuick = await response.json();
      assert.equal(committedQuick.state, 'ROLLED_BACK');
      await route.abort('failed');
    });
    await button(quickDialog, '确认整单快速回滚').dblclick();
    await button(quickDialog, '使用原请求重试').waitFor();
    assert.equal(quickWrites.length, 1);
    assert.equal(await button(publishPage, '完结发布单').isDisabled(), true);
    assert.equal(await button(publishPage, '快速回滚').isDisabled(), true);
    assert.equal(await quickDialog.getByLabel('快速回滚原因（选填）', { exact: true }).isDisabled(), true);
    assert.equal(quickPreviews.length, previewCountBeforeWrite);
    if (output) await publishPage.screenshot({ path: join(output, 'release-quick-rollback-unknown.png'), fullPage: false, animations: 'disabled' });
    await publishPage.unroute(quickRoute);
    publishPage.once('dialog', dialog => dialog.accept());
    await publishPage.reload();
    await button(publishPage, '恢复原发布请求').click();
    await quickHeading(publishPage, '已回滚');
    assert.equal(quickWrites.length, 2);
    assert.deepEqual(quickWrites[0], quickWrites[1]);
    assert.equal(quickPreviews.length, previewCountBeforeWrite);
    assert.equal(new URL(publishPage.url()).pathname.split('/').pop(), committedQuick.id);
    const quickOriginal = await read(applicant, quickDraft.id);
    const quickRestored = await query(applicant);
    const actualPublisher = (await identity(publisher)).account.id;
    assert.equal(quickOriginal.id, committedQuick.id);
    assert.equal(quickOriginal.id, quickDraft.id);
    assert.equal(quickOriginal.state, 'ROLLED_BACK');
    assert.equal(quickOriginal.rollback.publisher_id, actualPublisher);
    assert.equal(quickOriginal.applicant_id, (await identity(applicant)).account.id);
    assert.equal(quickOriginal.history.length, 5);
    assert.equal(quickOriginal.history.at(-1).action, 'QUICK_ROLLBACK');
    assert.equal(quickOriginal.history.at(-1).reason, quickReason);
    assert.equal(quickOriginal.history.at(-1).actor_id, actualPublisher);
    assert.equal(quickOriginal.history.filter(event => event.action === 'APPROVE').length, 1);
    assert.deepEqual(quickOriginal.items, (await read(applicant, quickDraft.id)).items);
    assert.equal(quickOriginal.executions.length, 2);
    assert.notEqual(quickOriginal.publication.notification.id, quickOriginal.rollback.notification.id);
    assert.equal(quickRestored.rows[0].name, `T8 ${engineName} browser published value`);
    assert.equal(quickRestored.record_versions[0], '3');
    for (const action of ['快速回滚', '完结发布单', '申请回滚', '批准发布单', '执行发布']) assert.equal(await button(publishPage, action).count(), 0);
    const actualResult = publishPage.getByRole('region', {name:'发布结果',exact:true});
    await actualResult.getByText(`值：Quick ${engineName} published value`,{exact:true}).waitFor();
    await button(publishPage,'恢复结果').click();
    await actualResult.getByText(`值：T8 ${engineName} browser published value`,{exact:true}).waitFor();
    await button(publishPage,'申请差异').click();
    await publishPage.getByText(`Quick ${engineName} published value`,{exact:true}).waitFor();
    check('the original detail switches preserved application, actual publication and actual rollback');
    await publishPage.setViewportSize({ width: 390, height: 844 });
    await assertMobileLayout(publishPage, 'release-quick-rollback-result');
    await publishPage.evaluate(() => scrollTo(0, 0));
    if (output) await publishPage.screenshot({ path: join(output, 'release-quick-rollback-result-mobile.png'), fullPage: true, animations: 'disabled' });
    check('quick rollback commits once without approval; lost response and reload recover the same preview digest, reason and key, record the actual publisher and finish the original order');

    // Another active publisher can complete after this browser has reviewed a
    // restoration. Its real server write must win without partial restoration.
    const competingPublisher = await account(['PUBLISHER']);
    const competingDraft = await api(applicant, 'POST', '/api/v1/release-orders', {
      title: '浏览器完结与快速回滚竞争', table_name: table,
      items: [{ operation: 'MODIFY', id: '1', expected_record_version: '3', content: { name: 'Completion wins the reviewed quick rollback' } }],
    }, 201);
    const submittedCompetition = await api(applicant, 'POST', `/api/v1/release-orders/${competingDraft.id}/submit`, { expected_version: competingDraft.version });
    const approvedCompetition = await api(reviewer, 'POST', `/api/v1/release-orders/${competingDraft.id}/approve`, { expected_version: submittedCompetition.version, reason: 'Independent approval' });
    const publishedCompetition = await api(publisher, 'POST', `/api/v1/release-orders/${competingDraft.id}/execute`, { expected_version: approvedCompetition.version });
    await publishPage.setViewportSize({ width: 1440, height: 1000 });
    await publishPage.goto(`${base}/configuration/release-orders/${competingDraft.id}`);
    await button(publishPage, '快速回滚').click();
    await publishPage.getByRole('region', { name: '整单恢复预览', exact: true }).waitFor();
    await publishPage.getByLabel('快速回滚原因（选填）', { exact: true }).fill('Retain reason when completion wins');
    const completedCompetition = await api(competingPublisher, 'POST', `/api/v1/release-orders/${competingDraft.id}/complete`, { expected_version: publishedCompetition.version });
    const competingResponse = publishPage.waitForResponse(response => response.url().endsWith(`/${competingDraft.id}/quick-rollback`) && response.request().method() === 'POST');
    await button(publishPage, '确认整单快速回滚').click();
    assert.equal((await competingResponse).status(), 409);
    await publishPage.getByText('原原因已保留。请关闭此窗口，查看最新状态与配置后重新审阅恢复预览。', { exact: true }).waitFor();
    assert.equal(await publishPage.getByLabel('快速回滚原因（选填）', { exact: true }).inputValue(), 'Retain reason when completion wins');
    await button(publishPage, '取消快速回滚').click();
    await button(publishPage, '查看最新状态与配置').click();
    await publishPage.getByText(/最新发布单版本：.*状态：COMPLETED/).waitFor();
    assert.equal(await button(publishPage, '确认按最新状态快速回滚').count(), 0);
    assert.deepEqual(await read(publisher, competingDraft.id), completedCompetition);
    const competitionRows = await query(applicant);
    assert.equal(competitionRows.rows[0].name, 'Completion wins the reviewed quick rollback');
    assert.equal(competitionRows.record_versions[0], '4');
    assert.equal(completedCompetition.history.filter(event => event.action === 'COMPLETE').length, 1);
    assert.equal(completedCompetition.history.filter(event => event.action === 'QUICK_ROLLBACK').length, 0);
    check('a real competing completion rejects an already reviewed quick rollback, preserves its reason and prevents rebuilding after the original order ends');

    assert.deepEqual(errors, []);
    if (output) await writeFile(join(output, 'rollback-evidence.json'), JSON.stringify({
      checks,
      original_order_id: forward.id,
      rollback_request_replayed_with_same_key: true,
      quick_original_order_id: quickOriginal.id,
      quick_actual_publisher_id: actualPublisher,
      quick_request_replayed_with_same_key_body: true,
      quick_preview_reads: quickPreviews.length,
      quick_restored_record_version: quickRestored.record_versions[0],
      competition_completed_order_id: completedCompetition.id,
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
