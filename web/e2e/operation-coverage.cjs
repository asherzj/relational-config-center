const { configureFixtureReleaseTemplates } = require('./release-template-fixture.cjs');
const { createFixtureApprovalRole, fixtureApprovalInput } = require('./table-approval-fixture.cjs');
// Real Chromium -> production Web proxy -> Cookie-authenticated Admin -> isolated MySQL.
// SQL is used only to arrange disposable fixtures and to verify browser actions.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { browserOptions, selectedBrowser, registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');
const fs = require('node:fs/promises');
const { execFileSync } = require('node:child_process');

const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const container = process.env.RCC_E2E_MYSQL_CONTAINER;
assert.match(container || '', /^rcc-browser-\d+-\d+-mysql$/);

const tables = ['stage3_active_items', 'stage3_denied_items', 'stage3_candidate_items', 'stage3_catalog_alpha', 'stage3_catalog_beta'];
const literal = (value) => `'${String(value).replaceAll("'", "''")}'`;
const sql = (statement, root = false) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c',
  root ? 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --batch --skip-column-names -uroot "$MYSQL_DATABASE"' : 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE"'],
{ input: statement, encoding: 'utf8', timeout: 20000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();

const fixtureRows = () => sql(`SELECT CONCAT_WS('|',id,HEX(name),IF(category IS NULL,'NULL',CONCAT('HEX:',HEX(category))),IF(note IS NULL,'NULL',CONCAT('HEX:',HEX(note))),HEX(state),priority,HEX(created_by),DATE_FORMAT(created_at,'%Y%m%d%H%i%s.%f'),HEX(updated_by),DATE_FORMAT(updated_at,'%Y%m%d%H%i%s.%f')) FROM stage1_acceptance_items ORDER BY id;`);
const executionFields = (kind, code) => kind === 'query'
  ? sql(`SELECT CONCAT_WS('|',type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status) FROM rcc_query_policies WHERE code=${literal(code)};`)
  : sql(`SELECT CONCAT_WS('|',type_code,allow_add,allow_modify,allow_delete,COALESCE(create_operator_field,'NULL'),COALESCE(create_time_field,'NULL'),COALESCE(modify_operator_field,'NULL'),COALESCE(modify_time_field,'NULL'),status) FROM rcc_mutation_policies WHERE code=${literal(code)};`);

function fixtureSQL() {
  const tableDDL = tables.map((name) => `CREATE TABLE ${name} (
    id bigint unsigned NOT NULL AUTO_INCREMENT,
    name varchar(64) NOT NULL,
    created_by varchar(64) NOT NULL,
    created_at datetime(6) NOT NULL,
    updated_by varchar(64) NOT NULL,
    updated_at datetime(6) NOT NULL,
    PRIMARY KEY (id)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;`).join('\n');
  return `${tableDDL}
    INSERT INTO stage3_active_items(name,created_by,created_at,updated_by,updated_at) VALUES('Stage3Alpha','fixture',NOW(6),'fixture',NOW(6)),('Other','fixture',NOW(6),'fixture',NOW(6));
    INSERT INTO stage3_denied_items(name,created_by,created_at,updated_by,updated_at) VALUES('DeniedSeed','fixture',NOW(6),'fixture',NOW(6));
    INSERT INTO stage3_candidate_items(name,created_by,created_at,updated_by,updated_at) VALUES('CandidateSeed','fixture',NOW(6),'fixture',NOW(6));
    INSERT INTO stage3_catalog_alpha(name,created_by,created_at,updated_by,updated_at) VALUES('CatalogAlpha','fixture',NOW(6),'fixture',NOW(6));
    INSERT INTO stage3_catalog_beta(name,created_by,created_at,updated_by,updated_at) VALUES('CatalogBeta','fixture',NOW(6),'fixture',NOW(6));
    INSERT INTO rcc_query_policies(code,name,description,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status,creator,modifier) VALUES
      ('stage3_query_existing_v1','Stage 3 existing query','existing assignment','page_query','id','DESC',20,200,'ACTIVE','stage3-fixture','stage3-fixture'),
      ('stage3_query_full_v1','Stage 3 full query','new assignment candidate','page_query','id','DESC',20,200,'ACTIVE','stage3-fixture','stage3-fixture'),
      ('stage3_query_draft_v1','Stage 3 draft query','must not be assignable','page_query','id','DESC',20,200,'DRAFT','stage3-fixture','stage3-fixture');
    INSERT INTO rcc_mutation_policies(code,name,description,type_code,allow_add,allow_modify,allow_delete,create_operator_field,create_time_field,modify_operator_field,modify_time_field,status,creator,modifier) VALUES
      ('stage3_mutation_existing_v1','Stage 3 existing mutation','existing assignment','single_table_mutation',1,1,1,'created_by','created_at','updated_by','updated_at','ACTIVE','stage3-fixture','stage3-fixture'),
      ('stage3_mutation_denied_v1','Stage 3 denied mutation','all operations denied','single_table_mutation',0,0,0,NULL,NULL,NULL,NULL,'ACTIVE','stage3-fixture','stage3-fixture'),
      ('stage3_mutation_full_v1','Stage 3 full mutation','new assignment candidate','single_table_mutation',1,1,1,'created_by','created_at','updated_by','updated_at','ACTIVE','stage3-fixture','stage3-fixture'),
      ('stage3_mutation_draft_v1','Stage 3 draft mutation','must not be assignable','single_table_mutation',0,0,0,NULL,NULL,NULL,NULL,'DRAFT','stage3-fixture','stage3-fixture');
    INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES
      ('stage3_active_items','stage3_query_existing_v1','stage3_mutation_existing_v1',1,'stage3-fixture','Stage3ExistingModifier'),
      ('stage3_denied_items','stage3_query_full_v1','stage3_mutation_denied_v1',1,'stage3-fixture','Stage3DeniedModifier'),
      ('stage3_catalog_alpha','stage3_query_full_v1','stage3_mutation_full_v1',0,'stage3-fixture','Stage3FilterPerson'),
      ('stage3_catalog_beta','stage3_query_full_v1','stage3_mutation_full_v1',0,'stage3-fixture','Stage3OtherModifier');`;
}

(async () => {
  await fs.mkdir(output, { recursive: true });
  const before = fixtureRows();
  const passed = [];
  const evidence = [];
  const http = [];
  const pageErrors = [];
  let browser;
  let context;
  let approvalContext;
  let account;
  let browserVersion = null;
  let page;
  let failure = null;
  let cleanup = null;
  let sequence = 0;
  const check = (name, details = {}) => {
    passed.push(name);
    evidence.push({ case: name, ...details });
    console.log('PASS', name, JSON.stringify(details));
  };
  const responseFor = (method, pathname) => http.filter((entry) => entry.method === method && entry.path === pathname).at(-1);
  const writes = () => http.filter((entry) => !['GET', 'HEAD'].includes(entry.method) && !entry.path.endsWith('/query'));
  async function releaseWrite(pathname, data, expectedStatus, actor = context) {
    const response = await authenticatedRequest(actor, base, pathname, {
      method: 'POST',
      headers: { 'Idempotency-Key': randomUUID() },
      data,
    });
    assert.equal(response.status(), expectedStatus, `POST ${pathname}: ${await response.text()}`);
    return response.json();
  }
  async function publishRelease(draft) {
    let order = await releaseWrite(`/api/v1/release-orders/${draft.id}/submit`, { expected_version: draft.version }, 200);
    order = await releaseWrite(`/api/v1/release-orders/${draft.id}/approve`, await fixtureApprovalInput(approvalContext, base, draft.id, { expected_version: order.version, reason: 'Independent operation coverage review' }), 200, approvalContext);
    const result = await releaseWrite(`/api/v1/release-orders/${draft.id}/execute`, { expected_version: order.version }, 200);
    await releaseWrite(`/api/v1/release-orders/${draft.id}/complete`, { expected_version: result.version }, 200);
    return result;
  }
  async function open(pathname) {
    if (page) await page.close();
    page = await context.newPage();
    page.setDefaultTimeout(12000);
    page.setDefaultNavigationTimeout(15000);
    const pageName = `page-${++sequence}`;
    page.on('pageerror', (error) => pageErrors.push({ page: pageName, url: page.url(), message: error.message }));
    page.on('response', async (response) => {
      const request = response.request();
      const url = new URL(response.url());
      if (!url.pathname.startsWith('/api/') || url.pathname.startsWith('/api/v1/auth/')) return;
      let body;
      if (!['GET', 'HEAD'].includes(request.method())) {
        try { body = request.postDataJSON(); } catch { body = request.postData() || undefined; }
      }
      http.push({ page: pageName, method: request.method(), path: url.pathname, status: response.status(), requestId: response.headers()['x-request-id'], ...(body === undefined ? {} : { body }) });
    });
    await page.goto(`${base}${pathname}`);
  }
  const button = (name) => page.getByRole('button', { name, exact: true });
  const input = (name) => page.getByRole('textbox', { name, exact: true });
  const checkbox = (name) => page.getByRole('checkbox', { name, exact: true });
  const autoFillInput = (name) => page.locator('label.field', { has: page.getByText(name, { exact: true }) }).locator('input');
  const operationToggle = (name) => page.locator('label.capability-toggle', { has: page.getByText(name, { exact: true }) }).getByRole('checkbox');
  const catalogRows = () => page.getByRole('region', { name: '表规则目录' }).locator('tbody tr');
  async function managed(table) {
    await open('/configuration/managed-data');
    const select = page.getByRole('combobox', { name: 'Managed Table', exact: true });
    await select.waitFor();
    await select.selectOption(table);
    await page.getByRole('region', { name: 'Managed Data 查询结果' }).locator('tbody').waitFor();
  }
  try {
    browser = await selectedBrowser(playwright).launch(browserOptions());
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    account = await registerFixtureAccount(context, base);
    approvalContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const reviewer = await registerFixtureAccount(approvalContext, base, { roles: ['VIEWER'] });
    sql(fixtureSQL());
    await configureFixtureReleaseTemplates(context, base, tables);
    await createFixtureApprovalRole(context, base, `Operation review ${randomUUID()}`, [reviewer.accountID], ['stage3_active_items', 'stage3_denied_items']);
    browserVersion = browser.version();

    const draftCode = 'stage3_ui_mutation_v1';
    await open('/platform/mutation-policies/new');
    await page.getByRole('heading', { name: '新建变更规则草稿' }).waitFor();
    await page.getByRole('textbox', { name: /规则编码/ }).fill(draftCode);
    await input('显示名称').fill('Stage 3 UI mutation draft');
    await input('描述').fill('Created by the real browser with initially denied operations');
    await button('创建草稿').click();
    await page.waitForURL(`**/platform/mutation-policies/${draftCode}`);
    assert.equal(responseFor('POST', '/api/v1/mutation-policies').status, 201);
    assert.equal(sql(`SELECT CONCAT_WS('|',allow_add,allow_modify,allow_delete,COALESCE(create_operator_field,'NULL'),COALESCE(create_time_field,'NULL'),COALESCE(modify_operator_field,'NULL'),COALESCE(modify_time_field,'NULL')) FROM rcc_mutation_policies WHERE code=${literal(draftCode)};`), '0|0|0|NULL|NULL|NULL|NULL');
    check('browser creates a complete Mutation Draft through Admin', { http: responseFor('POST', '/api/v1/mutation-policies'), sql: '0|0|0|NULL|NULL|NULL|NULL' });

    await page.getByLabel('变更规则详情', { exact: true }).getByRole('button', { name: '修改执行规则', exact: true }).click();
    await page.getByRole('heading', { name: '编辑变更规则草稿' }).waitFor();
    const editPath = `/api/v1/mutation-policies/${draftCode}`;
    let writeCount = writes().filter((entry) => entry.method === 'PUT' && entry.path === editPath).length;
    await autoFillInput('新增时填写 Operator 的列').fill('created_by');
    await button('保存执行规则').click();
    assert.equal(writes().filter((entry) => entry.method === 'PUT' && entry.path === editPath).length, writeCount);
    assert.match(await page.locator('#createOperatorField-error').innerText(), /需要允许 ADD/);
    assert.equal(await autoFillInput('新增时填写 Operator 的列').inputValue(), 'created_by');
    check('permission conflict is retained and blocks the HTTP write');

    await operationToggle('ADD').check();
    await autoFillInput('新增时填写创建时间的列').fill('created-at');
    await button('保存执行规则').click();
    assert.equal(writes().filter((entry) => entry.method === 'PUT' && entry.path === editPath).length, writeCount);
    assert.match(await page.locator('#createTimeField-error').innerText(), /安全的字段名/);
    assert.equal(await autoFillInput('新增时填写创建时间的列').inputValue(), 'created-at');
    check('unsafe Auto Fill column is retained and blocks the HTTP write');

    await autoFillInput('新增时填写创建时间的列').fill('ID');
    assert.equal(await page.locator('#createTimeField-error').count(), 0);
    await button('保存执行规则').click();
    await page.locator('#createTimeField-error').waitFor();
    assert.equal(writes().filter((entry) => entry.method === 'PUT' && entry.path === editPath).length, writeCount);
    assert.match(await page.locator('#createTimeField-error').innerText(), /id 是主键/);
    assert.equal(await autoFillInput('新增时填写创建时间的列').inputValue(), 'ID');
    check('case-insensitive id Auto Fill target is retained and blocks the HTTP write');

    await autoFillInput('新增时填写创建时间的列').fill('created_at');
    await autoFillInput('新增和修改时填写 Operator 的列').fill('created_by');
    await button('保存执行规则').click();
    assert.equal(writes().filter((entry) => entry.method === 'PUT' && entry.path === editPath).length, writeCount);
    assert.match(await page.locator('#createOperatorField-error').innerText(), /不能重复/);
    assert.match(await page.locator('#modifyOperatorField-error').innerText(), /不能重复/);
    check('exact duplicate Auto Fill columns block the HTTP write');

    await autoFillInput('新增和修改时填写 Operator 的列').fill('updated_by');
    await autoFillInput('新增时填写 Operator 的列').fill('creator');
    await autoFillInput('新增时填写 Operator 的列').fill('created_by');
    await autoFillInput('新增和修改时填写 Operator 的列').fill('CREATED_BY');
    assert.equal(await page.locator('#createOperatorField-error').count(), 0);
    assert.equal(await page.locator('#modifyOperatorField-error').count(), 0);
    await button('保存执行规则').click();
    await page.locator('#modifyOperatorField-error').waitFor();
    assert.equal(writes().filter((entry) => entry.method === 'PUT' && entry.path === editPath).length, writeCount, 'case-insensitive duplicate Auto Fill must not reach Admin');
    assert.match(await page.locator('#createOperatorField-error').innerText(), /不能重复/);
    assert.match(await page.locator('#modifyOperatorField-error').innerText(), /不能重复/);
    check('case-insensitive duplicate Auto Fill columns block the HTTP write');

    await autoFillInput('新增和修改时填写 Operator 的列').fill('updated_by');
    await autoFillInput('新增和修改时填写更新时间的列').fill('updated_at');
    await operationToggle('MODIFY').check();
    await operationToggle('DELETE').check();
    await input('显示名称').fill('Stage 3 UI mutation updated');
    await input('描述').fill('All three grants and all four Auto Fill slots saved by PUT');
    const putResponse = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname === editPath);
    await button('保存执行规则').click();
    assert.equal((await putResponse).status(), 200);
    await page.waitForURL(`**/platform/mutation-policies/${draftCode}`);
    const saved = sql(`SELECT CONCAT_WS('|',name,description,allow_add,allow_modify,allow_delete,create_operator_field,create_time_field,modify_operator_field,modify_time_field,status) FROM rcc_mutation_policies WHERE code=${literal(draftCode)};`);
    assert.equal(saved, 'Stage 3 UI mutation updated|All three grants and all four Auto Fill slots saved by PUT|1|1|1|created_by|created_at|updated_by|updated_at|DRAFT');
    await page.reload();
    await page.getByRole('heading', { name: '变更规则详情' }).waitFor();
    assert.equal(await input('显示名称').inputValue(), 'Stage 3 UI mutation updated');
    assert.equal(await operationToggle('ADD').isChecked(), true);
    assert.equal(await autoFillInput('新增和修改时填写更新时间的列').inputValue(), 'updated_at');
    await page.screenshot({ path: `${output}/mutation-draft-reopened.png`, fullPage: true });
    check('Mutation Draft PUT survives close and reopen with all execution fields', { http: responseFor('PUT', editPath), sql: saved });

    await page.getByLabel('变更规则详情', { exact: true }).getByRole('button', { name: '删除', exact: true }).click();
    const deleteResponse = page.waitForResponse((response) => response.request().method() === 'DELETE' && new URL(response.url()).pathname === editPath);
    await button('确认删除').click();
    assert.equal((await deleteResponse).status(), 204);
    await page.waitForURL('**/platform/mutation-policies');
    assert.equal(sql(`SELECT COUNT(*) FROM rcc_mutation_policies WHERE code=${literal(draftCode)};`), '0');
    check('Mutation Draft is deleted by the visible UI command', { http: responseFor('DELETE', editPath), sqlCount: 0 });

    for (const [kind, code, expectedStatus] of [['query', 'stage3_query_existing_v1', 'ACTIVE'], ['mutation', 'stage3_mutation_existing_v1', 'ACTIVE']]) {
      const collection = kind === 'query' ? 'query-policies' : 'mutation-policies';
      const beforeExecution = executionFields(kind, code);
      await open(`/platform/${collection}/${code}?mode=metadata`);
      await page.getByRole('heading', { name: kind === 'query' ? '修改查询规则名称和描述' : '修改变更规则名称和描述' }).waitFor();
      const locked = kind === 'query' ? input('默认排序字段') : operationToggle('ADD');
      assert.equal(await locked.isDisabled(), true);
      await input('显示名称').fill(`Stage 3 ${kind} renamed`);
      await input('描述').fill(`Stage 3 ${kind} metadata only`);
      const metadataPath = `/api/v1/${collection}/${code}/metadata`;
      const metadataResponse = page.waitForResponse((response) => response.request().method() === 'PATCH' && new URL(response.url()).pathname === metadataPath);
      await button('保存名称和描述').click();
      assert.equal((await metadataResponse).status(), 200);
      await page.waitForURL(`**/platform/${collection}/${code}`);
      assert.equal(executionFields(kind, code), beforeExecution);
      assert.equal(sql(`SELECT CONCAT_WS('|',name,description,status) FROM rcc_${kind}_policies WHERE code=${literal(code)};`), `Stage 3 ${kind} renamed|Stage 3 ${kind} metadata only|${expectedStatus}`);
      await page.reload();
      assert.equal(await input('显示名称').inputValue(), `Stage 3 ${kind} renamed`);
      assert.equal(await input('描述').inputValue(), `Stage 3 ${kind} metadata only`);
      check(`${kind} Active metadata saves without changing execution fields`, { http: responseFor('PATCH', metadataPath), executionBeforeAfter: beforeExecution });
    }

    for (const [kind, code] of [['query', 'stage3_query_existing_v1'], ['mutation', 'stage3_mutation_existing_v1']]) {
      const collection = `${kind}-policies`;
      await open(`/platform/${collection}/${code}`);
      const lifecyclePath = `/api/v1/${collection}/${code}/deprecate`;
      const response = page.waitForResponse((item) => item.request().method() === 'POST' && new URL(item.url()).pathname === lifecyclePath);
      await page.getByLabel(kind === 'query' ? '查询规则详情' : '变更规则详情', { exact: true }).getByRole('button', { name: '弃用', exact: true }).click();
      await button('确认弃用').click();
      assert.equal((await response).status(), 200);
      assert.equal(sql(`SELECT status FROM rcc_${kind}_policies WHERE code=${literal(code)};`), 'DEPRECATED');
      check(`${kind} is deprecated through the real lifecycle UI`, { http: responseFor('POST', lifecyclePath), status: 'DEPRECATED' });
    }

    for (const [kind, code] of [['query', 'stage3_query_existing_v1'], ['mutation', 'stage3_mutation_existing_v1']]) {
      const collection = `${kind}-policies`;
      const beforeExecution = executionFields(kind, code);
      await open(`/platform/${collection}/${code}?mode=metadata`);
      await page.getByRole('heading', { name: kind === 'query' ? '修改查询规则名称和描述' : '修改变更规则名称和描述' }).waitFor();
      await input('显示名称').fill(`Stage 3 ${kind} Deprecated renamed`);
      await input('描述').fill(`Stage 3 ${kind} Deprecated metadata only`);
      const metadataPath = `/api/v1/${collection}/${code}/metadata`;
      const response = page.waitForResponse((item) => item.request().method() === 'PATCH' && new URL(item.url()).pathname === metadataPath);
      await button('保存名称和描述').click();
      assert.equal((await response).status(), 200);
      await page.waitForURL(`**/platform/${collection}/${code}`);
      assert.equal(executionFields(kind, code), beforeExecution);
      assert.equal(sql(`SELECT CONCAT_WS('|',name,description,status) FROM rcc_${kind}_policies WHERE code=${literal(code)};`), `Stage 3 ${kind} Deprecated renamed|Stage 3 ${kind} Deprecated metadata only|DEPRECATED`);
      await page.reload();
      assert.equal(await input('显示名称').inputValue(), `Stage 3 ${kind} Deprecated renamed`);
      assert.equal(await input('描述').inputValue(), `Stage 3 ${kind} Deprecated metadata only`);
      check(`${kind} Deprecated metadata saves without changing execution fields`, { http: responseFor('PATCH', metadataPath), executionBeforeAfter: beforeExecution });
    }

    await managed('stage3_active_items');
    assert.ok(responseFor('POST', '/api/v1/tables/stage3_active_items/query').status === 200);
    await button('新增记录').click();
    await checkbox('包含 name').check();
    await page.getByRole('dialog').getByLabel('name 值',{exact:true}).fill('DeprecatedStillRuns');
    await button('查看 Change Set').click();
    const draftResponse = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/release-orders');
    await button('确认并保存草稿').click();
    const created = await draftResponse;
    assert.equal(created.status(), 201);
    const published = await publishRelease(await created.json());
    assert.equal(published.state, 'SUCCEEDED');
    assert.equal(sql(`SELECT CONCAT_WS('|',name,created_by,updated_by) FROM stage3_active_items WHERE name='DeprecatedStillRuns';`), `DeprecatedStillRuns|${account.accountID}|${account.accountID}`);
    check('existing assignment keeps querying and publishes through a release order with Deprecated definitions', { query: responseFor('POST', '/api/v1/tables/stage3_active_items/query'), releaseOrder: published.id, sql: `DeprecatedStillRuns|${account.accountID}|${account.accountID}` });

    await open('/platform/table-policies?mode=create');
    const candidate = page.getByRole('combobox', { name: '真实数据库表', exact: true });
    await candidate.selectOption('stage3_candidate_items');
    const queryOptions = await page.getByRole('combobox', { name: 'Active 查询规则' }).locator('option').evaluateAll((nodes) => nodes.map((node) => node.value));
    const mutationOptions = await page.getByRole('combobox', { name: 'Active 变更规则' }).locator('option').evaluateAll((nodes) => nodes.map((node) => node.value));
    assert.ok(queryOptions.includes('stage3_query_full_v1'));
    assert.ok(mutationOptions.includes('stage3_mutation_full_v1'));
    assert.ok(!queryOptions.includes('stage3_query_existing_v1') && !queryOptions.includes('stage3_query_draft_v1'));
    assert.ok(!mutationOptions.includes('stage3_mutation_existing_v1') && !mutationOptions.includes('stage3_mutation_draft_v1'));
    check('new assignment choices include only Active definitions', { queryOptions: queryOptions.filter((value) => value.startsWith('stage3_')), mutationOptions: mutationOptions.filter((value) => value.startsWith('stage3_')) });

    await open('/platform/table-policies');
    await page.getByText(/共 \d+ 个表规则/).waitFor();
    await catalogRows().filter({ hasText: 'stage3_catalog_alpha' }).waitFor();
    const unfilteredCount = await catalogRows().count();
    const initialWrites = writes().length;
    const filter = page.getByRole('searchbox', { name: '筛选表规则' });
    for (const [value, expected] of [
      ['  STAGE3_CATALOG_ALPHA  ', ['stage3_catalog_alpha']],
      ['stage3_query_full_v1', ['stage3_denied_items', 'stage3_catalog_alpha', 'stage3_catalog_beta']],
      ['stage3_mutation_denied_v1', ['stage3_denied_items']],
      ['stage3filterperson', ['stage3_catalog_alpha']],
    ]) {
      await filter.fill(value);
      const rows = catalogRows();
      await page.getByText(`显示 ${expected.length} 个`, { exact: true }).waitFor();
      for (const name of expected) await rows.filter({ hasText: name }).waitFor();
      assert.equal(await rows.count(), expected.length);
      for (const name of expected) assert.ok((await rows.allInnerTexts()).some((text) => text.includes(name)));
    }
    await filter.fill('stage3-no-such-policy');
    await page.getByText('没有匹配的表规则', { exact: true }).waitFor();
    await filter.fill('');
    await page.getByText(`显示 ${unfilteredCount} 个`, { exact: true }).waitFor();
    assert.ok(await catalogRows().count() >= 5);
    assert.equal(writes().length, initialWrites);
    check('Table Policy Catalog filters full loaded data without writes', { cases: ['table/trim/case', 'query code', 'mutation code', 'modifier', 'no match', 'clear'], writeCount: 0 });

    await managed('stage3_active_items');
    http.length = 0;
    await input('筛选 name 值').fill('Stage3Alpha');
    assert.equal(await button('添加条件').count(),0);
    const queryPath = '/api/v1/tables/stage3_active_items/query';
    const queryResponse = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === queryPath);
    await button('收起筛选').click();
    await button('查询').click();
    assert.equal((await queryResponse).status(),200);
    const submitted = responseFor('POST',queryPath);
    assert.deepEqual(submitted.body.conditions,[{field:'name',operator:'exact',value:'Stage3Alpha'}]);
    assert.equal(sql("SELECT COUNT(*) FROM stage3_active_items WHERE name='Stage3Alpha';"),'1');
    assert.equal(await page.getByRole('cell',{name:'Stage3Alpha',exact:true}).count(),1);
    await button('展开筛选').click();
    assert.equal(await input('筛选 name 值').inputValue(),'Stage3Alpha');
    await page.screenshot({path:`${output}/combined-field-query.png`,fullPage:true});
    check('direct field filter submits only entered AND conditions and preserves input after collapse/query',{http:submitted,sqlCount:1});

    await managed('stage3_denied_items');
    const addButton = button('新增记录');
    const modifyButton = page.getByRole('button', { name: /^修改记录 / }).first();
    const deleteButton = page.getByRole('button', { name: /^删除记录 / }).first();
    for (const [control, operation] of [[addButton, 'ADD'], [modifyButton, 'MODIFY'], [deleteButton, 'DELETE']]) {
      assert.equal(await control.isVisible(), true);
      assert.equal(await control.isDisabled(), true);
      assert.match(await control.getAttribute('title'), new RegExp(`${operation} 未由当前变更规则授权`));
      assert.match(await page.locator(`#mutation-${operation.toLowerCase()}-reason`).innerText(), new RegExp(`${operation} 未由当前变更规则授权`));
    }
    await page.screenshot({ path: `${output}/unauthorized-controls.png`, fullPage: true });
    check('all three unauthorized operation controls stay visible, disabled, and explained');

    await open('/platform/table-policies/stage3_denied_items?mode=replace');
    await page.getByRole('combobox', { name: 'Active 变更规则' }).selectOption('stage3_mutation_full_v1');
    const replacePath = '/api/v1/table-policies/stage3_denied_items';
    await button('检查并替换').click();
    const replaceResponse = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname === replacePath);
    await button('确认替换').click();
    assert.equal((await replaceResponse).status(), 200);
    assert.equal(sql(`SELECT mutation_policy_code FROM rcc_table_policies WHERE table_name='stage3_denied_items';`), 'stage3_mutation_full_v1');
    await managed('stage3_denied_items');
    assert.equal(await button('新增记录').isEnabled(), true);
    assert.equal(await page.getByRole('button', { name: /^修改记录 / }).first().isEnabled(), true);
    assert.equal(await page.getByRole('button', { name: /^删除记录 / }).first().isEnabled(), true);
    await button('新增记录').click();
    await checkbox('包含 name').check();
    await page.getByRole('dialog').getByLabel('name 值',{exact:true}).fill('CapabilitySwitchApplied');
    await button('查看 Change Set').click();
    const switchedDraftResponse = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/release-orders');
    await button('确认并保存草稿').click();
    const switchedCreated = await switchedDraftResponse;
    assert.equal(switchedCreated.status(), 201);
    const switchedPublished = await publishRelease(await switchedCreated.json());
    assert.equal(switchedPublished.state, 'SUCCEEDED');
    assert.equal(sql(`SELECT COUNT(*) FROM stage3_denied_items WHERE name='CapabilitySwitchApplied';`), '1');
    check('real Table Policy replacement governs the next UI capability and release publication', { replacement: responseFor('PUT', replacePath), releaseOrder: switchedPublished.id, sqlCount: 1 });

    assert.equal(http.some((entry) => /^\/api\/v1\/tables\/[^/]+\/rows(?:\/|$)/.test(entry.path) && !['GET', 'HEAD'].includes(entry.method)), false, 'browser must not use removed direct record write routes');

    assert.deepEqual(pageErrors, []);
  } catch (error) {
    failure = { name: error.name, message: error.message, stack: error.stack };
    if (page) {
      await page.screenshot({ path: `${output}/failure.png`, fullPage: true }).catch(() => {});
      await fs.writeFile(`${output}/failure-body.txt`, await page.locator('body').innerText().catch(() => 'page unavailable')).catch(() => {});
    }
    throw error;
  } finally {
    if (browser) await browser.close().catch(() => {});
    try {
      sql(`DELETE a FROM rcc_table_release_templates a JOIN rcc_table_policies p ON p.id=a.table_policy_id WHERE p.table_name LIKE 'stage3\\_%';
        DELETE FROM rcc_table_policies WHERE table_name LIKE 'stage3\\_%';
        DELETE FROM rcc_query_policies WHERE code LIKE 'stage3\\_%';
        DELETE FROM rcc_mutation_policies WHERE code LIKE 'stage3\\_%';
        ${tables.map((name) => `DROP TABLE IF EXISTS ${name};`).join('\n')}`);
      const after = fixtureRows();
      cleanup = {
        fixtureRowsEqual: before === after,
        tablePolicies: Number(sql(`SELECT COUNT(*) FROM rcc_table_policies WHERE table_name LIKE 'stage3\\_%';`)),
        queryPolicies: Number(sql(`SELECT COUNT(*) FROM rcc_query_policies WHERE code LIKE 'stage3\\_%';`)),
        mutationPolicies: Number(sql(`SELECT COUNT(*) FROM rcc_mutation_policies WHERE code LIKE 'stage3\\_%';`)),
        physicalTables: Number(sql(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name LIKE 'stage3\\_%';`)),
      };
      assert.deepEqual(cleanup, { fixtureRowsEqual: true, tablePolicies: 0, queryPolicies: 0, mutationPolicies: 0, physicalTables: 0 });
    } catch (cleanupError) {
      cleanup = { error: cleanupError.message };
      if (!failure) failure = { name: cleanupError.name, message: cleanupError.message, stack: cleanupError.stack };
      process.exitCode = 1;
    }
    await fs.writeFile(`${output}/http-evidence.json`, JSON.stringify(http, null, 2) + '\n');
    await fs.writeFile(`${output}/result.json`, JSON.stringify({ ok: failure === null, base, browser: browserVersion, passed, evidence, pageErrors, cleanup, failure }, null, 2) + '\n');
  }
})().catch((error) => { console.error(error); process.exitCode = 1; });
