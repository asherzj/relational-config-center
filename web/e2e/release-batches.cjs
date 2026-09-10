const {readAllReleaseDetailPages,executionCommands,applicationItems}=require('./release-detail-pages.cjs');
const {repeatReleaseAction,reopenDraftSave,repeatDraftSave}=require('./release-original-action.cjs');
// Real Chrome → same-origin Admin process → isolated MySQL batch acceptance.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { join } = require('node:path');
const { mkdir, writeFile } = require('node:fs/promises');
const { browserOptions, selectedBrowser, registerFixtureAccount, setFixtureRoles } = require('./local-account.cjs');
const base = process.env.RCC_WEB_URL;
const table = 'batch_browser_items';
const output = process.env.RCC_E2E_OUTPUT;

(async () => {
  const browser = await selectedBrowser(playwright).launch(browserOptions());
  const checks = [], errors = [];
  const check = name => { checks.push(name); console.log('PASS', name); };
  const identity = async context => (await context.request.get(`${base}/api/v1/auth/session`)).json();
  const api = async (context, method, path, data, expected = 200) => {
    const session = await identity(context);
    const response = await context.request.fetch(`${base}${path}`, {
      method, headers: { Origin: base, 'X-CSRF-Token': session.csrf_token, 'Idempotency-Key': randomUUID() },
      ...(data === undefined ? {} : { data }), timeout: 20000,
    });
    assert.equal(response.status(), expected, `${method} ${path}: ${await response.text()}`);
    return response.json();
  };
  const button = (page, name) => page.getByRole('button', { name, exact: true });
  const heading = (page, state, title = `${table} 配置变更`) => ({ waitFor: async () => {
    await page.getByRole('heading', { name: title, exact: true }).waitFor();
    await page.getByText(`${table} · ${state}`, { exact: true }).waitFor();
  } });
  const read = async (context, id) => readAllReleaseDetailPages(context,base,await api(context,'GET',`/api/v1/release-orders/${id}`));
  const query = (context, conditions = [], pageNumber = 1) => api(context, 'POST', `/api/v1/tables/${table}/query`, {
    conditions, order: { field: 'id', direction: 'ASC' }, page_size: 200, page_number: pageNumber,
  });
  const screenshot = async (page, name) => {
    if (output) await page.screenshot({ path: join(output, name), fullPage: false, animations: 'disabled' });
  };
  const assertMobileLayout = async (page, name) => {
    const layout = await page.evaluate(() => ({
      viewport: innerWidth, document: document.documentElement.scrollWidth,
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
  const submit = async (page, count, title) => {
    await button(page, '提交审批').click();
    await page.getByText(`全部 ${count.toLocaleString('en-US')} 项将一起提交，预览分页不改变操作范围。`, { exact: true }).waitFor();
    await button(page, '确认提交审批').click();
    await heading(page, '待审批', title).waitFor();
  };
  const approve = async (review, url, count, title) => {
    await review.goto(url);
    await button(review, '批准发布单').click();
    await review.getByText(`全部 ${count.toLocaleString('en-US')} 项将一起批准，预览分页不改变操作范围。`, { exact: true }).waitFor();
    if (count > 20) {
      const approval = review.getByRole('dialog', { name: '批准发布单', exact: true });
      await approval.getByLabel('定位明细', { exact: true }).fill(String(count));
      await approval.getByRole('region', { name: `明细 ${count}`, exact: true }).getByText(`batch item ${count}`, { exact: true }).waitFor();
    }
    await review.getByLabel('审批意见', { exact: true }).fill(`Independently reviewed all ${count} batch items`);
    await button(review, '确认批准').click();
    await heading(review, '已批准', title).waitFor();
  };
  let page, review;
  try {
    if (output) await mkdir(output, { recursive: true });
    const applicant = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const reviewer = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await registerFixtureAccount(applicant, base);
    // Policy creation/activation/assignment use the same public API as the UI.
    await api(applicant, 'POST', '/api/v1/query-policies', {
      code: 'batch_browser_query_v1', name: 'Batch browser query', description: 'Isolated acceptance', type_code: 'page_query',
      default_order_field: 'id', default_order_direction: 'ASC', default_page_size: 20, max_page_size: 200,
    }, 201);
    await api(applicant, 'POST', '/api/v1/query-policies/batch_browser_query_v1/activate');
    await api(applicant, 'POST', '/api/v1/mutation-policies', {
      code: 'batch_browser_mutation_v1', name: 'Batch browser mutation', description: 'Isolated acceptance',
      type_code: 'single_table_mutation', allow_add: true, allow_modify: true, allow_delete: true,
    }, 201);
    await api(applicant, 'POST', '/api/v1/mutation-policies/batch_browser_mutation_v1/activate');
    await api(applicant, 'POST', '/api/v1/table-policies', {
      table_name: table, query_policy_code: 'batch_browser_query_v1', mutation_policy_code: 'batch_browser_mutation_v1',
    }, 201);
    await api(applicant, 'POST', `/api/v1/table-policies/${table}/enable`,{expected_version:'1'});
    await setFixtureRoles(applicant, base, (await identity(applicant)).account.id, ['EDITOR', 'PUBLISHER']);
    await registerFixtureAccount(reviewer, base, { roles: ['APPROVER'] });
    page = await applicant.newPage();
    review = await reviewer.newPage();
    for (const current of [page, review]) {
      current.setDefaultTimeout(15000);
      current.on('pageerror', error => errors.push(error.message));
    }
    const applicantID = (await identity(applicant)).account.id;
    const reviewerID = (await identity(reviewer)).account.id;
    assert.notEqual(applicantID, reviewerID);

    await page.goto(`${base}/configuration/managed-data`);
    await page.getByLabel('Managed Table', { exact: true }).selectOption(table);
    await button(page, '修改记录 1').click();
    await page.getByLabel('label 值', { exact: true }).fill('initial batch intent');
    await button(page, '查看 Change Set').click();
    await button(page, '确认并保存草稿').click();
    await page.waitForURL('**/configuration/release-orders/*');
    await heading(page, '草稿').waitFor();
    const mixedID = new URL(page.url()).pathname.split('/').pop();
    let order = await read(applicant, mixedID);
    assert.equal(order.items.length, 1);
    assert.equal(order.items[0].operation, 'MODIFY');

    await page.getByRole('link', { name: '添加明细', exact: true }).click();
    assert.equal(await page.getByLabel('Managed Table', { exact: true }).inputValue(), table);
    await page.getByRole('checkbox', { name: '选择记录 2', exact: true }).check();
    await page.getByRole('checkbox', { name: '选择记录 3', exact: true }).check();
    await button(page, '删除已选 2 项').click();
    assert.equal(await page.getByLabel('保存到草稿', { exact: true }).inputValue(), mixedID);
    await page.getByText('将所选 2 项加入发布草稿。现在不会删除配置。', { exact: true }).waitFor();
    await screenshot(page, 'batch-explicit-delete-selection.png');
    await button(page, '确认并保存草稿').click();
    await page.waitForURL(`**/configuration/release-orders/${mixedID}`);
    await heading(page, '草稿').waitFor();
    order = await read(applicant, mixedID);
    assert.deepEqual(order.items.map(item => [item.operation, item.id]), [['MODIFY', '1'], ['DELETE', '2'], ['DELETE', '3']]);
    assert.deepEqual(order.items.map(item => item.expected_record_version), ['0', '0', '0']);
    assert.equal((await query(applicant)).rows.length, 3);
    check('data-page MODIFY creates a draft; detail link preselects its table and destination; two explicit row selections append DELETEs without touching data');

    await button(page, '编辑草稿').click();
    await page.getByLabel('编辑明细', { exact: true }).selectOption('2');
    await button(page, '移除此明细').click();
    assert.equal(await page.getByLabel('编辑明细', { exact: true }).locator('option').count(), 2);
    await page.getByLabel('编辑明细', { exact: true }).selectOption('0');
    await page.getByLabel('label 申请值', { exact: true }).fill('published mixed label');
    const [editResponse] = await Promise.all([
      page.waitForResponse(response => response.request().method() === 'PUT' && new URL(response.url()).pathname === `/api/v1/release-orders/${mixedID}`),
      button(page, '保存草稿修改').click(),
    ]);
    const editEvidence = {
      method: editResponse.request().method(),
      path: new URL(editResponse.url()).pathname,
      request: editResponse.request().postDataJSON(),
      status: editResponse.status(),
      response: await editResponse.text(),
    };
    if (output) await writeFile(join(output, 'batch-edit-response.json'), JSON.stringify(editEvidence, null, 2));
    assert.equal(editEvidence.status, 200, `PUT release order: ${editEvidence.response}`);
    assert.equal(editEvidence.request.items, undefined);
    assert.deepEqual(editEvidence.request.changes.upserts.map(item => [item.operation, item.id]), [['MODIFY', '1']]);
    assert.deepEqual(editEvidence.request.changes.delete_detail_ids, [order.items[2].detail_id]);
    const savedOrder = JSON.parse(editEvidence.response);
    assert.equal(savedOrder.items.length, 2);
    assert.equal(savedOrder.items[0].content.label, 'published mixed label');
    // The edited value is already visible before save completes. Wait for the
    // actual response and the editor to close before reading durable state.
    await page.getByRole('dialog', { name: `编辑多表草稿`, exact: true }).waitFor({ state: 'hidden' });
    await heading(page, '草稿').waitFor();
    await page.getByText('published mixed label', { exact: true }).waitFor();
    order = await read(applicant, mixedID);
    assert.equal(order.version, savedOrder.version);
    assert.equal(order.items.length, 2);
    assert.equal(order.items[0].content.label, 'published mixed label');
    assert.equal(order.items[1].id, '2');

    await page.getByRole('link', { name: '添加明细', exact: true }).click();
    await button(page, '新增记录').click();
    for (const [field, value] of Object.entries({ code: 'ui-added', label: 'added in mixed order' })) {
      await page.getByLabel(`包含 ${field}`, { exact: true }).check();
      await page.getByLabel(`${field} 值`, { exact: true }).fill(value);
    }
    await button(page, '查看 Change Set').click();
    assert.equal(await page.getByLabel('保存到草稿', { exact: true }).inputValue(), mixedID);
    await button(page, '确认并保存草稿').click();
    await page.waitForURL(`**/configuration/release-orders/${mixedID}`);
    await heading(page, '草稿').waitFor();
    order = await read(applicant, mixedID);
    assert.deepEqual(order.items.map(item => item.operation), ['MODIFY', 'DELETE', 'ADD']);
    const list = await api(applicant, 'GET', `/api/v1/release-orders?table_name=${table}&applicant_id=${applicantID}`);
    assert.equal(list.orders.length, 1, 'appending must not create another order');
    check('detail editor removes a selected later item and edits a retained item; a new ADD appends to the same mixed draft');

    await screenshot(page, 'batch-mixed-draft-desktop.png');
    await submit(page, 3);
    assert.equal(await button(page, '批准发布单').count(), 0);
    await approve(review, page.url(), 3);
    await page.reload();
    await heading(page, '已批准').waitFor();
    const writes = [];
    applicant.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith(`/${mixedID}/execute`)) {
        writes.push({ body: request.postData(), key: request.headers()['idempotency-key'] });
      }
    });
    let committed;
    const executeRoute = `**/api/v1/release-orders/${mixedID}/execute`;
    await page.route(executeRoute, async route => {
      const response = await route.fetch();
      assert.equal(response.status(), 200, await response.text());
      committed = await response.json();
      await route.abort('failed');
    });
    await button(page, '执行发布').click();
    await page.getByText('全部 3 项将一起发布，预览分页不改变操作范围。', { exact: true }).waitFor();
    await button(page, '确认发布到数据库').click();
    await page.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();
    assert.equal(committed.state, 'SUCCEEDED');
    await page.unroute(executeRoute);
    page.once('dialog', dialog => dialog.accept());
    await page.reload();
    const executeReplay=page.waitForResponse(response=>response.request().method()==='POST'&&response.url().endsWith(`/${mixedID}/execute`));
    await repeatReleaseAction(page,'执行发布','确认发布到数据库');
    assert.equal((await executeReplay).status(),200);
    await heading(page, '已发布待完结').waitFor();
    assert.equal(writes.length, 2);
    assert.deepEqual(writes[0], writes[1]);
    order = await read(applicant, mixedID);
    assert.deepEqual(executionCommands(order), executionCommands(committed));
    assert.equal(executionCommands(order).length, 3);
    assert.equal(order.history.filter(event => event.action === 'EXECUTE').length, 1);
    const mixedRows = await query(applicant);
    assert.equal(mixedRows.page.total_count, 3);
    assert.equal(mixedRows.rows.find(row => row.id === '1').label, 'published mixed label');
    assert.equal(mixedRows.rows.some(row => row.id === '2'), false);
    assert.equal(mixedRows.rows.find(row => row.id === '3').label, 'original three');
    assert.equal(mixedRows.rows.filter(row => row.code === 'ui-added').length, 1);
    assert.equal(executionCommands(order)[2].id, mixedRows.rows.find(row => row.code === 'ui-added').id);
    check('independent APPROVER approves all mixed items; EDITOR/PUBLISHER recovers a genuinely committed lost execute response after refresh with the original key and no duplicate rows/history');

    const large = await api(applicant, 'POST', '/api/v1/release-orders', {
      title: '浏览器千条批量变更',
      items: Array.from({ length: 1000 }, (_, index) => ({ table_name:table,operation: 'ADD', content: { code: `large-${index + 1}`, label: `batch item ${index + 1}` } })),
    }, 201);
    await page.goto(`${base}/configuration/release-orders/${large.id}`);
    await heading(page, '草稿', '浏览器千条批量变更').waitFor();
    await page.getByRole('navigation', { name: '明细分页', exact: true }).getByText('共 1,000 项，当前展示 1–20 项', { exact: true }).waitFor();
    await page.getByLabel('定位明细', { exact: true }).fill('1000');
    const finalItem = page.getByRole('region', { name: '明细 1000', exact: true });
    await finalItem.getByText('batch item 1000', { exact: true }).waitFor();
    await finalItem.scrollIntoViewIfNeeded();
    await screenshot(page, 'batch-1000-final-item-desktop.png');
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByLabel('主导航', { exact: true }).waitFor({ state: 'hidden' });
    await assertMobileLayout(page, 'batch-1000-detail');
    await finalItem.scrollIntoViewIfNeeded();
    await screenshot(page, 'batch-1000-final-item-mobile.png');
    await page.setViewportSize({ width: 1440, height: 1000 });
    check('1000-item detail shows the complete count and paging reaches the actual final item on desktop and 390px mobile');

    await submit(page, 1000, '浏览器千条批量变更');
    await approve(review, page.url(), 1000, '浏览器千条批量变更');
    await page.reload();
    await heading(page, '已批准', '浏览器千条批量变更').waitFor();
    await button(page, '执行发布').click();
    await page.getByText('全部 1,000 项将一起发布，预览分页不改变操作范围。', { exact: true }).waitFor();
    await button(page, '确认发布到数据库').click();
    await heading(page, '已发布待完结', '浏览器千条批量变更').waitFor();
    const published = await read(applicant, large.id);
    const commands = executionCommands(published);
    assert.equal(commands.length, 1000);
    assert.equal(new Set(commands.map(command => command.id)).size, 1000);
    assert.ok(commands.every(command => command.record_version === '1' && command.operation === 'ADD'));
    assert.equal(published.executions[0].table_versions[table], '2');
    const actualRows = [];
    for (let number = 1; number <= 6; number++) {
      const data = await query(applicant, [], number);
      assert.equal(data.page.total_count, 1003);
      actualRows.push(...data.rows);
    }
    assert.equal(actualRows.length, 1003);
    commands.forEach((command, index) => {
      const row = actualRows.find(candidate => candidate.id === command.id);
      assert.equal(row?.code, `large-${index + 1}`);
      assert.equal(row?.label, `batch item ${index + 1}`);
    });
    await page.getByLabel('定位结果', { exact: true }).fill('1000');
    const result = page.getByRole('region', { name: '发布结果', exact: true });
    const finalResult = result.getByRole('article').filter({ has: page.getByRole('heading', { name: `明细 1000 · ${commands[999].table_name} · ADD · 记录 ${commands[999].id}`, exact: true }) });
    await finalResult.getByText('值：batch item 1000', { exact: true }).waitFor();
    await finalResult.scrollIntoViewIfNeeded();
    await screenshot(page, 'batch-1000-final-result-desktop.png');
    await page.setViewportSize({ width: 390, height: 844 });
    await assertMobileLayout(page, 'batch-1000-results');
    await finalResult.scrollIntoViewIfNeeded();
    await screenshot(page, 'batch-1000-final-result-mobile.png');
    check('UI submission, independent approval and execution publish all 1000 items; all actual IDs match real queried rows and result paging reaches item 1000 on desktop/mobile');
    assert.deepEqual(errors, []);
    if (output) await writeFile(join(output, 'batch-evidence.json'), JSON.stringify({
      checks, mixed_order_id: mixedID, large_order_id: large.id, mixed_items: order.items.length,
      published_items: commands.length, unique_actual_ids: new Set(commands.map(command => command.id)).size,
      first_id: commands[0].id, last_id: commands[999].id, actual_table_rows: actualRows.length,
      original_key_recovery_equal: true, browser_errors: errors,
    }, null, 2));
    console.log(JSON.stringify({ checks }));
  } catch (error) {
    if (page && output) await page.screenshot({ path: join(output, 'batch-failure.png'), fullPage: false }).catch(() => {});
    throw error;
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
