// Real browser -> production Web proxy -> Admin -> unique disposable MySQL.
// route.fetch executes real writes; only delivery of selected responses is changed.
const { chromium } = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
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
  const uncertain = () => page.getByRole('alert', { name: '提交结果尚未确认', exact: true });
  const check = (name, details) => { passed.push(name); evidence.push({ case: name, ...details }); console.log('PASS', name, JSON.stringify(details)); writeFileSync(`${output}/progress.json`, JSON.stringify({ passed, evidence }, null, 2)); };
  const auditCount = (entity = table) => Number(sql(`SELECT COUNT(*) FROM stage2_write_audit WHERE entity=${literal(entity)};`));
  const resetAudit = () => sql('TRUNCATE stage2_write_audit;');
  const rowCount = name => Number(sql(`SELECT COUNT(*) FROM ${table} WHERE name=${literal(name)};`));
  const seed = name => sql(`INSERT INTO ${table}(name,created_by,created_at,updated_by,updated_at) VALUES(${literal(name)},'fixture',NOW(6),'fixture',NOW(6)); SELECT LAST_INSERT_ID();`);
  const writes = () => requests.filter(r => !['GET', 'HEAD'].includes(r.method) && !r.path.endsWith('/query'));
  async function open(path) {
    if (context) await context.close();
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    page = await context.newPage();
    page.setDefaultTimeout(10000);
    page.on('pageerror', error => pageErrors.push(error.message));
    requests = []; faults = [];
    page.on('request', request => {
      const path = new URL(request.url()).pathname;
      if (path.startsWith('/api/')) requests.push({ path, method: request.method(), ...(path.endsWith('/query') ? { query: request.postDataJSON() } : {}) });
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
    sql(`CREATE TABLE stage2_write_audit(id BIGINT AUTO_INCREMENT PRIMARY KEY,entity VARCHAR(80),operation VARCHAR(12));
      CREATE TRIGGER stage2_data_add AFTER INSERT ON ${table} FOR EACH ROW INSERT INTO stage2_write_audit(entity,operation) VALUES('${table}','ADD');
      CREATE TRIGGER stage2_data_modify AFTER UPDATE ON ${table} FOR EACH ROW INSERT INTO stage2_write_audit(entity,operation) VALUES('${table}','MODIFY');
      CREATE TRIGGER stage2_data_delete AFTER DELETE ON ${table} FOR EACH ROW INSERT INTO stage2_write_audit(entity,operation) VALUES('${table}','DELETE');`, true);
    for (const entity of ['rcc_query_policies', 'rcc_mutation_policies', 'rcc_table_policies']) {
      for (const [suffix, event] of [['add', 'INSERT'], ['modify', 'UPDATE'], ['delete', 'DELETE']]) {
        sql(`CREATE TRIGGER stage2_${entity}_${suffix} AFTER ${event} ON ${entity} FOR EACH ROW INSERT INTO stage2_write_audit(entity,operation) VALUES('${entity}','${event}');`, true);
      }
    }
    browser = await chromium.launch({ headless: true });
    for (const kind of ['abort', 'json', 'contract', '503']) {
      const name = `stage2_add_${kind}`;
      await managed(); await addEditor(name); resetAudit();
      await fault(`/api/v1/tables/${table}/rows`, 'POST', kind);
      await button('确认并执行').click();
      await uncertain().waitFor();
      assert.equal(rowCount(name), 1);
      assert.equal(await button('确认并执行').isDisabled(), true);
      assert.equal(await button('返回修改').isDisabled(), true);
      assert.match(await page.getByRole('dialog', { name: 'ADD Change Set', exact: true }).innerText(), new RegExp(name));
      await verifyUnknown(`ADD ${kind}: committed once, draft retained, check is read-only`, table);
      if (kind === 'abort') await page.screenshot({ path: `${output}/add-response-lost.png`, fullPage: true });
    }
    for (const operation of ['MODIFY', 'DELETE']) {
      const name = `stage2_${operation.toLowerCase()}_lost`;
      const id = seed(name);
      await managed();
      if (operation === 'MODIFY') {
        await button(`修改记录 ${id}`).click();
        await page.getByRole('checkbox', { name: '包含 name', exact: true }).check();
        await page.getByRole('textbox', { name: 'name 值', exact: true }).fill(`${name}_saved`);
        await button('查看 Change Set').click();
      } else await button(`删除记录 ${id}`).click();
      resetAudit();
      await fault(`/api/v1/tables/${table}/rows/${id}`, operation === 'MODIFY' ? 'PATCH' : 'DELETE');
      await button('确认并执行').click(); await uncertain().waitFor();
      assert.equal(sql(`SELECT COUNT(*) FROM ${table} WHERE id=${id};`), operation === 'DELETE' ? '0' : '1');
      if (operation === 'MODIFY') assert.equal(rowCount(`${name}_saved`), 1);
      else assert.doesNotMatch(await page.getByRole('dialog').innerText(), /尚未执行删除|取消删除会/);
      assert.equal(await button('确认并执行').isDisabled(), true);
      await verifyUnknown(`${operation} response lost: actual final row and exact-id check`, table);
      const query = requests.filter(r => r.path.endsWith('/query')).at(-1).query;
      assert.deepEqual(query.conditions, [{ field: 'id', operator: 'exact', value: id }]);
    }
    // A second unknown attempt must not inherit the first attempt's read evidence.
    sql(`INSERT INTO rcc_query_policies(code,name,description,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status,creator,modifier) VALUES('stage2_limited_query_v1','Limited recovery','','page_query','id','DESC',5,5,'ACTIVE','fixture','fixture');
      UPDATE rcc_table_policies SET query_policy_code='stage2_limited_query_v1' WHERE table_name='${table}';`);
    await managed(); await addEditor('stage2_attempt_first'); resetAudit();
    await fault(`/api/v1/tables/${table}/rows`, 'POST');
    await button('确认并执行').click(); await uncertain().waitFor();
    await button('只读核对当前状态').click();
    await page.getByText('当前查询结果（仅供核对）', { exact: true }).waitFor();
    assert.equal(requests.filter(r => r.path.endsWith('/query')).at(-1).query.page_size, undefined);
    assert.ok(await page.locator('.write-recovery-snapshot tbody tr').count() <= 5);
    assert.equal(await button('确认并执行').isDisabled(), true);
    await button('我已核对，返回修改').click();
    assert.equal(await page.getByRole('textbox', { name: 'name 值', exact: true }).inputValue(), 'stage2_attempt_first');
    assert.equal(await page.getByRole('checkbox', { name: '包含 name', exact: true }).isChecked(), true);
    assert.equal(writes().length, 1); assert.equal(auditCount(), 1);
    await page.getByRole('textbox', { name: 'name 值', exact: true }).fill('stage2_attempt_second');
    await button('查看 Change Set').click(); await button('确认并执行').click(); await uncertain().waitFor();
    assert.equal(rowCount('stage2_attempt_first'), 1); assert.equal(rowCount('stage2_attempt_second'), 1);
    assert.equal(writes().length, 2); assert.equal(auditCount(), 2);
    assert.equal(await button('我已核对，返回修改').count(), 0);
    assert.equal(await page.getByText('当前查询结果（仅供核对）', { exact: true }).count(), 0);
    assert.equal(await button('确认并执行').isDisabled(), true);
    await button('只读核对当前状态').click();
    await page.getByText('当前查询结果（仅供核对）', { exact: true }).waitFor();
    assert.equal(writes().length, 2); assert.equal(auditCount(), 2);
    check('explicit resume retains draft; the next unknown attempt needs a fresh read under max page size 5', { requests: writes(), actualWrites: auditCount(), finalRows: [rowCount('stage2_attempt_first'), rowCount('stage2_attempt_second')] });
    sql(`UPDATE rcc_table_policies SET query_policy_code='notification_page_query_v1' WHERE table_name='${table}';`);

    await managed(); await addEditor('stage2_readback_recovery'); resetAudit();
    let failReadback = true;
    await page.route(`**/api/v1/tables/${table}/query`, async route => {
      const query = route.request().postDataJSON();
      if (failReadback && query.conditions?.some(c => c.field === 'id')) return route.fulfill({ status: 503, json: { error: { code: 'query_unavailable', message: 'readback failed', request_id: 'stage2-readback' } } });
      return route.continue();
    });
    await button('确认并执行').click();
    await page.getByRole('heading', { name: 'ADD 已执行，回查未完成', exact: true }).waitFor();
    assert.equal(rowCount('stage2_readback_recovery'), 1); assert.equal(auditCount(), 1);
    const beforeRetry = writes().length;
    failReadback = false; await button('重新回查').click();
    await page.getByRole('heading', { name: 'ADD 已完成', exact: true }).waitFor();
    assert.match(await page.getByRole('dialog', { name: 'ADD 写入结果', exact: true }).innerText(), /stage2_readback_recovery/);
    assert.equal(writes().length, beforeRetry); assert.equal(auditCount(), 1);
    check('known success followed by readback failure recovers without a second write', { writes: writes(), actualWrites: auditCount() });

    await managed(); await addEditor('stage2_double_confirm'); resetAudit();
    let receivedResolve; const received = new Promise(resolve => { receivedResolve = resolve; });
    const gate = new Promise(resolve => { release = resolve; });
    await page.route(`**/api/v1/tables/${table}/rows`, async route => {
      const response = await route.fetch(); receivedResolve(); await gate; await route.fulfill({ response });
    });
    await button('确认并执行').dblclick(); await received;
    await page.keyboard.press('Enter'); await page.keyboard.press('Enter');
    await page.goBack();
    await page.getByRole('alertdialog', { name: '正在提交，请稍候', exact: true }).waitFor();
    const closeEvent = page.waitForEvent('dialog').catch(error => ({ error }));
    await page.close({ runBeforeUnload: true }); const native = await closeEvent; assert.ok(!native.error, native.error?.message); assert.equal(native.type(), 'beforeunload'); await native.dismiss();
    assert.equal(writes().length, 1); assert.equal(auditCount(), 1);
    release(); release = null;
    await page.getByRole('heading', { name: 'ADD 已完成', exact: true }).waitFor();
    assert.equal(rowCount('stage2_double_confirm'), 1);
    check('double-click + Enter + back/close during pending executes one ADD', { writes: writes(), actualWrites: auditCount() });

    await managed(); await addEditor('stage2_validation', 'invalid-state'); resetAudit();
    await button('确认并执行').click();
    await page.getByRole('alert').filter({ hasText: '写入内容不符合实时字段 Schema' }).waitFor();
    assert.equal(rowCount('stage2_validation'), 0); assert.equal(auditCount(), 0);
    assert.equal(await button('确认并执行').isDisabled(), false);
    await button('返回修改').click();
    assert.equal(await page.getByRole('textbox', { name: 'name 值', exact: true }).inputValue(), 'stage2_validation');
    await page.getByRole('textbox', { name: 'state 值', exact: true }).fill('active');
    await button('查看 Change Set').click(); await button('确认并执行').click();
    await page.getByRole('heading', { name: 'ADD 已完成', exact: true }).waitFor();
    assert.equal(rowCount('stage2_validation'), 1); assert.equal(auditCount(), 1); assert.equal(writes().length, 2);
    check('real ENUM rejection keeps input editable and corrected submission writes once', { requests: writes(), actualWrites: auditCount() });

    await managed(); await addEditor('stage2_database_down'); resetAudit();
    const databaseEndpointBefore = docker('port', container, '3306/tcp');
    stopped = true; docker('stop', '--time', '1', container);
    await button('确认并执行').click(); await uncertain().waitFor({ timeout: 40000 });
    assert.equal(await button('确认并执行').isDisabled(), true);
    docker('start', container); await waitDatabase(); stopped = false;
    assert.equal(docker('port', container, '3306/tcp'), databaseEndpointBefore, 'database recovery must retain the Admin-configured TCP endpoint');
    assert.equal(rowCount('stage2_database_down'), 0);
    const recoveryReads = [];
    for (let attempt = 0; attempt < 6; attempt++) {
      const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === `/api/v1/tables/${table}/query`);
      await button('只读核对当前状态').click();
      const response = await responsePromise;
      recoveryReads.push(response.status());
      if (response.ok()) break;
      await page.getByRole('alert').filter({ hasText: 'Managed Table 查询暂时不可用' }).last().waitFor();
      assert.equal(await button('我已核对，返回修改').count(), 0);
      assert.equal(writes().length, 1); assert.equal(auditCount(), 0);
    }
    await page.getByText('当前查询结果（仅供核对）', { exact: true }).waitFor();
    assert.equal(recoveryReads.at(-1), 200);
    assert.equal(writes().length, 1); assert.equal(auditCount(), 0);
    check('actual database outage and recovery: failed reads stay locked, only reads are retried', { writes: writes(), actualWrites: auditCount(), recoveryReadStatuses: recoveryReads, finalRowCount: rowCount('stage2_database_down') });
    assert.equal(await button('确认并执行').isDisabled(), true);

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
            assert.equal(await button(label).isDisabled(), false);
            assert.equal(await page.getByRole('textbox', { name: '显示名称', exact: true }).inputValue(), `Saved ${code}`);
            assert.equal(writes().length, 1);
          }
        } else {
          const label = { activate: '激活', deprecate: '弃用', delete: '删除' }[operation];
          await page.getByRole('dialog').getByRole('button', { name: label, exact: true }).click();
          resetAudit(); await fault(path, method);
          await button(`确认${label}`).click(); await uncertain().waitFor();
          assert.equal(await button(`确认${label}`).isDisabled(), true);
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
      await button(label).click(); await uncertain().waitFor();
      assert.equal(sql(`SELECT ${['enable', 'disable'].includes(operation) ? 'enabled' : 'query_policy_code'} FROM rcc_table_policies WHERE table_name='${assignmentTable}';`), ['enable', 'disable'].includes(operation) ? operation === 'enable' ? '1' : '0' : 'stage1_query_v1');
      await verifyUnknown(`table assignment ${operation}: persisted change and read-only recovery`, 'rcc_table_policies');
      await button(['create', 'replace'].includes(operation) ? '我已核对，返回修改' : '我已核对，结束本次核对').click();
      if (['enable', 'disable'].includes(operation)) {
        await page.waitForURL(`${base}/platform/table-policies`);
        await page.getByRole('dialog').waitFor({ state: 'detached' });
        await page.getByRole('region', { name: '表规则目录', exact: true }).getByRole('row').filter({ hasText: assignmentTable }).getByText(operation === 'enable' ? '已启用' : '未启用', { exact: true }).waitFor();
      }
      assert.equal(await uncertain().count(), 0); assert.equal(writes().length, 1);
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
    for (const trigger of ['stage2_data_add','stage2_data_modify','stage2_data_delete', ...['rcc_query_policies','rcc_mutation_policies','rcc_table_policies'].flatMap(entity => ['add','modify','delete'].map(suffix => `stage2_${entity}_${suffix}`))]) sql(`DROP TRIGGER IF EXISTS ${trigger};`);
    sql(`UPDATE rcc_table_policies SET query_policy_code='notification_page_query_v1' WHERE table_name='${table}'; DELETE FROM ${table} WHERE name LIKE 'stage2_%'; DELETE FROM rcc_table_policies WHERE table_name='stage2_assignment_items'; DELETE FROM rcc_query_policies WHERE code LIKE 'stage2_%'; DELETE FROM rcc_mutation_policies WHERE code LIKE 'stage2_%'; DROP TABLE IF EXISTS stage2_assignment_items; DROP TABLE IF EXISTS stage2_write_audit;`);
    await fs.writeFile(`${output}/result.json`, JSON.stringify({ ok: !failure, passed, evidence, routeErrors, pageErrors, failure }, null, 2));
  }
  if (failure) { console.error(JSON.stringify(failure, null, 2)); process.exitCode = 1; }
})();
