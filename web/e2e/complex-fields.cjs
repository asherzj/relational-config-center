// Real Playwright browser -> production same-origin Web proxy -> Cookie-authenticated Admin -> disposable MySQL 8.4.
// SQL arranges this suite's tables and independently verifies bytes and database semantics.
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const { browserOptions, registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');
const fs = require('node:fs/promises');
const { execFileSync } = require('node:child_process');
const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const container = process.env.RCC_E2E_MYSQL_CONTAINER;
const engine = process.env.RCC_E2E_ENGINE || 'chromium';
const browserType = playwright[engine];
assert.ok(browserType, `unsupported Playwright engine ${engine}`);
assert.match(container || '', /^rcc-browser-\d+-\d+-mysql$/);
const tables = ['stage4_complex', 'stage4_ids', 'stage4_defaults', 'stage4_auto', 'stage4_generated', 'stage4_limits', 'stage4_zero_ids', 'stage4_default_id_constant', 'stage4_default_id_expression', 'stage4_explicit_ids'];
const hex = (value) => Buffer.from(value, 'utf8').toString('hex');
const literal = (value) => `CONVERT(0x${hex(value)} USING utf8mb4)`;
const sql = (statement) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c', 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --default-character-set=utf8mb4 --raw --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE"'], { input: statement, encoding: 'utf8', timeout: 20000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
const releaseOrderCount = (table) => sql(`SELECT COUNT(*) FROM rcc_release_orders WHERE table_name=${literal(table)};`);
const sqlAsRoot = (statement) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c', 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --default-character-set=utf8mb4 --raw --batch --skip-column-names -uroot "$MYSQL_DATABASE"'], { input: statement, encoding: 'utf8', timeout: 20000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
const publicationState = (table) => ({
  businessRows: sql(`SELECT COUNT(*) FROM ${table};`),
  recordVersions: sql(`SELECT COUNT(*) FROM rcc_record_versions WHERE table_name=${literal(table)};`),
  activeTargets: sql(`SELECT COUNT(*) FROM rcc_release_targets WHERE table_name=${literal(table)};`),
  commands: sql(`SELECT COUNT(*) FROM rcc_publication_commands WHERE table_name=${literal(table)};`),
  notifications: sql(`SELECT COUNT(*) FROM rcc_refresh_notifications WHERE table_name=${literal(table)};`),
  tableVersion: sql(`SELECT CONCAT(table_version,'|',command_cursor) FROM rcc_table_publications WHERE table_name=${literal(table)};`),
});
const row = (table, id) => JSON.parse(sql(`SELECT JSON_OBJECT(${Object.entries({id:'CAST(id AS CHAR)',note:'note',payload:'CAST(payload AS CHAR)',nullable_text:'nullable_text',empty_text:'empty_text',default_text:'default_text',big_signed:'CAST(big_signed AS CHAR)',big_unsigned:'CAST(big_unsigned AS CHAR)',amount:'CAST(amount AS CHAR)',day:'CAST(day AS CHAR)',clock:'CAST(clock AS CHAR)',local_time:'CAST(local_time AS CHAR)',instant:"CONCAT(DATE_FORMAT(instant,'%Y-%m-%dT%H:%i:%s.%f'),'Z')"}).map(([key, expression]) => `'${key}',${expression}`).join(',')}) FROM ${table} WHERE id=${id};`));
const fixtureRows = () => sql("SELECT CONCAT_WS('|',id,HEX(name),IF(category IS NULL,'NULL',HEX(category)),IF(note IS NULL,'NULL',HEX(note)),HEX(state),priority,HEX(created_by),DATE_FORMAT(created_at,'%Y%m%d%H%i%s.%f'),HEX(updated_by),DATE_FORMAT(updated_at,'%Y%m%d%H%i%s.%f')) FROM stage1_acceptance_items ORDER BY id;");
const matrix = [
  ['text/json', 'Chinese, emoji, LF, TAB, surrounding spaces and long text', 'exact submitted bytes for TEXT; MySQL canonical JSON, including integer > 2^53'],
  ['null/empty/omitted', 'SQL NULL, JSON literal null, JSON string "null", empty string and omitted ADD/MODIFY', 'distinct values; default on ADD, retain on MODIFY'],
  ['numeric/temporal', 'uint64 IDs above 2^53 and 2^63, near upper bound; signed/unsigned endpoints, DECIMAL, UTC microseconds', 'string HTTP, exact SQL and reopened editor'],
  ['defaults/generated', 'ordinary expression default explicit/omitted/Auto Fill; real STORED/VIRTUAL generated', 'ordinary values writable, Auto Fill succeeds, generated writes rejected'],
  ['storage limits', 'VARCHAR too long and signed TINYINT out of range', 'stable editable rejection, rollback, successful corrected retry'],
];
function fixtureSQL() {
  return `CREATE TABLE stage4_complex (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, note TEXT, payload JSON,
    nullable_text TEXT, empty_text VARCHAR(64), default_text VARCHAR(64) DEFAULT '数据库默认🙂',
    big_signed BIGINT, big_unsigned BIGINT UNSIGNED, amount DECIMAL(65,20),
    day DATE, clock TIME(6), local_time DATETIME(6), instant TIMESTAMP(6) NULL
  ) CHARACTER SET utf8mb4;
  CREATE TABLE stage4_ids (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(64)) CHARACTER SET utf8mb4;
  CREATE TABLE stage4_defaults (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(64), stamp DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), expression_value VARCHAR(64) DEFAULT (CONCAT('表达式','🙂'))) CHARACTER SET utf8mb4;
  CREATE TABLE stage4_auto (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(64), created_by CHAR(36), created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), updated_by CHAR(36), updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)) CHARACTER SET utf8mb4;
  CREATE TABLE stage4_generated (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, base_value INT DEFAULT 7, stored_value INT GENERATED ALWAYS AS (base_value*2) STORED, virtual_value INT GENERATED ALWAYS AS (base_value+1) VIRTUAL);
  CREATE TABLE stage4_zero_ids (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(64));
  CREATE TABLE stage4_default_id_constant (id BIGINT UNSIGNED PRIMARY KEY DEFAULT 42, label VARCHAR(64));
  CREATE TABLE stage4_default_id_expression (id BIGINT UNSIGNED PRIMARY KEY DEFAULT (40+3), label VARCHAR(64));
  CREATE TABLE stage4_explicit_ids (id BIGINT PRIMARY KEY, label VARCHAR(64));
  CREATE TABLE stage4_limits (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(4), small_value TINYINT);
  INSERT INTO rcc_mutation_policies(code,name,description,type_code,allow_add,allow_modify,allow_delete,status,creator,modifier) VALUES ('stage4_plain_v1','Stage 4 plain','','single_table_mutation',1,1,1,'ACTIVE','fixture','fixture');
  INSERT INTO rcc_mutation_policies(code,name,description,type_code,allow_add,allow_modify,allow_delete,create_operator_field,create_time_field,modify_operator_field,modify_time_field,status,creator,modifier) VALUES ('stage4_auto_v1','Stage 4 auto','','single_table_mutation',1,1,1,'created_by','created_at','updated_by','updated_at','ACTIVE','fixture','fixture');
  INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES ${tables.map((table) => `('${table}','stage1_query_v1','${table === 'stage4_auto' ? 'stage4_auto_v1' : 'stage4_plain_v1'}',1,'fixture','fixture')`).join(',')};`;
}
(async () => {
  await fs.mkdir(output, { recursive: true });
  await fs.writeFile(`${output}/semantic-matrix.json`, JSON.stringify(matrix, null, 2));
  const before = fixtureRows();
  const http = [], evidence = [], failures = [], pageErrors = [];
  const previewDifferences = [];
  const sqlErrors = [];
  let browser, context, approver, publisher, page, cleanup, browserVersion, currentOrder;
  const button = (name) => page.getByRole('button', { name, exact: true });
  const input = (name) => page.getByRole('textbox', { name: `${name} 值`, exact: true });
  const checkbox = (name) => page.getByRole('checkbox', { name, exact: true });
  async function managed(table) {
    if (page) await page.close();
    page = await context.newPage();
    page.setDefaultTimeout(12000);
    page.on('pageerror', (error) => pageErrors.push(error.message));
    await page.goto(`${base}/configuration/managed-data`);
    const [response] = await Promise.all([
      page.waitForResponse((r) => new URL(r.url()).pathname === `/api/v1/tables/${table}/query`),
      page.getByRole('combobox', { name: 'Managed Table', exact: true }).selectOption(table),
    ]);
    const result = await record(response);
    assert.equal(result.status, 200, JSON.stringify(result));
    await page.getByRole('region', { name: 'Managed Data 查询结果' }).locator('tbody').waitFor();
  }
  async function record(response) {
    let responseBody;
    try { responseBody = await response.json(); } catch { responseBody = null; }
    const request = response.request();
    const item = { method: request.method(), path: new URL(response.url()).pathname, status: response.status(), requestId: response.headers()['x-request-id'], idempotencyKey: request.headers()['idempotency-key'], body: request.postDataJSON(), response: responseBody };
    http.push(item);
    return item;
  }
  async function api(actor, method, path, body, expected = 200, key = randomUUID()) {
    const response = await authenticatedRequest(actor, base, path, {
      method,
      headers: { 'Idempotency-Key': key },
      ...(body === undefined ? {} : { data: body }),
    });
    let responseBody;
    try { responseBody = await response.json(); } catch { responseBody = null; }
    const entry = { method, path, status: response.status(), requestId: response.headers()['x-request-id'], body, response: responseBody, transport: 'Playwright APIRequest through real same-origin proxy' };
    http.push(entry);
    const expectedStatuses = Array.isArray(expected) ? expected : [expected];
    assert.ok(expectedStatuses.includes(entry.status), `expected ${expectedStatuses.join(' or ')}: ${JSON.stringify(entry)}`);
    return entry;
  }
  async function assignRoles(administrator, accountID, roles) {
    let after = '';
    let account;
    do {
      const path = `/api/v1/account-roles?limit=100${after ? `&after=${encodeURIComponent(after)}` : ''}`;
      const listed = await authenticatedRequest(administrator, base, path, { method: 'GET' });
      assert.equal(listed.status(), 200, await listed.text());
      const page = await listed.json();
      account = page.accounts.find((candidate) => candidate.id === accountID);
      after = page.next_cursor;
    } while (!account && after);
    assert.ok(account, `role catalog omitted ${accountID}`);
    const changed = await authenticatedRequest(administrator, base, `/api/v1/account-roles/${accountID}`, {
      method: 'PUT', headers: { 'Idempotency-Key': randomUUID() }, data: { roles, expected_version: account.version },
    });
    assert.equal(changed.status(), 200, await changed.text());
    assert.deepEqual((await changed.json()).roles, roles);
  }
  async function edit(values) {
    const adding = await page.getByRole('dialog', { name: /^新增 .* 记录$/ }).count() === 1;
    for (const [name, value] of Object.entries(values)) {
      if (adding) await checkbox(`包含 ${name}`).check();
      else assert.equal(await checkbox(`包含 ${name}`).count(), 0, 'MODIFY fields are editable without an inclusion toggle');
      if (value === null) await checkbox(`${name} 使用 NULL`).check();
      else {
        const nullToggle = checkbox(`${name} 使用 NULL`);
        if (await nullToggle.count() && await nullToggle.isChecked()) await nullToggle.uncheck();
        await input(name).fill(value);
      }
    }
  }
  async function preview(operation, values) {
    await button('查看 Change Set').click();
    const dialog = page.getByRole('dialog', { name: `${operation} Change Set`, exact: true });
    await dialog.waitFor();
    for (const [name, value] of Object.entries(values)) {
      const fieldRow = dialog.locator('tbody tr').filter({ has: page.getByRole('rowheader', { name: new RegExp(`^${name}(\\s*变化)?$`) }) });
      if (value !== null && value !== '') {
        const actual = await fieldRow.locator('td').last().textContent();
        if (actual !== value) previewDifferences.push({ field: name, expected: value, actual });
      }
    }
  }
  async function execute(table, operation, values, id, status = 200) {
    let expectedContent = values;
    if (operation === 'MODIFY') {
      const original = (await api(context, 'POST', `/api/v1/tables/${table}/query`, { conditions: [{ field: 'id', operator: 'exact', value: id }] })).response;
      const assignment = (await api(context, 'GET', `/api/v1/table-policies/${table}`)).response;
      const policy = (await api(context, 'GET', `/api/v1/mutation-policies/${assignment.mutation_policy_code}`)).response;
      const reserved = new Set(['id', policy.create_operator_field, policy.create_time_field, policy.modify_operator_field, policy.modify_time_field]);
      expectedContent = { ...Object.fromEntries(original.columns.filter(column => !column.generated && !reserved.has(column.name)).map(column => [column.name, original.rows[0][column.name]])), ...values };
    }
    await preview(operation, values);
    const requestedStatuses = Array.isArray(status) ? status : [status];
    const expectedFailures = requestedStatuses.filter(candidate => candidate >= 400);
    const pending = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/release-orders' && response.request().method() === 'POST');
    await button('确认并保存草稿').click();
    const draft = await record(await pending);
    assert.equal(draft.body.table_name, table);
    assert.equal(draft.body.items.length, 1);
    assert.equal(draft.body.items[0].operation, operation);
    assert.deepEqual(draft.body.items[0].content, expectedContent, 'release draft content must preserve edited and unchanged original values');
    if (id !== undefined) assert.equal(draft.body.items[0].id, id, 'record identity must travel in JSON');
    assert.deepEqual(previewDifferences, [], 'Change Set must preserve exact values');
    assert.ok(draft.requestId);
    if (draft.status !== 201) {
      assert.ok(expectedFailures.includes(draft.status), JSON.stringify(draft));
      return { ...draft, stage: 'draft' };
    }
    currentOrder = draft.response;
    assert.equal(currentOrder.state, 'DRAFT');
    assert.deepEqual(currentOrder.items[0].content, expectedContent, 'stored draft must retain the exact submitted content');
    await page.waitForURL(`**/configuration/release-orders/${currentOrder.id}`);
    await page.getByRole('heading', { name: `${table} 配置变更`, exact: true }).waitFor();

    const submitted = await api(context, 'POST', `/api/v1/release-orders/${currentOrder.id}/submit`, { expected_version: currentOrder.version }, expectedFailures.length ? [200, ...expectedFailures] : 200);
    if (submitted.status !== 200) {
      const retained = await api(context, 'GET', `/api/v1/release-orders/${currentOrder.id}`);
      currentOrder = retained.response;
      assert.equal(currentOrder.state, 'DRAFT');
      assert.deepEqual(currentOrder.items[0].content, expectedContent);
      return { ...submitted, stage: 'submit', order: currentOrder };
    }
    currentOrder = submitted.response;
    assert.equal(currentOrder.state, 'PENDING_APPROVAL');
    const approved = await api(approver, 'POST', `/api/v1/release-orders/${currentOrder.id}/approve`, { expected_version: currentOrder.version, reason: 'Independent complex field review' }, expectedFailures.length ? [200, ...expectedFailures] : 200);
    if (approved.status !== 200) {
      const retained = await api(context, 'GET', `/api/v1/release-orders/${currentOrder.id}`);
      currentOrder = retained.response;
      assert.equal(currentOrder.state, 'PENDING_APPROVAL');
      assert.deepEqual(currentOrder.items[0].content, expectedContent);
      return { ...approved, stage: 'approval', order: currentOrder };
    }
    currentOrder = approved.response;
    assert.equal(currentOrder.state, 'APPROVED');
    assert.notEqual(currentOrder.applicant_id, currentOrder.history.find((event) => event.action === 'APPROVE').actor_id, 'approval must come from another permanent account');

    const beforePublication = expectedFailures.length ? publicationState(table) : undefined;
    const result = await api(publisher, 'POST', `/api/v1/release-orders/${currentOrder.id}/execute`, { expected_version: currentOrder.version }, expectedFailures.length ? expectedFailures : 200);
    if (expectedFailures.length) {
      const afterPublication = publicationState(table);
      assert.deepEqual(afterPublication, beforePublication, 'failed publication must roll back business rows, versions, commands, table version and notifications');
      const retained = await api(context, 'GET', `/api/v1/release-orders/${currentOrder.id}`);
      currentOrder = retained.response;
      assert.equal(currentOrder.state, 'APPROVED', 'failed publication keeps the approved release retryable');
      assert.deepEqual(currentOrder.items[0].content, expectedContent, 'failed publication retains the exact approved input');
      await page.reload();
      await page.getByRole('heading', { name: `${table} 配置变更`, exact: true }).waitFor();
      return { ...result, stage: 'publication', order: currentOrder, beforePublication, afterPublication };
    }
    currentOrder = result.response;
    assert.equal(currentOrder.state, 'SUCCEEDED');
    assert.equal(currentOrder.publication.commands.length, 1);
    assert.equal(currentOrder.publication.commands[0].operation, operation);
    await page.reload();
    await page.getByRole('heading', { name: `${table} 配置变更`, exact: true }).waitFor();
    await page.getByText(`${table} · 已发布待完结`, { exact: true }).waitFor();
    // Finish each independent data fixture before a later case uses its identity.
    await api(publisher, 'POST', `/api/v1/release-orders/${currentOrder.id}/complete`, { expected_version: currentOrder.version });
    return { ...result, response: { ...result.response, id: currentOrder.publication.commands[0].id }, stage: 'publication' };
  }
  async function closeSuccess(operation, expected, commandExpected = expected) {
    assert.equal(currentOrder.state, 'SUCCEEDED');
    const command = currentOrder.publication.commands[0];
    assert.equal(command.operation, operation);
    if (commandExpected) {
      const actual = Object.fromEntries(command.final.fields.map((field) => [field.name, field.encoding === 'sql_null' ? null : field.value]));
      for (const [name, value] of Object.entries(commandExpected)) assert.equal(actual[name], value, `${name} publication result`);
    }
  }
  async function returnToRejectedDraft(rejected, expected) {
    assert.match(rejected.idempotencyKey, /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/);
    assert.equal(await page.getByRole('heading', { name: '待处理发布请求', exact: true }).count(), 0, 'definitive pre-order rejection must clear the request journal');
    const back = button('返回修改');
    assert.equal(await back.isEnabled(), true, 'definitive pre-order rejection must unlock retained input for editing');
    await back.click();
    for (const [name, value] of Object.entries(expected)) {
      if (rejected.body.items[0].operation === 'ADD') assert.equal(await checkbox(`包含 ${name}`).isChecked(), true, `${name} inclusion must remain selected`);
      else assert.equal(await checkbox(`包含 ${name}`).count(), 0);
      assert.equal(await input(name).inputValue(), value ?? '', `${name} rejected input must remain editable`);
      const nullToggle = checkbox(`${name} 使用 NULL`);
      assert.equal(await nullToggle.count() ? await nullToggle.isChecked() : false, value === null, `${name} rejected NULL state`);
    }
  }
  async function reopened(table, id, expected) {
    await managed(table);
    await button(`修改记录 ${id}`).click();
    for (const [name, value] of Object.entries(expected)) {
      if (name === 'id') continue;
      assert.equal(await input(name).inputValue(), value ?? '', `${name} reopened editor`);
      const nullToggle = checkbox(`${name} 使用 NULL`);
      assert.equal(await nullToggle.count() ? await nullToggle.isChecked() : false, value === null, `${name} NULL state`);
    }
  }
  async function run(name, fn) {
    if (process.env.RCC_E2E_CASE && !name.includes(process.env.RCC_E2E_CASE)) return;
    const first = http.length;
    previewDifferences.length = 0;
    try { const details = await fn(); evidence.push({ case: name, ok: true, http: [first, http.length], ...details }); console.log('PASS', name); }
    catch (error) {
      failures.push({ case: name, message: error.message, stack: error.stack, http: [first, http.length], previewDifferences: [...previewDifferences], idAndCRSnapshot: { zero: sql('SELECT id FROM stage4_zero_ids;'), constant: sql('SELECT id FROM stage4_default_id_constant;'), expression: sql('SELECT id FROM stage4_default_id_expression;'), textHex: name.startsWith('CRLF') ? sql('SELECT HEX(note) FROM stage4_complex;') : undefined }, sqlSnapshot: Object.fromEntries(tables.map((table) => [table, sql(`SELECT COUNT(*) FROM ${table};`)])), ...(name.startsWith('TEXT') ? { savedTextHex: sql('SELECT HEX(note) FROM stage4_complex ORDER BY id;') } : {}) });
      console.error('FAIL', name, error.message.slice(0, 1500));
      if (page) await page.screenshot({ path: `${output}/failure-${failures.length}.png`, fullPage: true }).catch(() => {});
    }
  }
  try {
    sql(fixtureSQL());
    browser = await browserType.launch(browserOptions()); browserVersion = browser.version();
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: 'America/Los_Angeles' });
    approver = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: 'America/Los_Angeles' });
    publisher = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: 'America/Los_Angeles' });
    const applicantIdentity = await registerFixtureAccount(context, base);
    const approverIdentity = await registerFixtureAccount(approver, base);
    const publisherIdentity = await registerFixtureAccount(publisher, base);
    await assignRoles(context, approverIdentity.accountID, ['APPROVER']);
    await assignRoles(context, publisherIdentity.accountID, ['PUBLISHER']);
    await assignRoles(context, applicantIdentity.accountID, ['EDITOR', 'ADMIN']);
    await run('TEXT and JSON browser ADD, MODIFY, SQL byte comparison, readback and reopened editor', async () => {
      const values = {
        note: `  中文🙂第一行\n第二行\t保留空白\n${'长文本🧪 abc\t'.repeat(700)}\n尾部  `,
        payload: '  {\n "text": "中文🙂\\n下一行\\t末尾  ",\n "large": 9007199254740993, "null": null\n }  ',
        nullable_text: null, empty_text: '', big_signed: '-9223372036854775808', big_unsigned: '18446744073709551615',
        amount: '123456789012345678901234567890123456789012345.12345678901234567890',
        day: '2024-02-29', clock: '23:59:59.123456', local_time: '2026-09-07 12:34:56.123456', instant: '2026-09-07T12:34:56.654321Z',
      };
      await managed('stage4_complex'); await button('新增记录').click(); await edit(values);
      const response = await execute('stage4_complex', 'ADD', values);
      const id = response.response.id;
      const expected = { id, ...values, default_text: '数据库默认🙂', payload: sql(`SELECT CAST(CAST(${literal(values.payload)} AS JSON) AS CHAR);`) };
      assert.deepEqual(row('stage4_complex', id), expected);
      await closeSuccess('ADD', expected, { ...expected, instant: '2026-09-07 12:34:56.654321' }); await reopened('stage4_complex', id, expected);
      const changed = { note: `${values.note}\n追加🙂\t  `, payload: '{ "large": 18446744073709551615, "text": "修改🙂\\n\\t " }', big_signed: '9223372036854775807', big_unsigned: '0', amount: '-123456789012345678901234567890123456789012345.12345678901234567890', clock: '00:00:00.000001' };
      await edit(changed); await execute('stage4_complex', 'MODIFY', changed, id);
      const modified = { ...expected, ...changed, payload: sql(`SELECT CAST(CAST(${literal(changed.payload)} AS JSON) AS CHAR);`) };
      assert.deepEqual(row('stage4_complex', id), modified);
      await closeSuccess('MODIFY', modified, { ...modified, instant: '2026-09-07 12:34:56.654321' }); await reopened('stage4_complex', id, modified);
      return { id, textBytes: Buffer.byteLength(modified.note), sql: modified };
    });
    await run('multiline TEXT query uses the exact browser input without removing LF', async () => {
      const note = '  查询🙂\n下一行\t末尾  ';
      sql(`INSERT INTO stage4_complex(note) VALUES (${literal(note)});`);
      await managed('stage4_complex');
      await page.getByRole('textbox', { name: '筛选 note 值', exact: true }).evaluate((node, value) => {const clipboardData=new DataTransfer();clipboardData.setData('text/plain',value);node.dispatchEvent(new ClipboardEvent('paste',{clipboardData,bubbles:true,cancelable:true}));},note);
      const pending = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/tables/stage4_complex/query');
      await button('查询').click(); const response = await record(await pending);
      assert.equal(response.status, 200); assert.equal(response.body.conditions[0].value, note);
      assert.equal(response.response.rows.length, 1); assert.equal(response.response.rows[0].note, note);
      return { note, matched: 1 };
    });
    await run('SQL NULL, JSON null literal and string, empty and omitted values stay distinct', async () => {
      const results = [];
      for (const payload of [null, 'null', '"null"', '""']) {
        await managed('stage4_complex'); await button('新增记录').click();
        const values = { payload, empty_text: '', nullable_text: null };
        await edit(values); const response = await execute('stage4_complex', 'ADD', values); const id = response.response.id;
        const saved = row('stage4_complex', id);
        assert.equal(saved.payload, payload); assert.equal(saved.empty_text, ''); assert.equal(saved.nullable_text, null); assert.equal(saved.default_text, '数据库默认🙂');
        await closeSuccess('ADD', saved); await reopened('stage4_complex', id, saved);
        await edit({ empty_text: 'patched' }); await execute('stage4_complex', 'MODIFY', { empty_text: 'patched' }, id);
        assert.deepEqual(row('stage4_complex', id), { ...saved, empty_text: 'patched' });
        await closeSuccess('MODIFY'); await reopened('stage4_complex', id, { ...saved, empty_text: 'patched' });
        results.push({ id, payload, sqlNull: sql(`SELECT payload IS NULL FROM stage4_complex WHERE id=${id};`) });
      }
      return { results };
    });
    for (const id of ['9007199254740993', '9223372036854775808', '18446744073709551614']) await run(`uint64 auto ID ${id} survives browser create, query, release MODIFY and reopen`, async () => {
      sql(`ALTER TABLE stage4_ids AUTO_INCREMENT=${id};`);
      await managed('stage4_ids'); await button('新增记录').click(); const values = { label: `id-${id}` }; await edit(values);
      const response = await execute('stage4_ids', 'ADD', values);
      assert.equal(response.response.id, id);
      assert.equal(sql(`SELECT CONCAT(id,'|',label) FROM stage4_ids WHERE id=${id};`), `${id}|${values.label}`);
      await closeSuccess('ADD'); await reopened('stage4_ids', id, values);
      const changed = { label: '修改🙂' }; await edit(changed); await execute('stage4_ids', 'MODIFY', changed, id);
      assert.equal(sql(`SELECT label FROM stage4_ids WHERE id=${id};`), changed.label);
      await closeSuccess('MODIFY'); await reopened('stage4_ids', id, changed);
      return { id, sqlCount: sql(`SELECT COUNT(*) FROM stage4_ids WHERE id=${id};`) };
    });
    await run('ambiguous auto ID zero is rejected while omitted ID publishes the actual generated identity', async () => {
      const table = 'stage4_zero_ids';
      const beforeCount = sql(`SELECT COUNT(*) FROM ${table};`);
      const beforeOrders = releaseOrderCount(table);
      await managed(table); await button('新增记录').click(); const ambiguous = { id: '0', label: 'zero-probe' }; await edit(ambiguous);
      const rejected = await execute(table, 'ADD', ambiguous, undefined, 422);
      assert.equal(rejected.stage, 'draft');
      assert.equal(rejected.response.error.code, 'release_auto_id_ambiguous');
      assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), beforeCount);
      assert.equal(releaseOrderCount(table), beforeOrders, 'rejected draft creation must not create a release order');
      await returnToRejectedDraft(rejected, ambiguous);

      await checkbox('包含 id').uncheck();
      const values = { label: 'generated-id' }; await input('label').fill(values.label);
      const response = await execute(table, 'ADD', values);
      const actualID = sql("SELECT CAST(id AS CHAR) FROM stage4_zero_ids WHERE label='generated-id';");
      assert.equal(response.response.id, actualID, `release result must identify SQL row ${actualID}`);
      await closeSuccess('ADD', { id: actualID, label: values.label }); await reopened(table, actualID, { label: values.label });
      await edit({ label: 'generated-patched' }); await execute(table, 'MODIFY', { label: 'generated-patched' }, actualID);
      assert.equal(sql(`SELECT label FROM ${table} WHERE id=${actualID};`), 'generated-patched');
      await closeSuccess('MODIFY', { id: actualID, label: 'generated-patched' });
      return { ambiguousZeroRejected: true, responseID: response.response.id, actualID, sqlMode: sql('SELECT @@session.sql_mode;') };
    });
    for (const [table, expectedID] of [['stage4_default_id_constant', '42'], ['stage4_default_id_expression', '43']]) await run(`non-auto default ID ${table} requires explicit identity before publication`, async () => {
      const beforeOrders = releaseOrderCount(table);
      await managed(table); await button('新增记录').click(); await edit({ label: 'default-id-probe' });
      const rejected = await execute(table, 'ADD', { label: 'default-id-probe' }, undefined, 422);
      assert.equal(rejected.stage, 'draft'); assert.equal(rejected.response.error.code, 'publication_unsupported'); assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0');
      assert.equal(releaseOrderCount(table), beforeOrders, 'rejected draft creation must not create a release order');
      await returnToRejectedDraft(rejected, { label: 'default-id-probe' }); await edit({ id: expectedID });
      const response = await execute(table, 'ADD', { label: 'default-id-probe', id: expectedID });
      assert.equal(sql(`SELECT CAST(id AS CHAR) FROM ${table} WHERE label='default-id-probe';`), expectedID);
      assert.equal(response.response.id, expectedID);
      await closeSuccess('ADD', { id: expectedID, label: 'default-id-probe' }); await reopened(table, expectedID, { label: 'default-id-probe' });
      assert.equal(await input('id').count(), 0, 'MODIFY cannot change the primary key');
      return { actualID: expectedID, responseID: response.response.id, missingIDRejected: true };
    });
    await run('identity review: explicit numeric ID returns the stored canonical identity', async () => {
      const table = 'stage4_default_id_constant';
      await managed(table); await button('新增记录').click(); const values = { id: '00077', label: 'canonical-id' }; await edit(values);
      const response = await execute(table, 'ADD', values);
      const actualID = sql("SELECT CAST(id AS CHAR) FROM stage4_default_id_constant WHERE label='canonical-id';");
      assert.equal(actualID, '77'); assert.equal(response.response.id, actualID, 'ADD must identify the actual SQL row');
      await closeSuccess('ADD', { id: actualID, label: values.label });
      await reopened(table, actualID, { label: values.label });
      return { submittedID: values.id, actualID, responseID: response.response.id };
    });
    for (const item of [
      { name: 'signed integer', type: 'BIGINT', input: '-00077', canonical: '-77' },
      { name: 'decimal equal value', type: 'DECIMAL(6,2)', input: '+001.2', canonical: '1.20' },
      { name: 'decimal rounding', type: 'DECIMAL(6,2)', input: '1.235', canonical: '1.24' },
      { name: 'CHAR trailing spaces', type: 'CHAR(8) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci', input: 'ab  ', canonical: 'ab' },
      { name: 'VARCHAR trailing spaces', type: 'VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci', input: 'ab  ', canonical: 'ab  ' },
      { name: 'VARCHAR Unicode and URI delimiters', type: 'VARCHAR(32)', input: '键/值 %2F?#', canonical: '键/值 %2F?#' },
      { name: 'ENUM case normalization', type: "ENUM('Alpha','Beta')", input: 'alpha', draftRejection: 'release_snapshot_unsupported' },
      { name: 'TIME equal value', type: 'TIME(2)', input: '01:02:03.700000', canonical: '01:02:03.70' },
      { name: 'TIME rounding', type: 'TIME(2)', input: '01:02:03.777', canonical: '01:02:03.78' },
      { name: 'DATETIME equal value', type: 'DATETIME(2)', input: '2026-09-07 12:34:56.700000', canonical: '2026-09-07 12:34:56.70', queryID: '2026-09-07 12:34:56.7', commandValue: '2026-09-07 12:34:56.70' },
      { name: 'DATETIME rounding', type: 'DATETIME(2)', input: '2026-09-07 12:34:59.999', canonical: '2026-09-07 12:35:00.00', queryID: '2026-09-07 12:35:00', commandValue: '2026-09-07 12:35:00.00' },
      { name: 'TIMESTAMP equal value', type: 'TIMESTAMP(2)', input: '2026-09-07T12:34:56.700000Z', canonical: '2026-09-07T12:34:56.70Z', queryID: '2026-09-07T12:34:56.7Z', commandValue: '2026-09-07 12:34:56.70' },
      { name: 'TIMESTAMP rounding', type: 'TIMESTAMP(2)', input: '2026-09-07T12:34:59.999Z', canonical: '2026-09-07T12:35:00.00Z', queryID: '2026-09-07T12:35:00Z', commandValue: '2026-09-07 12:35:00.00' },
      { name: 'FLOAT exact', type: 'FLOAT', input: '0.5', canonical: '0.5' },
      { name: 'FLOAT precision boundary', type: 'FLOAT', input: '0.1', canonical: '0.10000000149011612' },
      { name: 'DOUBLE', type: 'DOUBLE', input: '0.1', canonical: '0.1' },
      { name: 'DATE', type: 'DATE', input: '2024-02-29', canonical: '2024-02-29' },
    ]) await run(`identity matrix: ${item.name} ${item.draftRejection ? 'is rejected before draft creation' : 'publishes a canonical stored key'}`, async () => {
      const sqlMode = sql('SELECT @@session.sql_mode;');
      if (item.name === 'TIME rounding' && sqlMode.split(',').includes('TIME_TRUNCATE_FRACTIONAL')) item.canonical = '01:02:03.77';
      const table = 'stage4_explicit_ids'; sql(`DROP TABLE ${table}; CREATE TABLE ${table}(id ${item.type} PRIMARY KEY, label VARCHAR(64));`);
      const beforeOrders = releaseOrderCount(table);
      await managed(table); await button('新增记录').click(); const values = { id: item.input, label: item.name }; await edit(values);
      const response = await execute(table, 'ADD', values, undefined, item.draftRejection ? 422 : 200);
      if (item.draftRejection) {
        assert.equal(response.stage, 'draft');
        assert.equal(response.response.error.code, item.draftRejection);
        assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0', 'draft capability rejection must not write a row');
        assert.equal(releaseOrderCount(table), beforeOrders, 'draft capability rejection must not create a release order');
        await returnToRejectedDraft(response, values);
        await button('取消').click();
        await page.getByRole('button', { name: '放弃修改并离开', exact: true }).click();
        return { ...item, status: response.status, rows: '0', ordersCreated: '0', retainedInputReviewed: true, failureStage: response.stage };
      }
      const publishedID = item.canonical;
      const queryID = item.queryID ?? publishedID;
      assert.equal(response.response.id, publishedID);
      assert.equal(currentOrder.publication.commands[0].id, publishedID, 'publication command must use the canonical published identity');
      await closeSuccess('ADD', { id: publishedID, label: item.name }, { id: item.commandValue ?? publishedID, label: item.name });
      const sqlID = sql(`SELECT JSON_QUOTE(CAST(id AS CHAR)) FROM ${table};`);
      const query = { conditions: [{ field: 'id', operator: 'exact', value: publishedID }], page_number: 1, page_size: 1 };
      const queried = await api(context, 'POST', `/api/v1/tables/${table}/query`, query); assert.equal(queried.response.rows.length, 1); assert.equal(queried.response.rows[0].id, queryID);
      await reopened(table, queryID, { label: item.name });
      await edit({ label: 'canonical-patched' });
      const modified = await execute(table, 'MODIFY', { label: 'canonical-patched' }, queryID);
      assert.equal(currentOrder.items[0].id, queryID, 'MODIFY draft must retain the query/editor identity');
      assert.equal(modified.response.id, publishedID);
      assert.equal(currentOrder.publication.commands[0].id, publishedID, 'MODIFY command must return the canonical published identity');
      await closeSuccess('MODIFY', { id: publishedID, label: 'canonical-patched' }, { id: item.commandValue ?? publishedID, label: 'canonical-patched' });
      const final = await api(context, 'POST', `/api/v1/tables/${table}/query`, query); assert.equal(final.response.rows.length, 1); assert.equal(final.response.rows[0].id, queryID); assert.equal(final.response.rows[0].label, 'canonical-patched'); assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '1');
      return { ...item, publishedID, queryID, responseID: response.response.id, sqlID, sqlMode, exactQueryAndReleaseModify: true };
    });
    await run('identity preflight read denial prevents INSERT and preserves the draft', async () => {
      const table = 'stage4_explicit_ids';
      sql(`DROP TABLE ${table}; CREATE TABLE ${table}(id BIGINT PRIMARY KEY, label VARCHAR(64));`);
      await managed(table); await button('新增记录').click(); await edit({ id: '88', label: 'readback-denied' });
      assert.equal(sql('SELECT CURRENT_USER();'), 'rcc_admin@%');
      const grantsBefore = sql('SHOW GRANTS;').split('\n').sort();
      const originalLog = sqlAsRoot('SELECT @@GLOBAL.general_log;');
      const originalOutput = sqlAsRoot('SELECT @@GLOBAL.log_output;');
      assert.match(originalLog, /^[01]$/); assert.match(originalOutput, /^(?:FILE|TABLE|NONE)(?:,(?:FILE|TABLE))*$/);
      const catalogs = [
        'rcc_query_policies', 'rcc_mutation_policies', 'rcc_table_policies',
        'rcc_accounts', 'rcc_login_sessions', 'rcc_auth_control_lock', 'rcc_preauth_credentials', 'rcc_auth_rate_limits',
        'rcc_record_versions', 'rcc_release_orders', 'rcc_release_requests', 'rcc_release_targets',
        'rcc_table_publications', 'rcc_publication_commands', 'rcc_refresh_notifications',
      ];
      let details;
      const renewIdleConnections = () => {
        const ids = sqlAsRoot("SELECT ID FROM information_schema.PROCESSLIST WHERE USER='rcc_admin' AND COMMAND='Sleep';").split('\n').filter(Boolean);
        for (const id of ids) { assert.match(id, /^\d+$/); sqlAsRoot(`KILL CONNECTION ${id};`); }
        return ids;
      };
      const warmCatalog = async () => {
        const statuses = [];
        // Discard each killed pool entry through safe reads; retries here are
        // fixture preparation, never mutation replay or product behavior.
        for (let attempt = 0; attempt < 8; attempt++) {
          const response = await page.request.get(`${base}/api/v1/query-policies`);
          statuses.push(response.status());
          if (response.status() === 200) return statuses;
          assert.equal(response.status(), 503);
        }
        assert.fail(`Admin did not renew the killed pool entries: ${statuses}`);
      };
      try {
        // This container belongs only to this acceptance run. Preserve catalog
        // reads and INSERT, but deny the primary-key preflight SELECT.
        sqlAsRoot("REVOKE SELECT ON rcc.* FROM 'rcc_admin'@'%';" + catalogs.map(name => `GRANT SELECT ON rcc.${name} TO 'rcc_admin'@'%';`).join(''));
        const renewedConnections = renewIdleConnections();
        let denied = '';
        try { sql(`SELECT id FROM ${table};`); } catch (error) { denied = String(error.stderr || ''); }
        assert.match(denied, /ERROR 1142/, 'a fresh application-user session cannot SELECT the row table');
        sqlAsRoot("SET GLOBAL log_output='TABLE'; SET GLOBAL general_log=ON;");
        // A killed pooled connection can fail BEGIN before the write starts.
        // Warm the Admin pool with an allowed, read-only catalog request first.
        const catalogWarmupStatuses = await warmCatalog();
        const response = await execute(table, 'ADD', { id: '88', label: 'readback-denied' }, undefined, 503);
        assert.equal(response.stage, 'draft');
        assert.equal(response.response.error.code, 'release_unavailable');
        const rows = sqlAsRoot(`SELECT COUNT(*) FROM rcc.${table};`); assert.equal(rows, '0');
        const trace = sqlAsRoot("SELECT JSON_OBJECT('thread',thread_id,'command',command_type,'statement',CONVERT(argument USING utf8mb4)) FROM mysql.general_log WHERE user_host LIKE 'rcc_admin[%' ORDER BY event_time;").split('\n').filter(Boolean).map(JSON.parse)
          // Session queries bind token hashes. They are not evidence for this
          // business-table fault and must not enter persisted SQL traces.
          .filter(entry => !/\brcc_(accounts|login_sessions|preauth_credentials|auth_rate_limits|auth_control_lock)\b/i.test(entry.statement));
        await fs.writeFile(`${output}/identity-read-failure-trace.json`, JSON.stringify(trace, null, 2));
        // MySQL may reject SELECT during privilege checks before logging that
        // statement. Prove the transaction ran and ended without any INSERT.
        assert.ok(trace.some(entry => /^START TRANSACTION$/i.test(entry.statement)));
        assert.ok(trace.some(entry => /^ROLLBACK$/i.test(entry.statement)));
        assert.equal(trace.some(entry => /INSERT INTO\s+`?stage4_explicit_ids`?/i.test(entry.statement)), false, 'no INSERT is sent after the preflight fails');
        details = { status: response.status, requestId: response.requestId, rows, statements: trace, insertSent: false, renewedConnections, catalogWarmupStatuses, permissionError: denied };
      } finally {
        sqlAsRoot(`SET GLOBAL general_log=${originalLog}; SET GLOBAL log_output='${originalOutput}';`);
        sqlAsRoot("GRANT SELECT ON rcc.* TO 'rcc_admin'@'%';" + catalogs.map(name => `REVOKE SELECT ON rcc.${name} FROM 'rcc_admin'@'%';`).join(''));
        renewIdleConnections();
        await warmCatalog();
      }
      assert.deepEqual(sql('SHOW GRANTS;').split('\n').sort(), grantsBefore, 'restore the isolated user permissions');
      await page.getByText('草稿保存结果待确认。原请求已保留，刷新后仍可找回。', { exact: true }).waitFor();
      const original = http.at(-1);
      const retriedResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/release-orders' && response.request().method() === 'POST');
      await button('使用原请求重试').click();
      const retried = await record(await retriedResponse);
      assert.equal(retried.status, 201, JSON.stringify(retried));
      assert.deepEqual(retried.body, original.body);
      assert.equal(retried.idempotencyKey, original.idempotencyKey, 'draft recovery must reuse the original request key');
      assert.deepEqual(retried.response.items[0].content, { id: '88', label: 'readback-denied' });
      await api(context, 'POST', `/api/v1/release-orders/${retried.response.id}/cancel`, { expected_version: retried.response.version, reason: 'fault injection complete' });
      assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0');
      return { ...details, permissionsRestored: true, originalDraftRequestRecovered: true, recoveredOrderID: retried.response.id };
    });
    await run('publication capability gate rejects a nontransactional table before business DML', async () => {
      const table = 'stage4_explicit_ids';
      sql(`DROP TABLE ${table}; CREATE TABLE ${table}(id DECIMAL(6,2) PRIMARY KEY, label VARCHAR(64)) ENGINE=MyISAM;`);
      const values = { id: '1.235', label: 'nontransactional' };
      const response = await api(context, 'POST', '/api/v1/release-orders', { title: `${table} capability check`, table_name: table, items: [{ operation: 'ADD', content: values }] }, 422);
      assert.equal(response.response.error.code, 'incompatible_table');
      assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0', 'a known rejection must leave no persisted row');
      return { engine: 'MyISAM', status: response.status, code: response.response.error.code, rows: '0' };
    });
    for (const id of ['', '.', '..', '键/值 %2F?#']) await run(`JSON identity ${JSON.stringify(id)} publishes without path reinterpretation`, async () => {
      const table = 'stage4_explicit_ids';
      sql(`DROP TABLE ${table}; CREATE TABLE ${table}(id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci PRIMARY KEY, label VARCHAR(64)) ENGINE=InnoDB;`);
      const values = { id, label: `json-id-${id || 'empty'}` };
      await managed(table); await button('新增记录').click(); await edit(values);
      const response = await execute(table, 'ADD', values);
      assert.equal(response.response.id, id);
      const query = await api(context, 'POST', `/api/v1/tables/${table}/query`, { conditions: [{ field: 'id', operator: 'exact', value: id }], page_number: 1, page_size: 1 });
      assert.equal(query.response.rows.length, 1);
      assert.equal(query.response.rows[0].id, id);
      assert.equal(query.response.rows[0].label, values.label);
      assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '1');
      return { id, responseID: response.response.id, requestTransport: 'release item JSON', rows: '1' };
    });
    await run('CRLF and standalone CR stay protected through unrelated writes and explicit normalization', async () => {
      const note = 'first\r\nsecond\rthird';
      sql(`INSERT INTO stage4_complex(note) VALUES (${literal(note)});`);
      const id = sql('SELECT CAST(MAX(id) AS CHAR) FROM stage4_complex;');
      await managed('stage4_complex'); await button(`修改记录 ${id}`).click();
      assert.equal(await input('note').getAttribute('readonly'), '');
      assert.equal(await input('note').inputValue(), 'first\nsecond\nthird', 'DOM display is normalized, raw draft is protected');
      await edit({ empty_text: 'only other field' }); await execute('stage4_complex', 'MODIFY', { empty_text: 'only other field' }, id);
      assert.equal(sql(`SELECT HEX(note) FROM stage4_complex WHERE id=${id};`), hex(note).toUpperCase());
      await closeSuccess('MODIFY'); await managed('stage4_complex'); await button(`修改记录 ${id}`).click();
      await execute('stage4_complex', 'MODIFY', { note }, id);
      assert.equal(sql(`SELECT HEX(note) FROM stage4_complex WHERE id=${id};`), hex(note).toUpperCase());
      await closeSuccess('MODIFY'); await managed('stage4_complex'); await button(`修改记录 ${id}`).click();
      await page.getByRole('dialog').getByRole('button',{name:'note 值：转换为 LF 再编辑',exact:true}).click();
      assert.equal(await input('note').getAttribute('readonly'), null);
      const normalized = note.replace(/\r\n?/g, '\n') + '\nexplicit edit';
      await input('note').fill(normalized); await execute('stage4_complex', 'MODIFY', { note: normalized }, id);
      assert.equal(sql(`SELECT HEX(note) FROM stage4_complex WHERE id=${id};`), hex(normalized).toUpperCase());
      return { id, originalHex: hex(note), afterExplicitConversionHex: hex(normalized) };
    });
    await run('raw CR clipboard selection is preserved in record and query inputs before conversion', async () => {
      const clipboard = '第一行\r\nsecond\r末尾';
      const combined = `prefix ${clipboard} suffix`;
      // Dispatch an actual ClipboardEvent with raw text in Chromium. This tests
      // the component's browser paste boundary without reading the OS clipboard.
      async function paste(control) {
        await control.fill('prefix OLD suffix');
        await control.evaluate((node, text) => {
          node.focus(); node.setSelectionRange(7, 10);
          const clipboardData = new DataTransfer(); clipboardData.setData('text/plain', text);
          node.dispatchEvent(new ClipboardEvent('paste', { clipboardData, bubbles: true, cancelable: true }));
        }, clipboard);
        assert.equal(await control.getAttribute('readonly'), '');
      }
      await managed('stage4_complex'); await button('新增记录').click(); await checkbox('包含 note').check(); await paste(input('note'));
      const added = await execute('stage4_complex', 'ADD', { note: combined }); const id = added.response.id;
      assert.equal(sql(`SELECT HEX(note) FROM stage4_complex WHERE id=${id};`), hex(combined).toUpperCase());
      await closeSuccess('ADD'); await managed('stage4_complex');
      const queryInput = page.getByRole('textbox', { name: '筛选 note 值', exact: true }); await paste(queryInput);
      let pending = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/tables/stage4_complex/query');
      await button('查询').click(); const exact = await record(await pending);
      assert.equal(exact.body.conditions[0].value, combined); assert.equal(exact.response.rows.length, 1); assert.equal(exact.response.rows[0].id, id);
      await button('筛选 note 值：转换为 LF 再编辑').click();
      const normalized = combined.replace(/\r\n?/g, '\n'); assert.equal(await queryInput.inputValue(), normalized);
      pending = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/tables/stage4_complex/query');
      await button('查询').click(); const converted = await record(await pending);
      assert.equal(converted.body.conditions[0].value, normalized); assert.equal(converted.response.rows.length, 0);
      return { id, clipboardEvent: 'Chromium DataTransfer + ClipboardEvent fixture; no OS clipboard access', rawHex: hex(combined), convertedMatches: 0 };
    });
    await run('ordinary expression defaults allow explicit values and omission defaults', async () => {
      const metadata = sql("SELECT CONCAT_WS('|',COLUMN_NAME,EXTRA,GENERATION_EXPRESSION) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='stage4_defaults';");
      await managed('stage4_defaults'); await button('新增记录').click();
      const values = { label: 'explicit', stamp: '2026-01-02 03:04:05.123450', expression_value: '显式🙂' }; await edit(values);
      const displayedValues = { ...values, stamp: '2026-01-02 03:04:05.12345' };
      const response = await execute('stage4_defaults', 'ADD', values); const id = response.response.id;
      assert.equal(sql(`SELECT CONCAT_WS('|',label,DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f'),expression_value) FROM stage4_defaults WHERE id=${id};`), 'explicit|2026-01-02 03:04:05.123450|显式🙂');
      await closeSuccess('ADD', { id, ...displayedValues }, { id, ...values }); await reopened('stage4_defaults', id, displayedValues);
      const changed = { stamp: '2027-02-03 04:05:06.654321', expression_value: '修改表达式默认字段🙂' };
      await edit(changed); await execute('stage4_defaults', 'MODIFY', changed, id);
      assert.equal(sql(`SELECT CONCAT_WS('|',label,DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f'),expression_value) FROM stage4_defaults WHERE id=${id};`), `explicit|${changed.stamp}|${changed.expression_value}`);
      await closeSuccess('MODIFY', { id, ...values, ...changed }); await reopened('stage4_defaults', id, { ...values, ...changed });
      await managed('stage4_defaults'); await button('新增记录').click(); await edit({ label: 'omitted' });
      const omitted = await execute('stage4_defaults', 'ADD', { label: 'omitted' });
      assert.equal(sql(`SELECT CONCAT(expression_value,'|',stamp IS NOT NULL) FROM stage4_defaults WHERE id=${omitted.response.id};`), '表达式🙂|1');
      const defaultStamp = sql(`SELECT DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f') FROM stage4_defaults WHERE id=${omitted.response.id};`);
      const defaultStampDisplay = defaultStamp.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');
      const defaulted = { id: omitted.response.id, label: 'omitted', stamp: defaultStampDisplay, expression_value: '表达式🙂' };
      const defaultedCommand = { ...defaulted, stamp: defaultStamp };
      await closeSuccess('ADD', defaulted, defaultedCommand); await reopened('stage4_defaults', omitted.response.id, defaulted);
      await edit({ label: 'defaults-retained' }); await execute('stage4_defaults', 'MODIFY', { label: 'defaults-retained' }, omitted.response.id);
      assert.equal(sql(`SELECT CONCAT_WS('|',label,DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f'),expression_value) FROM stage4_defaults WHERE id=${omitted.response.id};`), `defaults-retained|${defaultStamp}|表达式🙂`);
      await closeSuccess('MODIFY', { ...defaulted, label: 'defaults-retained' }, { ...defaultedCommand, label: 'defaults-retained' });
      return { metadata, explicit: id, omitted: omitted.response.id, defaulted, defaultStampCommand: defaultStamp };
    });
    await run('Auto Fill records the publisher identity and one UTC database time', async () => {
      await managed('stage4_auto'); await button('新增记录').click();
      for (const field of ['created_by', 'created_at', 'updated_by', 'updated_at']) assert.equal(await input(field).count(), 0);
      const lower = sql("SELECT DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%d %H:%i:%s.%f');");
      await edit({ label: 'auto' }); const response = await execute('stage4_auto', 'ADD', { label: 'auto' }); const id = response.response.id;
      const stamps = sql(`SELECT CONCAT(DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f'),'|',DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s.%f')) FROM stage4_auto WHERE id=${id};`).split('|');
      assert.equal(stamps[0], stamps[1]); assert.ok(stamps[0] >= lower);
      assert.ok(stamps[0] <= sql("SELECT DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%d %H:%i:%s.%f');"));
      assert.equal(sql(`SELECT CONCAT(created_by,'|',updated_by) FROM stage4_auto WHERE id=${id};`), `${publisherIdentity.accountID}|${publisherIdentity.accountID}`);
      await closeSuccess('ADD'); await managed('stage4_auto'); await button(`修改记录 ${id}`).click(); await edit({ label: 'auto-modified' }); await execute('stage4_auto', 'MODIFY', { label: 'auto-modified' }, id);
      const after = sql(`SELECT CONCAT(DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f'),'|',DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s.%f')) FROM stage4_auto WHERE id=${id};`).split('|');
      assert.equal(after[0], stamps[0]); assert.ok(after[1] > stamps[1]);
      assert.equal(sql(`SELECT CONCAT(created_by,'|',updated_by) FROM stage4_auto WHERE id=${id};`), `${publisherIdentity.accountID}|${publisherIdentity.accountID}`);
      return { id, publisherID: publisherIdentity.accountID, stamps, after };
    });
    for (const field of ['stored_value', 'virtual_value']) await run(`${field} remains rejected as a real generated column`, async () => {
      await managed('stage4_generated'); await button('新增记录').click(); const values = { base_value: '9', [field]: '123' }; await edit(values);
      const response = await execute('stage4_generated', 'ADD', values, undefined, 422);
      assert.equal(response.response.error.code, 'invalid_mutation_content'); assert.equal(sql('SELECT COUNT(*) FROM stage4_generated;'), '0');
      return { metadata: sql("SELECT CONCAT_WS('|',COLUMN_NAME,EXTRA,GENERATION_EXPRESSION) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='stage4_generated';") };
    });
    await run('generated fields calculate on omitted ADD and stay unwritable on release MODIFY', async () => {
      await managed('stage4_generated'); await button('新增记录').click(); await edit({ base_value: '9' });
      const created = await execute('stage4_generated', 'ADD', { base_value: '9' }); const id = created.response.id;
      assert.equal(sql(`SELECT CONCAT_WS('|',base_value,stored_value,virtual_value) FROM stage4_generated WHERE id=${id};`), '9|18|10');
      await closeSuccess('ADD');
      for (const field of ['stored_value', 'virtual_value']) {
        await managed('stage4_generated'); await button(`修改记录 ${id}`).click();
        assert.equal(await input(field).count(), 0, 'generated columns must be excluded from the MODIFY editor');
        const current = (await api(context, 'POST', '/api/v1/tables/stage4_generated/query', { conditions: [{ field: 'id', operator: 'exact', value: id }] })).response;
        const rejected = await api(context, 'POST', '/api/v1/release-orders', { title: 'Reject generated column input', table_name: 'stage4_generated', items: [{ operation: 'MODIFY', id, expected_record_version: current.record_versions[0], content: { [field]: '123' } }] }, 422);
        assert.equal(rejected.response.error.code, 'invalid_mutation_content');
        assert.equal(sql(`SELECT CONCAT_WS('|',base_value,stored_value,virtual_value) FROM stage4_generated WHERE id=${id};`), '9|18|10');
      }
      await managed('stage4_generated'); await button(`修改记录 ${id}`).click(); await edit({ base_value: '11' }); await execute('stage4_generated', 'MODIFY', { base_value: '11' }, id);
      assert.equal(sql(`SELECT CONCAT_WS('|',base_value,stored_value,virtual_value) FROM stage4_generated WHERE id=${id};`), '11|22|12');
      return { id, before: '9|18|10', after: '11|22|12' };
    });
    for (const [name, values, fixed] of [['VARCHAR length', { label: '12345' }, { label: '1234' }], ['TINYINT range', { small_value: '128' }, { small_value: '127' }]]) await run(`${name} is editable rejection with rollback and corrected browser retry`, async () => {
      const beforeCount = sql('SELECT COUNT(*) FROM stage4_limits;');
      try { sql(`INSERT INTO stage4_limits (${Object.keys(values)[0]}) VALUES (${literal(Object.values(values)[0])});`); assert.fail('database must reject invalid value'); } catch (error) {
        const diagnostic = String(error.stderr || ''); assert.match(diagnostic, /ERROR (1406|1264)/); sqlErrors.push({ case: name, diagnostic });
      }
      await managed('stage4_limits'); await button('新增记录').click(); await edit(values);
      const response = await execute('stage4_limits', 'ADD', values, undefined, 422);
      assert.equal(response.stage, 'publication', 'real MySQL rejection must occur after draft, submit and independent approval');
      assert.equal(response.response.error.code, 'invalid_mutation_content'); assert.equal(sql('SELECT COUNT(*) FROM stage4_limits;'), beforeCount);
      assert.deepEqual(response.order.items[0].content, values, 'approved input remains readable after publication rollback');
      assert.equal(response.order.history.some(event => event.action === 'EXECUTE'), false, 'failed publication cannot append a successful execute event');
      await api(context, 'POST', `/api/v1/release-orders/${response.order.id}/cancel`, { expected_version: response.order.version, reason: 'correct invalid storage value in a new release' });
      await managed('stage4_limits'); await button('新增记录').click();
      await edit(fixed); const retried = await execute('stage4_limits', 'ADD', fixed); assert.ok(retried.response.id);
      assert.equal(sql('SELECT COUNT(*) FROM stage4_limits;'), String(Number(beforeCount) + 1));
      assert.equal(sql(`SELECT ${Object.keys(fixed)[0]} FROM stage4_limits WHERE id=${retried.response.id};`), Object.values(fixed)[0]);
      return { rejectedRequestId: response.requestId, retryRequestId: retried.requestId, failedOrderID: response.order.id, failureStage: response.stage, retainedApprovedInput: true };
    });
  } catch (error) { failures.push({ case: 'setup or harness', message: error.message, stack: error.stack }); }
  finally {
    if (browser) await browser.close().catch(() => {});
    try {
      sql(`DELETE FROM rcc_table_policies WHERE table_name IN (${tables.map((t) => `'${t}'`).join(',')}); DELETE FROM rcc_mutation_policies WHERE code IN ('stage4_plain_v1','stage4_auto_v1'); ${tables.map((t) => `DROP TABLE IF EXISTS ${t};`).join('\n')}`);
      cleanup = { fixtureRowsEqual: before === fixtureRows(), remainingTables: sql(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN (${tables.map((t) => `'${t}'`).join(',')});`), remainingTablePolicies: sql(`SELECT COUNT(*) FROM rcc_table_policies WHERE table_name IN (${tables.map((t) => `'${t}'`).join(',')});`), remainingPolicies: sql("SELECT COUNT(*) FROM rcc_mutation_policies WHERE code IN ('stage4_plain_v1','stage4_auto_v1');") };
      assert.deepEqual(cleanup, { fixtureRowsEqual: true, remainingTables: '0', remainingTablePolicies: '0', remainingPolicies: '0' });
    } catch (error) { failures.push({ case: 'cleanup', message: error.message }); }
    if (evidence.length + failures.length === 0) failures.push({ case: 'empty selection', message: 'No complex-fields case was selected' });
    if (pageErrors.length) failures.push({ case: 'page errors', pageErrors });
    await fs.writeFile(`${output}/http-evidence.json`, JSON.stringify(http, null, 2) + '\n');
    await fs.writeFile(`${output}/result.json`, JSON.stringify({ ok: failures.length === 0, browser: browserVersion, evidence, failures, sqlErrors, pageErrors, cleanup }, null, 2) + '\n');
    if (failures.length) process.exitCode = 1;
  }
})();
