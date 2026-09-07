// Real Chromium -> production same-origin Web proxy -> Cookie-authenticated Admin -> disposable MySQL 8.4.
// SQL arranges this suite's tables and independently verifies bytes and database semantics.
const { chromium } = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const { registerFixtureAccount, authenticatedRequest } = require('./local-account.cjs');
const fs = require('node:fs/promises');
const { execFileSync } = require('node:child_process');
const base = process.env.RCC_WEB_URL;
const output = process.env.RCC_E2E_OUTPUT;
const container = process.env.RCC_E2E_MYSQL_CONTAINER;
assert.match(container || '', /^rcc-browser-\d+-\d+-mysql$/);
const tables = ['stage4_complex', 'stage4_ids', 'stage4_defaults', 'stage4_auto', 'stage4_generated', 'stage4_limits', 'stage4_zero_ids', 'stage4_default_id_constant', 'stage4_default_id_expression', 'stage4_explicit_ids'];
const hex = (value) => Buffer.from(value, 'utf8').toString('hex');
const literal = (value) => `CONVERT(0x${hex(value)} USING utf8mb4)`;
const sql = (statement) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c', 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --default-character-set=utf8mb4 --raw --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE"'], { input: statement, encoding: 'utf8', timeout: 20000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
const sqlAsRoot = (statement) => execFileSync('docker', ['exec', '-i', container, 'sh', '-c', 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --default-character-set=utf8mb4 --raw --batch --skip-column-names -uroot "$MYSQL_DATABASE"'], { input: statement, encoding: 'utf8', timeout: 20000, stdio: ['pipe', 'pipe', 'pipe'] }).trim();
const row = (table, id) => JSON.parse(sql(`SELECT JSON_OBJECT(${Object.entries({id:'CAST(id AS CHAR)',note:'note',payload:'CAST(payload AS CHAR)',nullable_text:'nullable_text',empty_text:'empty_text',default_text:'default_text',big_signed:'CAST(big_signed AS CHAR)',big_unsigned:'CAST(big_unsigned AS CHAR)',amount:'CAST(amount AS CHAR)',day:'CAST(day AS CHAR)',clock:'CAST(clock AS CHAR)',local_time:'CAST(local_time AS CHAR)',instant:"CONCAT(DATE_FORMAT(instant,'%Y-%m-%dT%H:%i:%s.%f'),'Z')"}).map(([key, expression]) => `'${key}',${expression}`).join(',')}) FROM ${table} WHERE id=${id};`));
const fixtureRows = () => sql("SELECT CONCAT_WS('|',id,HEX(name),IF(category IS NULL,'NULL',HEX(category)),IF(note IS NULL,'NULL',HEX(note)),HEX(state),priority,HEX(created_by),DATE_FORMAT(created_at,'%Y%m%d%H%i%s.%f'),HEX(updated_by),DATE_FORMAT(updated_at,'%Y%m%d%H%i%s.%f')) FROM stage1_acceptance_items ORDER BY id;");
const matrix = [
  ['text/json', 'Chinese, emoji, LF, TAB, surrounding spaces and long text', 'exact submitted bytes for TEXT; MySQL canonical JSON, including integer > 2^53'],
  ['null/empty/omitted', 'SQL NULL, JSON literal null, JSON string "null", empty string and omitted ADD/PATCH', 'distinct values; default on ADD, retain on PATCH'],
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
  CREATE TABLE stage4_auto (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(64), created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6), updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6)) CHARACTER SET utf8mb4;
  CREATE TABLE stage4_generated (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, base_value INT DEFAULT 7, stored_value INT GENERATED ALWAYS AS (base_value*2) STORED, virtual_value INT GENERATED ALWAYS AS (base_value+1) VIRTUAL);
  CREATE TABLE stage4_zero_ids (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(64));
  CREATE TABLE stage4_default_id_constant (id BIGINT UNSIGNED PRIMARY KEY DEFAULT 42, label VARCHAR(64));
  CREATE TABLE stage4_default_id_expression (id BIGINT UNSIGNED PRIMARY KEY DEFAULT (40+3), label VARCHAR(64));
  CREATE TABLE stage4_explicit_ids (id BIGINT PRIMARY KEY, label VARCHAR(64));
  CREATE TABLE stage4_limits (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, label VARCHAR(4), small_value TINYINT);
  INSERT INTO rcc_mutation_policies(code,name,description,type_code,allow_add,allow_modify,allow_delete,status,creator,modifier) VALUES ('stage4_plain_v1','Stage 4 plain','','single_table_mutation',1,1,1,'ACTIVE','fixture','fixture');
  INSERT INTO rcc_mutation_policies(code,name,description,type_code,allow_add,allow_modify,allow_delete,create_time_field,modify_time_field,status,creator,modifier) VALUES ('stage4_auto_v1','Stage 4 auto','','single_table_mutation',1,1,1,'created_at','updated_at','ACTIVE','fixture','fixture');
  INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES ${tables.map((table) => `('${table}','stage1_query_v1','${table === 'stage4_auto' ? 'stage4_auto_v1' : 'stage4_plain_v1'}',1,'fixture','fixture')`).join(',')};`;
}
(async () => {
  await fs.mkdir(output, { recursive: true });
  await fs.writeFile(`${output}/semantic-matrix.json`, JSON.stringify(matrix, null, 2));
  const before = fixtureRows();
  const http = [], evidence = [], failures = [], pageErrors = [];
  const previewDifferences = [];
  const sqlErrors = [];
  let browser, context, page, cleanup, browserVersion;
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
    const item = { method: request.method(), path: new URL(response.url()).pathname, status: response.status(), requestId: response.headers()['x-request-id'], body: request.postDataJSON(), response: responseBody };
    http.push(item);
    return item;
  }
  async function edit(values) {
    for (const [name, value] of Object.entries(values)) {
      await checkbox(`包含 ${name}`).check();
      if (value === null) await checkbox(`${name} 使用 NULL`).check();
      else {
        if (await checkbox(`${name} 使用 NULL`).isChecked()) await checkbox(`${name} 使用 NULL`).uncheck();
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
  async function execute(table, operation, values, id, status = operation === 'ADD' ? 201 : 200) {
    await preview(operation, values);
    const path = `/api/v1/tables/${table}/rows${id ? `/${id}` : ''}`;
    const pending = page.waitForResponse((r) => new URL(r.url()).pathname === path && r.request().method() === (operation === 'ADD' ? 'POST' : 'PATCH'));
    await button('确认并执行').click();
    const result = await record(await pending);
    assert.deepEqual(result.body.content, values, 'HTTP content must preserve included values and omissions');
    assert.deepEqual(previewDifferences, [], 'Change Set must preserve exact values');
    assert.ok(result.requestId);
    assert.equal(result.status, status, JSON.stringify(result));
    return result;
  }
  async function closeSuccess(operation, expected) {
    const dialog = page.getByRole('dialog', { name: `${operation} 写入结果`, exact: true });
    await dialog.waitFor();
    if (expected) for (const [name, value] of Object.entries(expected)) {
      const entry = dialog.locator('dl > div').filter({ has: page.locator('dt', { hasText: new RegExp(`^${name}$`) }) });
      assert.equal(await entry.locator('dd').textContent(), value === null ? 'NULL' : value === '' ? '空字符串' : value, `${name} exact-id readback`);
    }
    await dialog.getByRole('button', { name: '关闭', exact: true }).click();
    await dialog.waitFor({ state: 'detached' });
  }
  async function reopened(table, id, expected) {
    await managed(table);
    await button(`修改记录 ${id}`).click();
    for (const [name, value] of Object.entries(expected)) {
      if (name === 'id') continue;
      assert.equal(await input(name).inputValue(), value ?? '', `${name} reopened editor`);
      assert.equal(await checkbox(`${name} 使用 NULL`).isChecked(), value === null, `${name} NULL state`);
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
    browser = await chromium.launch({ headless: true }); browserVersion = browser.version();
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, timezoneId: 'America/Los_Angeles' });
    await registerFixtureAccount(context, base);
    await run('TEXT and JSON browser ADD, PATCH, SQL byte comparison, readback and reopened editor', async () => {
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
      await closeSuccess('ADD', expected); await reopened('stage4_complex', id, expected);
      const changed = { note: `${values.note}\n追加🙂\t  `, payload: '{ "large": 18446744073709551615, "text": "修改🙂\\n\\t " }', big_signed: '9223372036854775807', big_unsigned: '0', amount: '-123456789012345678901234567890123456789012345.12345678901234567890', clock: '00:00:00.000001' };
      await edit(changed); await execute('stage4_complex', 'MODIFY', changed, id);
      const modified = { ...expected, ...changed, payload: sql(`SELECT CAST(CAST(${literal(changed.payload)} AS JSON) AS CHAR);`) };
      assert.deepEqual(row('stage4_complex', id), modified);
      await closeSuccess('MODIFY', modified); await reopened('stage4_complex', id, modified);
      return { id, textBytes: Buffer.byteLength(modified.note), sql: modified };
    });
    await run('multiline TEXT query uses the exact browser input without removing LF', async () => {
      const note = '  查询🙂\n下一行\t末尾  ';
      sql(`INSERT INTO stage4_complex(note) VALUES (${literal(note)});`);
      await managed('stage4_complex'); await button('添加条件').click();
      await page.getByRole('combobox', { name: '条件 1 字段' }).selectOption('note');
      await page.getByRole('textbox', { name: '条件 1 值', exact: true }).fill(note);
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
    for (const id of ['9007199254740993', '9223372036854775808', '18446744073709551614']) await run(`uint64 auto ID ${id} survives browser create, query, PATCH and reopen`, async () => {
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
    await run('explicit auto ID zero returns the actual inserted identity through browser ADD and PATCH', async () => {
      const table = 'stage4_zero_ids';
      await managed(table); await button('新增记录').click(); await edit({ label: 'prime-session' });
      const primed = await execute(table, 'ADD', { label: 'prime-session' });
      assert.equal(primed.response.id, '1'); await closeSuccess('ADD');
      await button('新增记录').click(); const values = { id: '0', label: 'zero-probe' }; await edit(values);
      const response = await execute(table, 'ADD', values);
      const actualID = sql("SELECT CAST(id AS CHAR) FROM stage4_zero_ids WHERE label='zero-probe';");
      const sqlMode = sql('SELECT @@session.sql_mode;');
      assert.equal(actualID, sqlMode.split(',').includes('NO_AUTO_VALUE_ON_ZERO') ? '0' : '2');
      assert.equal(response.response.id, actualID, `response must identify SQL row ${actualID}`);
      await closeSuccess('ADD', { id: actualID, label: values.label }); await reopened(table, actualID, { label: values.label });
      await edit({ label: 'zero-patched' }); await execute(table, 'MODIFY', { label: 'zero-patched' }, actualID);
      assert.equal(sql(`SELECT label FROM ${table} WHERE id=${actualID};`), 'zero-patched');
      await closeSuccess('MODIFY', { id: actualID, label: 'zero-patched' });
      return { priorGeneratedID: primed.response.id, responseID: response.response.id, actualID, sqlMode };
    });
    for (const [table, expectedID] of [['stage4_default_id_constant', '42'], ['stage4_default_id_expression', '43']]) await run(`non-auto default ID ${table} requires explicit identity before inserting`, async () => {
      await managed(table); await button('新增记录').click(); await edit({ label: 'default-id-probe' });
      const rejected = await execute(table, 'ADD', { label: 'default-id-probe' }, undefined, 400);
      assert.equal(rejected.response.error.code, 'missing_required_field'); assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0');
      await button('返回修改').click(); await edit({ id: expectedID });
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
      { name: 'decimal rounding', type: 'DECIMAL(6,2)', input: '1.235', rejected: true },
      { name: 'CHAR trailing spaces', type: 'CHAR(8) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci', input: 'ab  ', rejected: true },
      { name: 'VARCHAR trailing spaces', type: 'VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci', input: 'ab  ', canonical: 'ab  ' },
      { name: 'VARCHAR URI delimiters', type: 'VARCHAR(32)', input: '键/值 ?#%', canonical: '键/值 ?#%' },
      { name: 'ENUM case normalization', type: "ENUM('Alpha','Beta')", input: 'alpha', canonical: 'Alpha' },
      { name: 'TIME equal value', type: 'TIME(2)', input: '01:02:03.700000', canonical: '01:02:03.70' },
      { name: 'TIME rounding', type: 'TIME(2)', input: '01:02:03.777', canonical: '01:02:03.78' },
      { name: 'DATETIME equal value', type: 'DATETIME(2)', input: '2026-09-07 12:34:56.700000', canonical: '2026-09-07 12:34:56.7' },
      { name: 'DATETIME rounding', type: 'DATETIME(2)', input: '2026-09-07 12:34:59.999', rejected: true },
      { name: 'TIMESTAMP equal value', type: 'TIMESTAMP(2)', input: '2026-09-07T12:34:56.700000Z', canonical: '2026-09-07T12:34:56.7Z' },
      { name: 'TIMESTAMP rounding', type: 'TIMESTAMP(2)', input: '2026-09-07T12:34:59.999Z', rejected: true },
      { name: 'FLOAT exact', type: 'FLOAT', input: '0.5', canonical: '0.5' },
      { name: 'FLOAT precision boundary', type: 'FLOAT', input: '0.1', rejected: true },
      { name: 'DOUBLE', type: 'DOUBLE', input: '0.1', canonical: '0.1' },
      { name: 'DATE', type: 'DATE', input: '2024-02-29', canonical: '2024-02-29' },
    ]) await run(`identity matrix: ${item.name} keeps a usable stored key or rolls back`, async () => {
      const sqlMode = sql('SELECT @@session.sql_mode;');
      if (item.name === 'TIME rounding' && sqlMode.split(',').includes('TIME_TRUNCATE_FRACTIONAL')) item.canonical = '01:02:03.77';
      const table = 'stage4_explicit_ids'; sql(`DROP TABLE ${table}; CREATE TABLE ${table}(id ${item.type} PRIMARY KEY, label VARCHAR(64));`);
      await managed(table); await button('新增记录').click(); const values = { id: item.input, label: item.name }; await edit(values);
      const response = await execute(table, 'ADD', values, undefined, item.rejected ? 400 : 201);
      if (item.rejected) {
        assert.equal(response.response.error.code, 'invalid_mutation_content'); assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0', 'unlocatable identity must roll back the INSERT');
        await button('返回修改').click(); assert.equal(await input('id').inputValue(), item.input);
        return { ...item, status: response.status, rollbackRows: '0', draftRetained: true };
      }
      assert.equal(response.response.id, item.canonical); await closeSuccess('ADD', { id: item.canonical, label: item.name });
      const sqlID = sql(`SELECT JSON_QUOTE(CAST(id AS CHAR)) FROM ${table};`);
      const query = { conditions: [{ field: 'id', operator: 'exact', value: item.canonical }], page_number: 1, page_size: 1 };
      async function api(method, path, body) {
        const result = await authenticatedRequest(context, base, path, { method, data: body });
        const entry = { method, path, status: result.status(), requestId: result.headers()['x-request-id'], body, response: await result.json(), transport: 'Playwright APIRequest through real same-origin proxy' }; http.push(entry); assert.equal(entry.status, 200, JSON.stringify(entry)); return entry;
      }
      const queried = await api('POST', `/api/v1/tables/${table}/query`, query); assert.equal(queried.response.rows.length, 1); assert.equal(queried.response.rows[0].id, item.canonical);
      await api('PATCH', `/api/v1/tables/${table}/rows/${encodeURIComponent(item.canonical)}`, { content: { label: 'canonical-patched' } });
      const final = await api('POST', `/api/v1/tables/${table}/query`, query); assert.equal(final.response.rows[0].label, 'canonical-patched'); assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '1');
      return { ...item, responseID: response.response.id, sqlID, sqlMode, exactQueryAndPatch: true };
    });
    await run('identity review: response-lost explicit zero ADD checks without guessing a generated ID', async () => {
      const table = 'stage4_zero_ids';
      const before = sql(`SELECT COUNT(*) FROM ${table};`);
      await managed(table); let writes = 0; let actualWrite;
      page.on('request', request => { if (new URL(request.url()).pathname === `/api/v1/tables/${table}/rows` && request.method() === 'POST') writes++; });
      await page.route(`**/api/v1/tables/${table}/rows`, async route => {
        if (route.request().method() !== 'POST') return route.continue();
        const response = await route.fetch();
        actualWrite = { method: 'POST', path: `/api/v1/tables/${table}/rows`, status: response.status(), requestId: response.headers()['x-request-id'], body: route.request().postDataJSON(), response: await response.json(), fault: 'successful response withheld from browser' };
        http.push(actualWrite); await route.abort('failed');
      });
      await button('新增记录').click(); const values = { id: '0', label: 'lost-zero-id' }; await edit(values); await preview('ADD', values); await button('确认并执行').click();
      const recovery = page.getByRole('alert', { name: '提交结果尚未确认', exact: true }); await recovery.waitFor();
      assert.equal(actualWrite.status, 201); const actualID = sql("SELECT CAST(id AS CHAR) FROM stage4_zero_ids WHERE label='lost-zero-id';");
      assert.notEqual(actualID, '0', 'default SQL mode generates the stored identity');
      assert.equal(await button('确认并执行').isDisabled(), true);
      const queryResponse = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/tables/${table}/query`);
      await recovery.getByRole('button', { name: '只读核对当前状态', exact: true }).click(); const checked = await record(await queryResponse);
      assert.deepEqual(checked.body, { conditions: [], page_number: 1 }, 'lost ADD cannot trust submitted zero as the stored ID');
      assert.ok(checked.response.rows.some(row => row.id === actualID && row.label === values.label));
      assert.equal(writes, 1); assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), String(Number(before) + 1));
      assert.equal(await button('确认并执行').isDisabled(), true, 'read-only checking never unlocks a write automatically');
      return { actualWrite, actualID, readonlyQuery: checked.body, writeCount: writes, databaseCountBefore: before, databaseCountAfter: sql(`SELECT COUNT(*) FROM ${table};`) };
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
      const catalogs = ['rcc_query_policies', 'rcc_mutation_policies', 'rcc_table_policies', 'rcc_accounts', 'rcc_login_sessions', 'rcc_auth_control_lock', 'rcc_preauth_credentials', 'rcc_auth_rate_limits'];
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
        assert.equal(response.response.error.code, 'mutation_unavailable');
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
      const recovery = page.getByRole('alert', { name: '提交结果尚未确认', exact: true });
      const checked = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/tables/${table}/query`);
      await recovery.getByRole('button', { name: '只读核对当前状态', exact: true }).click(); const current = await record(await checked);
      assert.deepEqual(current.response.rows, []); assert.equal(await button('确认并执行').isDisabled(), true);
      return { ...details, permissionsRestored: true, recoveryReadOnly: true };
    });
    for (const item of [
      { name: 'nontransactional decimal rounding', type: 'DECIMAL(6,2)', engine: 'MyISAM', input: '1.235', status: 422, code: 'incompatible_table' },
      { name: 'empty string', type: 'VARCHAR(16)', engine: 'InnoDB', input: '', status: 400, code: 'invalid_mutation_content' },
      { name: 'single dot', type: 'VARCHAR(16)', engine: 'InnoDB', input: '.', status: 400, code: 'invalid_mutation_content' },
      { name: 'double dot', type: 'VARCHAR(16)', engine: 'InnoDB', input: '..', status: 400, code: 'invalid_mutation_content' },
    ]) await run(`identity guard: ${item.name} rejects without persisting a row`, async () => {
      const table = 'stage4_explicit_ids';
      sql(`DROP TABLE ${table}; CREATE TABLE ${table}(id ${item.type} PRIMARY KEY, label VARCHAR(64)) ENGINE=${item.engine};`);
      await managed(table); await button('新增记录').click(); const values = { id: item.input, label: item.name }; await edit(values);
      const response = await execute(table, 'ADD', values, undefined, item.status);
      assert.equal(response.response.error.code, item.code);
      assert.equal(sql(`SELECT COUNT(*) FROM ${table};`), '0', 'a known rejection must leave no persisted row');
      await button('返回修改').click(); assert.equal(await input('id').inputValue(), item.input);
      return { ...item, rows: '0', draftRetained: true };
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
      await checkbox('包含 note').check(); await execute('stage4_complex', 'MODIFY', { note }, id);
      assert.equal(sql(`SELECT HEX(note) FROM stage4_complex WHERE id=${id};`), hex(note).toUpperCase());
      await closeSuccess('MODIFY'); await managed('stage4_complex'); await button(`修改记录 ${id}`).click();
      await checkbox('包含 note').check(); await button('note 值：转换为 LF 再编辑').click();
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
      await closeSuccess('ADD'); await managed('stage4_complex'); await button('添加条件').click();
      await page.getByRole('combobox', { name: '条件 1 字段' }).selectOption('note');
      const queryInput = page.getByRole('textbox', { name: '条件 1 值', exact: true }); await paste(queryInput);
      let pending = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/tables/stage4_complex/query');
      await button('查询').click(); const exact = await record(await pending);
      assert.equal(exact.body.conditions[0].value, combined); assert.equal(exact.response.rows.length, 1); assert.equal(exact.response.rows[0].id, id);
      await button('条件 1 值：转换为 LF 再编辑').click();
      const normalized = combined.replace(/\r\n?/g, '\n'); assert.equal(await queryInput.inputValue(), normalized);
      pending = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/tables/stage4_complex/query');
      await button('查询').click(); const converted = await record(await pending);
      assert.equal(converted.body.conditions[0].value, normalized); assert.equal(converted.response.rows.length, 0);
      return { id, clipboardEvent: 'Chromium DataTransfer + ClipboardEvent fixture; no OS clipboard access', rawHex: hex(combined), convertedMatches: 0 };
    });
    await run('ordinary expression defaults allow explicit values and omission defaults', async () => {
      const metadata = sql("SELECT CONCAT_WS('|',COLUMN_NAME,EXTRA,GENERATION_EXPRESSION) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='stage4_defaults';");
      await managed('stage4_defaults'); await button('新增记录').click();
      const values = { label: 'explicit', stamp: '2026-01-02 03:04:05.123456', expression_value: '显式🙂' }; await edit(values);
      const response = await execute('stage4_defaults', 'ADD', values); const id = response.response.id;
      assert.equal(sql(`SELECT CONCAT_WS('|',label,DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f'),expression_value) FROM stage4_defaults WHERE id=${id};`), 'explicit|2026-01-02 03:04:05.123456|显式🙂');
      await closeSuccess('ADD', { id, ...values }); await reopened('stage4_defaults', id, values);
      const changed = { stamp: '2027-02-03 04:05:06.654321', expression_value: '修改表达式默认字段🙂' };
      await edit(changed); await execute('stage4_defaults', 'MODIFY', changed, id);
      assert.equal(sql(`SELECT CONCAT_WS('|',label,DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f'),expression_value) FROM stage4_defaults WHERE id=${id};`), `explicit|${changed.stamp}|${changed.expression_value}`);
      await closeSuccess('MODIFY', { id, ...values, ...changed }); await reopened('stage4_defaults', id, { ...values, ...changed });
      await managed('stage4_defaults'); await button('新增记录').click(); await edit({ label: 'omitted' });
      const omitted = await execute('stage4_defaults', 'ADD', { label: 'omitted' });
      assert.equal(sql(`SELECT CONCAT(expression_value,'|',stamp IS NOT NULL) FROM stage4_defaults WHERE id=${omitted.response.id};`), '表达式🙂|1');
      const defaultStamp = sql(`SELECT DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f') FROM stage4_defaults WHERE id=${omitted.response.id};`);
      const defaulted = { id: omitted.response.id, label: 'omitted', stamp: defaultStamp.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, ''), expression_value: '表达式🙂' };
      await closeSuccess('ADD', defaulted); await reopened('stage4_defaults', omitted.response.id, defaulted);
      await edit({ label: 'defaults-retained' }); await execute('stage4_defaults', 'MODIFY', { label: 'defaults-retained' }, omitted.response.id);
      assert.equal(sql(`SELECT CONCAT_WS('|',label,DATE_FORMAT(stamp,'%Y-%m-%d %H:%i:%s.%f'),expression_value) FROM stage4_defaults WHERE id=${omitted.response.id};`), `defaults-retained|${defaultStamp}|表达式🙂`);
      await closeSuccess('MODIFY', { ...defaulted, label: 'defaults-retained' });
      return { metadata, explicit: id, omitted: omitted.response.id, defaulted };
    });
    await run('Auto Fill writes expression-default audit columns with one UTC database time', async () => {
      await managed('stage4_auto'); await button('新增记录').click(); assert.equal(await input('created_at').count(), 0); assert.equal(await input('updated_at').count(), 0);
      const lower = sql("SELECT DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%d %H:%i:%s.%f');");
      await edit({ label: 'auto' }); const response = await execute('stage4_auto', 'ADD', { label: 'auto' }); const id = response.response.id;
      const stamps = sql(`SELECT CONCAT(DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f'),'|',DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s.%f')) FROM stage4_auto WHERE id=${id};`).split('|');
      assert.equal(stamps[0], stamps[1]); assert.ok(stamps[0] >= lower);
      assert.ok(stamps[0] <= sql("SELECT DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%d %H:%i:%s.%f');"));
      await closeSuccess('ADD'); await managed('stage4_auto'); await button(`修改记录 ${id}`).click(); await edit({ label: 'auto-modified' }); await execute('stage4_auto', 'MODIFY', { label: 'auto-modified' }, id);
      const after = sql(`SELECT CONCAT(DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f'),'|',DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s.%f')) FROM stage4_auto WHERE id=${id};`).split('|');
      assert.equal(after[0], stamps[0]); assert.ok(after[1] > stamps[1]); return { id, stamps, after };
    });
    for (const field of ['stored_value', 'virtual_value']) await run(`${field} remains rejected as a real generated column`, async () => {
      await managed('stage4_generated'); await button('新增记录').click(); const values = { base_value: '9', [field]: '123' }; await edit(values);
      const response = await execute('stage4_generated', 'ADD', values, undefined, 400);
      assert.equal(response.response.error.code, 'invalid_mutation_content'); assert.equal(sql('SELECT COUNT(*) FROM stage4_generated;'), '0');
      return { metadata: sql("SELECT CONCAT_WS('|',COLUMN_NAME,EXTRA,GENERATION_EXPRESSION) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='stage4_generated';") };
    });
    await run('generated fields calculate on omitted ADD and stay unwritable on PATCH', async () => {
      await managed('stage4_generated'); await button('新增记录').click(); await edit({ base_value: '9' });
      const created = await execute('stage4_generated', 'ADD', { base_value: '9' }); const id = created.response.id;
      assert.equal(sql(`SELECT CONCAT_WS('|',base_value,stored_value,virtual_value) FROM stage4_generated WHERE id=${id};`), '9|18|10');
      await closeSuccess('ADD');
      for (const field of ['stored_value', 'virtual_value']) {
        await managed('stage4_generated'); await button(`修改记录 ${id}`).click(); const values = { [field]: '123' }; await edit(values);
        const rejected = await execute('stage4_generated', 'MODIFY', values, id, 400);
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
      const response = await execute('stage4_limits', 'ADD', values, undefined, 400);
      assert.equal(response.response.error.code, 'invalid_mutation_content'); assert.equal(sql('SELECT COUNT(*) FROM stage4_limits;'), beforeCount);
      await button('返回修改').click();
      await page.getByRole('dialog', { name: 'ADD Change Set', exact: true }).waitFor({ state: 'detached' });
      for (const [field, value] of Object.entries(values)) {
        assert.equal(await input(field).inputValue(), value, 'rejected draft remains editable');
        assert.equal(await checkbox(`包含 ${field}`).isChecked(), true);
      }
      await edit(fixed); const retried = await execute('stage4_limits', 'ADD', fixed); assert.ok(retried.response.id);
      assert.equal(sql('SELECT COUNT(*) FROM stage4_limits;'), String(Number(beforeCount) + 1));
      assert.equal(sql(`SELECT ${Object.keys(fixed)[0]} FROM stage4_limits WHERE id=${retried.response.id};`), Object.values(fixed)[0]);
      return { rejectedRequestId: response.requestId, retryRequestId: retried.requestId };
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
