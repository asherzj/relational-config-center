const {readAllReleaseDetailPages,executionCommands,applicationItems}=require('./release-detail-pages.cjs');
const {repeatReleaseAction}=require('./release-original-action.cjs');
// Real browser -> production Web proxy -> Admin -> unique disposable MySQL.
// route.fetch executes real writes; only delivery of selected responses is changed.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const { randomUUID } = require('node:crypto');
const assert = require('node:assert/strict');
const { registerFixtureAccount, authenticatedRequest, selectedBrowser, browserOptions } = require('./local-account.cjs');
const fs = require('node:fs/promises');
const { writeFileSync } = require('node:fs');
const { execFileSync } = require('node:child_process');
const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const table = process.env.RCC_E2E_TABLE;
const container = process.env.RCC_E2E_MYSQL_CONTAINER;
assert.match(container || '', /^rcc-browser-\d+-\d+-mysql$/);
assert.equal(table, 'stage1_acceptance_items');
const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', timeout: 30000 }).trim();
const sql = (statement, root = false) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c',
  root ? 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --batch --skip-column-names -uroot "$MYSQL_DATABASE"' : 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE"'],
  { input: statement, encoding: 'utf8', timeout: 15000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
const literal = (value) => `'${String(value).replaceAll("'", "''")}'`;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
async function waitDatabase() {
  for (let attempt = 0; attempt < 60; attempt++) {
    try { if (sql('SELECT 1;') === '1') return; } catch {}
    await sleep(500);
  }
  throw new Error('disposable MySQL did not recover');
}
(async () => {
  await fs.mkdir(output, { recursive: true });
  const passed = []; const evidence = []; const routeErrors = []; const pageErrors = [];
  let browser; let context; let page; let failure; let stopped = false; let release;
  let requests = []; let faults = [];
  const button = name => page.getByRole('button', { name, exact: true });
  const waitRelease = async (state, title = `${table} recovery change`) => {
    await page.getByRole('heading', { name: title, exact: true }).waitFor();
    await page.getByText(`${table} · ${state}`, { exact: true }).waitFor();
  };
  const uncertain = () => page.getByRole('alert', { name: '提交结果尚未确认', exact: true });
  const check = (name, details) => { passed.push(name); evidence.push({ case: name, ...details }); console.log('PASS', name, JSON.stringify(details)); writeFileSync(`${output}/progress.json`, JSON.stringify({ passed, evidence }, null, 2)); };
  const auditCount = (entity = table) => Number(sql(`SELECT COUNT(*) FROM stage2_write_audit WHERE entity=${literal(entity)};`));
  const resetAudit = () => sql('TRUNCATE stage2_write_audit;');
  const rowCount = name => Number(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(name)};`));
  const seed = name => sql(`INSERT INTO ${table}(name,created_by,created_at,updated_by,updated_at) VALUES(${literal(name)},'fixture',NOW(6),'fixture',NOW(6)); SELECT LAST_INSERT_ID();`);
  const writes = () => requests.filter(r => !['GET', 'HEAD'].includes(r.method) && !r.path.endsWith('/query'));
  async function open(path) {
    if (page) await page.close();
    page = await context.newPage();
    page.setDefaultTimeout(10000);
    page.on('pageerror', error => pageErrors.push(error.message));
    requests = []; faults = [];
    page.on('request', request => {
      const path = new URL(request.url()).pathname;
      if (path.startsWith('/api/') && !path.startsWith('/api/v1/auth/')) requests.push({ path, method: request.method(), ...(path.endsWith('/query') ? { query: request.postDataJSON() } : {}), ...((path.endsWith('/execute') || path.startsWith('/api/v1/table-policies')) ? { body: request.postData(), key: request.headers()['idempotency-key'] } : {}) });
    });
    await page.goto(`${base}${path}`);
  }
  async function managed() {
    await open('/platform/query-policies');
    await page.getByRole('link', { name: '配置内容管理', exact: true }).click();
    await page.getByRole('combobox', { name: 'Managed Table', exact: true }).selectOption(table);
    await button('新增记录').waitFor({ state: 'visible' });
  }
  async function addEditor(name, state) {
    await button('新增记录').click();
    await page.getByRole('checkbox', { name: '包含 name', exact: true }).check();
    await page.getByRole('textbox', { name: 'name 值', exact: true }).fill(name);
    if (state) {
      await page.getByRole('checkbox', { name: '包含 state', exact: true }).check();
      await page.getByRole('textbox', { name: 'state 值', exact: true }).fill(state);
    }
    await button('查看 Change Set').click();
  }
  async function fault(path, method, kind = 'abort') {
    await page.route(`**${path}`, async route => {
      if (route.request().method() !== method) return route.continue();
      try {
        const response = await route.fetch();
        faults.push({ path, method, actualStatus: response.status(), requestId: response.headers()['x-request-id'] });
        assert.ok(response.ok(), `real write unexpectedly rejected: ${response.status()}`);
        if (kind === 'abort') return await route.abort('failed');
        if (kind === 'json') return await route.fulfill({ response, body: '{broken' });
        if (kind === 'contract') return await route.fulfill({ response, json: { id: 9007199254740992 } });
        if (kind === '503') return await route.fulfill({ status: 503, json: { error: { code: 'mutation_unavailable', message: 'Unavailable', request_id: response.headers()['x-request-id'] } } });
        if (kind === 'catalog503') return await route.fulfill({ status: 503, json: { error: { code: 'policy_catalog_unavailable', message: 'Unavailable', request_id: response.headers()['x-request-id'] } } });
      } catch (error) { routeErrors.push(error.message); await route.abort().catch(() => {}); }
    });
  }
  async function verifyUnknown(label, entity, expectedAudit = 1) {
    await uncertain().waitFor();
    const before = writes().length;
    assert.equal(before, 1); assert.equal(auditCount(entity), expectedAudit);
    await button('只读核对当前状态').click();
    await page.getByText('当前查询结果（仅供核对）', { exact: true }).waitFor();
    assert.equal(writes().length, before); assert.equal(auditCount(entity), expectedAudit);
    assert.equal(await uncertain().count(), 1);
    assert.doesNotMatch(await uncertain().innerText(), /请稍后重试|检查服务状态后重试/);
    check(label, { requests: writes(), actualWrites: auditCount(entity), faults, readonlyRequests: requests.filter(r => r.method === 'GET' || r.path.endsWith('/query')).slice(-1) });
  }
  try {
    // Audit catalog mutations only. Business-table triggers are deliberately
    // incompatible with the publication contract; its immutable Command and
    // execution event provide the authoritative once-only write evidence.
    sql(`CREATE TABLE stage2_write_audit(id BIGINT AUTO_INCREMENT PRIMARY KEY,entity VARCHAR(80),operation VARCHAR(12));`, true);
    for (const entity of ['rcc_query_policies', 'rcc_mutation_policies', 'rcc_table_policies']) {
      for (const [suffix, event] of [['add', 'INSERT'], ['modify', 'UPDATE'], ['delete', 'DELETE']]) {
        sql(`CREATE TRIGGER stage2_${entity}_${suffix} AFTER ${event} ON ${entity} FOR EACH ROW INSERT INTO stage2_write_audit(entity,operation) VALUES('${entity}','${event}');`, true);
      }
    }
    browser = await selectedBrowser(playwright).launch(browserOptions());
    const account = async roles => {
      const candidate = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
      await registerFixtureAccount(candidate, base, { roles });
      return candidate;
    };
    const admin = await account(['ADMIN']);
    const applicant = await account(['EDITOR']);
    const reviewer = await account(['APPROVER']);
    const publisher = await account(['PUBLISHER']);
    const api = async (actor, method, path, data, expected = 200) => {
      const response = await authenticatedRequest(actor, base, path, {
        method, headers: { 'Idempotency-Key': randomUUID() }, ...(data === undefined ? {} : { data }),
      });
      assert.equal(response.status(), expected, `${method} ${path}: ${await response.text()}`);
      return response.json();
    };
    const read = async id => readAllReleaseDetailPages(publisher,base,await api(publisher,'GET',`/api/v1/release-orders/${id}`));
    const approve = async order => {
      const submitted = await api(applicant, 'POST', `/api/v1/release-orders/${order.id}/submit`, { expected_version: order.version });
      return api(reviewer, 'POST', `/api/v1/release-orders/${order.id}/approve`, { expected_version: submitted.version, reason: 'Independent recovery acceptance' });
    };
    const prepare = async (operation, name) => {
      const id = operation === 'ADD' ? undefined : seed(name);
      const item = { operation, content: operation === 'DELETE' ? {} : { name: operation === 'MODIFY' ? `${name}_saved` : name }, ...(id ? { id, expected_record_version: '0' } : {}) };
      const draft = await api(applicant, 'POST', '/api/v1/release-orders', { title: `${table} recovery change`, items:[{...item,table_name:table}] }, 201);
      const order = await approve(draft);
      context = publisher;
      // Enter through a real in-app history entry so pending Back exercises
      // the router blocker, not a fresh tab's about:blank document.
      await open('/configuration/release-orders');
      await page.getByRole('row').filter({ hasText: order.id }).getByRole('link', { name: order.title, exact: true }).click();
      await button('执行发布').click();
      return { order, id, path: `/api/v1/release-orders/${order.id}/execute` };
    };
    const executeRequests = path => writes().filter(request => request.path === path);
    const commands = id => Number(sql(`SELECT COUNT(*) FROM rcc_publication_commands WHERE order_id=${literal(id)};`));
    const published = async (order, name, operation = 'ADD') => {
      const current = await read(order.id);
      assert.equal(current.state, 'SUCCEEDED');
      assert.equal(current.history.filter(event => event.action === 'EXECUTE').length, 1);
      assert.equal(commands(order.id), 1);
      assert.equal(executionCommands(current).length, 1);
      assert.equal(rowCount(operation === 'MODIFY' ? `${name}_saved` : name), operation === 'DELETE' ? 0 : 1);
      return current;
    };
    const recover = async (order, path, reload = false) => {
      await page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。',{exact:true}).waitFor();
      const first = executeRequests(path);
      assert.equal(first.length, 1);
      assert.ok(first[0].key);
      const current = await read(order.id);
      assert.equal(current.state, 'SUCCEEDED');
      // A read can establish current state but cannot acknowledge the pending
      // request. Only the original immutable request is allowed to resolve it.
      assert.equal(executeRequests(path).length, 1);
      assert.equal(await button('确认发布到数据库').count(), 1);
      await page.unroute(`**${path}`);
      const repeated=page.waitForResponse(response=>response.request().method()==='POST'&&new URL(response.url()).pathname===path);
      if (reload) {
        page.once('dialog', dialog => dialog.accept());
        await page.reload();
        await repeatReleaseAction(page,'执行发布','确认发布到数据库');
      } else await button('确认发布到数据库').click();
      assert.equal((await repeated).status(),200);
      await page.getByRole('dialog',{name:'执行发布',exact:true}).waitFor({state:'detached'});
      await waitRelease('已发布待完结');
      const attempts = executeRequests(path);
      assert.equal(attempts.length, 2);
      assert.deepEqual(attempts[0], attempts[1]);
      assert.equal(commands(order.id), 1);
      assert.equal(await button('恢复原发布请求').count(), 0);
      return attempts;
    };
    for (const kind of ['abort', 'json', 'contract', '503']) {
      const name = `stage2_add_${kind}`;
      const { order, path } = await prepare('ADD', name);
      await fault(path, 'POST', kind);
      await button('确认发布到数据库').click();
      await page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。',{exact:true}).waitFor();
      await published(order, name);
      assert.match(await page.getByRole('dialog', { name: '执行发布', exact: true }).innerText(), new RegExp(name));
      if (kind === 'abort') await page.screenshot({ path: `${output}/add-response-lost.png`, fullPage: true });
      const attempts = await recover(order, path, kind === 'abort');
      await published(order, name);
      check(`ADD ${kind}: one publication, immutable intent, original-key recovery`, { attempts, commands: commands(order.id), faults });
    }
    for (const operation of ['MODIFY', 'DELETE']) {
      const name = `stage2_${operation.toLowerCase()}_lost`;
      const { order, path, id } = await prepare(operation, name);
      await fault(path, 'POST');
      await button('确认发布到数据库').click();
      await page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。',{exact:true}).waitFor();
      await published(order, name, operation);
      const attempts = await recover(order, path);
      const actual = await published(order, name, operation);
      assert.equal(actual.items[0].id, id);
      check(`${operation} lost response: exact identity and original-key replay`, { attempts, id, commands: commands(order.id), faults });
    }
    // A second order has its own journal entry; resolving the first cannot
    // acknowledge another request or replace its idempotency key.
    const recoveredKeys = [];
    for (const suffix of ['first', 'second']) {
      const name = `stage2_attempt_${suffix}`;
      const { order, path } = await prepare('ADD', name);
      await fault(path, 'POST');
      await button('确认发布到数据库').click();
      await page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。',{exact:true}).waitFor();
      const attempts = await recover(order, path);
      recoveredKeys.push(attempts[0].key);
      await published(order, name);
    }
    assert.notEqual(recoveredKeys[0], recoveredKeys[1]);
    check('separate unknown publications retain separate original request keys', { recoveredKeys });

    const readbackName = 'stage2_readback_recovery';
    const readback = await prepare('ADD', readbackName);
    let failReadback = true;
    await page.route(`**/api/v1/release-orders/${readback.order.id}`, async route => {
      if (failReadback && route.request().method() === 'GET') return route.fulfill({ status: 503, json: { error: { code: 'release_unavailable', message: 'readback failed', request_id: 'stage2-readback' } } });
      return route.continue();
    });
    await button('确认发布到数据库').click();
    await page.getByRole('alert').filter({ hasText: 'stage2-readback' }).waitFor();
    await published(readback.order, readbackName);
    const beforeRetry = executeRequests(readback.path).length;
    failReadback = false;
    await button('重试').click();
    await waitRelease('已发布待完结');
    assert.equal(executeRequests(readback.path).length, beforeRetry);
    check('known publication followed by detail read failure recovers with GET only', { attempts: executeRequests(readback.path), commands: commands(readback.order.id) });

    const doubleName = 'stage2_double_confirm';
    const double = await prepare('ADD', doubleName);
    let receivedResolve; const received = new Promise(resolve => { receivedResolve = resolve; });
    const gate = new Promise(resolve => { release = resolve; });
    await page.route(`**${double.path}`, async route => {
      const response = await route.fetch(); receivedResolve(); await gate; await route.fulfill({ response });
    });
    await button('确认发布到数据库').dblclick(); await received;
    await page.keyboard.press('Enter'); await page.keyboard.press('Enter');
    await page.goBack();
    await page.getByRole('alertdialog', { name: '正在提交，请稍候', exact: true }).waitFor();
    const closeEvent = page.waitForEvent('dialog');
    await page.close({ runBeforeUnload: true });
    const native = await closeEvent; assert.equal(native.type(), 'beforeunload'); await native.dismiss();
    assert.equal(executeRequests(double.path).length, 1);
    assert.equal(commands(double.order.id), 1);
    release(); release = null;
    await waitRelease('已发布待完结');
    await published(double.order, doubleName);
    check('double-click, Enter and pending back/close execute once', { attempts: executeRequests(double.path), commands: commands(double.order.id) });

    context = applicant;
    await managed(); await addEditor('stage2_validation', 'invalid-state');
    await button('确认并保存草稿').click();
    await waitRelease('草稿', `${table} 配置变更`);
    assert.equal(rowCount('stage2_validation'), 0);
    const invalidDraft = await read(new URL(page.url()).pathname.split('/').at(-1));
    const invalidOrder = await approve(invalidDraft);
    context = publisher; await open(`/configuration/release-orders/${invalidOrder.id}`);
    await button('执行发布').click();
    const [rejection] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/release-orders/${invalidOrder.id}/execute`),
      button('确认发布到数据库').click(),
    ]);
    assert.equal(rejection.status(), 422);
    assert.equal((await rejection.json()).error.code, 'invalid_mutation_content');
    await page.getByRole('alert').filter({ hasText: '写入内容不符合实时字段 Schema' }).waitFor();
    const retained = await read(invalidOrder.id);
    assert.equal(retained.state, 'APPROVED');
    assert.equal(retained.items[0].content.state, 'invalid-state');
    assert.equal(rowCount('stage2_validation'), 0);
    assert.equal(commands(invalidOrder.id), 0);
    // An approved intention is immutable after a constraint failure. Correct a
    // copied draft and obtain new approval; do not edit the frozen original.
    context = applicant; await open(`/configuration/release-orders/${invalidOrder.id}`);
    await button('取消发布单').click();
    await page.getByRole('textbox', { name: '取消原因', exact: true }).fill('Correct the rejected ENUM value in a new proposal');
    await button('确认取消发布单').click();
    await waitRelease('已取消', `${table} 配置变更`);
    await button('复制新草稿').click();
    await button('读取最新配置').click();
    await button('确认最新基线并复制').click();
    await waitRelease('草稿', `${table} 配置变更`);
    const correctedID = new URL(page.url()).pathname.split('/').at(-1);
    assert.notEqual(correctedID, invalidOrder.id);
    await button('编辑草稿').click();
    assert.equal(await page.getByRole('textbox', { name: 'name 申请值', exact: true }).inputValue(), 'stage2_validation');
    assert.equal(await page.getByRole('textbox', { name: 'state 申请值', exact: true }).inputValue(), 'invalid-state');
    await page.getByRole('textbox', { name: 'state 申请值', exact: true }).fill('active');
    await button('保存草稿修改').click();
    await page.getByRole('dialog', { name: `编辑多表草稿`, exact: true }).waitFor({ state: 'detached' });
    const corrected = await read(correctedID);
    assert.equal(corrected.items[0].content.state, 'active');
    assert.equal(corrected.copied_from_id, invalidOrder.id);
    assert.equal(rowCount('stage2_validation'), 0);
    const validOrder = await approve(corrected);
    context = publisher; await open(`/configuration/release-orders/${validOrder.id}`);
    await button('执行发布').click(); await button('确认发布到数据库').click();
    await waitRelease('已发布待完结', `${table} 配置变更`);
    await published(validOrder, 'stage2_validation');
    assert.equal((await read(invalidOrder.id)).state, 'CANCELLED');
    assert.equal(commands(invalidOrder.id), 0);
    check('real ENUM failure preserves frozen intent; copied correction needs new approval and publishes once', { rejectedOrderID: invalidOrder.id, correctedOrderID: validOrder.id, commands: commands(validOrder.id) });

    const outageName = 'stage2_database_down';
    const outage = await prepare('ADD', outageName);
    const databaseEndpointBefore = docker('port', container, '3306/tcp');
    stopped = true; docker('stop', '--time', '1', container);
    await button('确认发布到数据库').click();
    await page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。',{exact:true}).waitFor({state:'attached',timeout:40000});
    const original = executeRequests(outage.path)[0];
    docker('start', container); await waitDatabase(); stopped = false;
    assert.equal(docker('port', container, '3306/tcp'), databaseEndpointBefore);
    assert.equal(rowCount(outageName), 0);
    const accountRecoveryStatuses = [];
    for (let attempt = 0; attempt < 6 && await page.locator('.session-interruption').count(); attempt++) {
      await button('重新检查登录状态').waitFor();
      const [response] = await Promise.all([
        page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/auth/session'),
        button('重新检查登录状态').click(),
      ]);
      accountRecoveryStatuses.push(response.status());
      await page.waitForFunction(() => {
        const overlay = document.querySelector('.session-interruption');
        return !overlay || Boolean(overlay.querySelector('button'));
      });
    }
    const waitForAccount = async () => {
      for (let attempt = 0; attempt < 12; attempt++) {
        const response = await publisher.request.get(`${base}/api/v1/auth/session`);
        accountRecoveryStatuses.push(response.status());
        if (response.status() === 200) return;
        assert.ok([503, 504].includes(response.status()), `unexpected account recovery status: ${response.status()}`);
        await sleep(300);
      }
      throw new Error('Admin authentication did not recover after disposable MySQL restart');
    };
    // MySQL accepting a new CLI connection is not proof that Admin's pooled
    // connections have recovered. Transient authentication failures happen
    // before release idempotency storage; keep the same request throughout.
    const replayStatuses = [];
    for (let attempt = 0; attempt < 6; attempt++) {
      await waitForAccount();
      const [response] = await Promise.all([
        page.waitForResponse(response => new URL(response.url()).pathname === outage.path),
        button('确认发布到数据库').click(),
      ]);
      replayStatuses.push(response.status());
      if (response.status() === 200) break;
      const payload = await response.json();
      assert.ok([503, 504].includes(response.status()), `unexpected original-key recovery status: ${response.status()}`);
      assert.ok(['auth_unavailable', 'auth_timeout', 'release_unavailable', 'release_result_unknown', 'request_timeout'].includes(payload.error.code), payload.error.code);
      await page.getByText('原请求与意见已保留；再次点击同一操作将提交原请求。',{exact:true}).waitFor();
      await sleep(300);
    }
    assert.equal(replayStatuses.at(-1), 200);
    await waitRelease('已发布待完结');
    const outageAttempts = executeRequests(outage.path);
    assert.ok(outageAttempts.length >= 2);
    for (const attempt of outageAttempts) assert.deepEqual(attempt, original);
    await published(outage.order, outageName);
    check('real database stop/start retains endpoint and original request, publishes once after recovery', { attempts: outageAttempts, commands: commands(outage.order.id), accountRecoveryStatuses, replayStatuses });
    context = admin;

    // Catalog scenarios are appended below; each uses a fresh browser document and audit baseline.
    for (const kind of ['query', 'mutation']) {
      const entity = `rcc_${kind}_policies`;
      for (const operation of ['create', 'replace', 'metadata', 'activate', 'deprecate', 'delete']) {
        const code = `stage2_${kind}_${operation}_v1`;
        if (operation !== 'create') {
          const status = ['metadata', 'deprecate'].includes(operation) ? 'ACTIVE' : 'DRAFT';
          if (kind === 'query') sql(`INSERT INTO ${entity}(code,name,description,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status,creator,modifier) SELECT '${code}','Before recovery','fixture',type_code,default_order_field,default_order_direction,default_page_size,max_page_size,'${status}','fixture','fixture' FROM rcc_query_policies WHERE code='stage1_query_v1';`);
          else sql(`INSERT INTO ${entity}(code,name,description,type_code,allow_add,allow_modify,allow_delete,status,creator,modifier) VALUES('${code}','Before recovery','fixture','single_table_mutation',1,1,1,'${status}','fixture','fixture');`);
        }
        const collection = `/platform/${kind}-policies`;
        const mode = operation === 'replace' ? '?mode=edit' : operation === 'metadata' ? '?mode=metadata' : '';
        await open(`${collection}/${operation === 'create' ? 'new' : code}${mode}`);
        const resource = `/api/v1/${kind}-policies${operation === 'create' ? '' : `/${code}`}`;
        const method = { create: 'POST', replace: 'PUT', metadata: 'PATCH', activate: 'POST', deprecate: 'POST', delete: 'DELETE' }[operation];
        const path = ['metadata', 'activate', 'deprecate'].includes(operation) ? `${resource}/${operation}` : resource;
        if (['create', 'replace', 'metadata'].includes(operation)) {
          if (operation === 'create') await page.getByRole('textbox', { name: /规则编码/ }).fill(code);
          await page.getByRole('textbox', { name: '显示名称', exact: true }).fill(`Saved ${code}`);
          resetAudit();
          await fault(path, method, operation === 'metadata' ? 'catalog503' : 'abort');
          const label = { create: '创建草稿', replace: '保存执行规则', metadata: '保存名称和描述' }[operation];
          await button(label).click(); await uncertain().waitFor();
          assert.equal(await button(label).isDisabled(), true);
          assert.equal(await page.getByRole('textbox', { name: '显示名称', exact: true }).inputValue(), `Saved ${code}`);
          assert.equal(sql(`SELECT name FROM ${entity} WHERE code='${code}';`), `Saved ${code}`);
          await verifyUnknown(`${kind} ${operation}: saved state retained after response failure`, entity);
          if (operation === 'create') {
            await button('我已核对，返回修改').click();
            // Mutation.reset publishes through its observer after this click.
            await button(label).and(page.locator(':enabled')).waitFor({ state: 'attached' });
            assert.equal(await button(label).isDisabled(), false);
            assert.equal(await page.getByRole('textbox', { name: '显示名称', exact: true }).inputValue(), `Saved ${code}`);
            assert.equal(writes().length, 1);
          }
        } else {
          const label = { activate: '激活', deprecate: '弃用', delete: '删除' }[operation];
          await page.getByRole('dialog').getByRole('button', { name: label, exact: true }).click();
          resetAudit(); await fault(path, method);
          await button(`确认${label}`).click(); await uncertain().waitFor();
          // The accepted command is settled before recovery is shown. Its modal
          // and every lifecycle write entry disappear until the read-only check
          // is acknowledged, so the same command cannot be submitted again.
          assert.equal(await button(`确认${label}`).count(), 0);
          assert.equal(await button(label).count(), 0);
          assert.equal(sql(`SELECT ${operation === 'delete' ? 'COUNT(*)' : 'status'} FROM ${entity} WHERE code='${code}';`), operation === 'delete' ? '0' : operation === 'activate' ? 'ACTIVE' : 'DEPRECATED');
          if (operation === 'delete') {
            await button('只读核对当前状态').click();
            await page.getByRole('alert').filter({ hasText: kind === 'query' ? '查询规则不存在或已被移除。' : '变更规则不存在或已被移除。' }).last().waitFor();
            assert.equal(writes().length, 1); assert.equal(auditCount(entity), 1);
            check(`${kind} delete: lost response, missing-resource check never repeats deletion`, { writes: writes(), actualWrites: auditCount(entity), faults });
          } else await verifyUnknown(`${kind} ${operation}: lifecycle response loss is recoverable by reads`, entity);
          await button('我已核对，结束本次核对').click();
          await page.waitForURL(`${base}${collection}`);
          await page.getByRole('dialog').waitFor({ state: 'detached' });
          assert.equal(await page.getByRole('dialog').count(), 0);
          assert.equal(await uncertain().count(), 0); assert.equal(writes().length, 1);
        }
      }
    }

    // Real rapid lifecycle actions: the isolated hook regression proves the same-render
    // race, while these native browser events prove one request/one database update.
    const rapidCode = 'stage2_rapid_lifecycle_v1';
    sql(`INSERT INTO rcc_query_policies(code,name,description,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status,creator,modifier) VALUES('${rapidCode}','Rapid lifecycle','','page_query','id','DESC',5,5,'DRAFT','fixture','fixture');`);
    await open('/platform/query-policies');
    await page.getByRole('row').filter({ hasText: rapidCode }).getByRole('button', { name: '查看', exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: '激活', exact: true }).click(); resetAudit();
    let arrivedResolve; const arrived = new Promise(resolve => { arrivedResolve = resolve; });
    const delayed = new Promise(resolve => { release = resolve; });
    await page.route(`**/api/v1/query-policies/${rapidCode}/activate`, async route => {
      const response = await route.fetch(); arrivedResolve(); await delayed; await route.fulfill({ response });
    });
    await button('确认激活').dblclick(); await arrived;
    await page.keyboard.press('Enter'); await page.keyboard.press('Enter');
    await page.goBack(); await page.getByRole('alertdialog', { name: '正在提交，请稍候', exact: true }).waitFor();
    assert.equal(writes().length, 1); assert.equal(auditCount('rcc_query_policies'), 1);
    release(); release = null;
    await page.getByRole('alertdialog', { name: '激活查询规则？', exact: true }).waitFor({ state: 'detached' });
    check('rapid lifecycle confirmation and pending back navigation update once', { writes: writes(), actualWrites: auditCount('rcc_query_policies') });

    const assignmentTable = 'stage2_assignment_items';
    sql(`CREATE TABLE ${assignmentTable} LIKE ${table};
      INSERT INTO rcc_mutation_policies(code,name,description,type_code,allow_add,allow_modify,allow_delete,create_operator_field,create_time_field,modify_operator_field,modify_time_field,status,creator,modifier)
      SELECT 'stage2_assignment_mutation_v1','Assignment recovery','',type_code,allow_add,allow_modify,allow_delete,create_operator_field,create_time_field,modify_operator_field,modify_time_field,'ACTIVE','fixture','fixture' FROM rcc_mutation_policies WHERE code='stage1_mutation_v1';`);
    for (const operation of ['create', 'replace', 'enable', 'disable']) {
      if (operation !== 'create') sql(`INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES('${assignmentTable}','notification_page_query_v1','stage2_assignment_mutation_v1',${operation === 'disable' ? 1 : 0},'fixture','fixture') ON DUPLICATE KEY UPDATE query_policy_code='notification_page_query_v1',enabled=${operation === 'disable' ? 1 : 0};`);
      await open(operation === 'create' ? '/platform/table-policies?mode=create' : `/platform/table-policies/${assignmentTable}${operation === 'replace' ? '?mode=replace' : ''}`);
      if (operation === 'create') await page.getByRole('combobox', { name: '真实数据库表', exact: true }).selectOption(assignmentTable);
      if (['create', 'replace'].includes(operation)) {
        await page.getByRole('combobox', { name: 'Active 查询规则', exact: true }).selectOption('stage1_query_v1');
        await page.getByRole('combobox', { name: 'Active 变更规则', exact: true }).selectOption('stage2_assignment_mutation_v1');
      } else await page.getByRole('dialog').getByRole('button', { name: operation === 'enable' ? '启用' : '停用', exact: true }).click();
      const path = `/api/v1/table-policies${operation === 'create' ? '' : `/${assignmentTable}`}${['enable', 'disable'].includes(operation) ? `/${operation}` : ''}`;
      resetAudit(); await fault(path, operation === 'replace' ? 'PUT' : 'POST');
      const label = { create: '创建未启用分配', replace: '检查并替换', enable: '确认启用', disable: '确认停用' }[operation];
      await button(label).click(); await page.getByText(/操作结果未知/).waitFor();
      assert.equal(sql(`SELECT ${['enable', 'disable'].includes(operation) ? 'enabled' : 'query_policy_code'} FROM rcc_table_policies WHERE table_name='${assignmentTable}';`), ['enable', 'disable'].includes(operation) ? operation === 'enable' ? '1' : '0' : 'stage1_query_v1');
      assert.equal(writes().length,1);const originalWrite={...writes()[0]};
      await page.unroute(`**${path}`);await button('重推原请求').click();
      await page.getByText({create:'表规则已创建并保持未启用',replace:'表规则已替换',enable:'表规则已启用',disable:'表规则已停用'}[operation],{exact:true}).waitFor();
      assert.equal(writes().length,2);
      assert.equal(writes()[1].key,originalWrite.key);assert.deepEqual(writes()[1].body,originalWrite.body);
      assert.equal(auditCount('rcc_table_policies'),1);
      check(`table assignment ${operation}: original request manually replayed without duplicate database change`, {writes:writes(),originalWrite});
    }
    assert.deepEqual(routeErrors, []);
    assert.deepEqual(pageErrors, []);
  } catch (error) {
    failure = { message: error.message, stack: error.stack, requests, faults };
    if (page && !page.isClosed()) {
      await page.screenshot({ path: `${output}/failure.png`, fullPage: true }).catch(() => {});
      await fs.writeFile(`${output}/failure-body.txt`, await page.locator('body').innerText()).catch(() => {});
    }
  } finally {
    if (release) release();
    if (stopped) { docker('start', container); await waitDatabase(); }
    if (browser) await browser.close();
    for (const trigger of ['rcc_query_policies','rcc_mutation_policies','rcc_table_policies'].flatMap(entity => ['add','modify','delete'].map(suffix => `stage2_${entity}_${suffix}`))) sql(`DROP TRIGGER IF EXISTS ${trigger};`);
    sql(`UPDATE rcc_table_policies SET query_policy_code='notification_page_query_v1' WHERE table_name='${table}'; DELETE FROM ${table} WHERE name LIKE 'stage2_%'; DELETE FROM rcc_table_policies WHERE table_name='stage2_assignment_items'; DELETE FROM rcc_query_policies WHERE code LIKE 'stage2_%'; DELETE FROM rcc_mutation_policies WHERE code LIKE 'stage2_%'; DROP TABLE IF EXISTS stage2_assignment_items; DROP TABLE IF EXISTS stage2_write_audit;`);
    await fs.writeFile(`${output}/result.json`, JSON.stringify({ ok: !failure, passed, evidence, routeErrors, pageErrors, failure }, null, 2));
  }
  if (failure) { console.error(JSON.stringify(failure, null, 2)); process.exitCode = 1; }
})();
