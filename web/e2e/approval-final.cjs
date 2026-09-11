const { configureFixtureReleaseTemplates } = require('./release-template-fixture.cjs');
// AC-023: formal role configuration → table assignments → split/default review
// → observed notification acknowledgement → publication and result visibility.
const assert = require('node:assert/strict');
const { join } = require('node:path');
const { mkdirSync, writeFileSync } = require('node:fs');
const playwright = require('playwright');
const { selectedBrowser, browserOptions, registerFixtureAccount } = require('./local-account.cjs');
const { approvalFixtureRequest: api } = require('./table-approval-fixture.cjs');
const base = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
const button = (page, name) => page.getByRole('button', { name, exact: true });
const tables = ['multitable_browser_a', 'multitable_browser_b'];
(async () => {
 const browser = await selectedBrowser(playwright).launch(browserOptions());
 const checks = [], errors = [], geometry = [], pages = [];
 if (output) mkdirSync(output, { recursive: true });
 const check = name => { checks.push(name); console.log('PASS', name); };
 const pageFor = async context => { const page = await context.newPage(); pages.push(page); page.setDefaultTimeout(15000); page.on('pageerror', error => errors.push(error.message)); return page; };
 const person = async roles => { const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } }); const identity = await registerFixtureAccount(context, base, { roles }); return { context, identity, page: await pageFor(context) }; };
 const shot = async (page, name) => {
  await page.evaluate(async () => { window.scrollTo(0, 0); await Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {}))); });
  const measured = await page.evaluate(() => ({ viewport: innerWidth, document: document.documentElement.scrollWidth }));
  geometry.push({ name, ...measured }); assert.ok(measured.document <= measured.viewport, `${name} overflows: ${JSON.stringify(measured)}`);
  if (output) await page.screenshot({ path: join(output, name + '.png'), fullPage: true, animations: 'disabled' });
 };
 const keyboard = async locator => { await locator.focus(); await locator.press('Enter'); };
 try {
  const admin = await person(['ADMIN']), applicant = await person(['EDITOR', 'PUBLISHER']), finance = await person(['VIEWER']);
  const oldContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const preparation = await (await oldContext.request.get(`${base}/api/v1/auth/csrf`)).json();
  const login = await oldContext.request.post(`${base}/api/v1/auth/login`, { headers: { Origin: base, 'X-CSRF-Token': preparation.csrf_token }, data: { username: 'legacy.browser', password: 'legacy browser password long enough' } });
  assert.equal(login.status(), 200); const identity = await login.json(); assert.deepEqual(identity.account.roles, ['VIEWER']);
  const ops = { context: oldContext, identity: { accountID: identity.account.id, credentials: { username: 'legacy.browser' } }, page: await pageFor(oldContext) };
  await admin.page.goto(`${base}/platform/account-roles`);
  await admin.page.getByLabel('检索账号', { exact: true }).fill('legacy.browser');
  await button(admin.page, '查询').click();
  await button(admin.page, '管理 legacy.browser 的角色').click();
  assert.equal(await admin.page.getByRole('checkbox', { name: /APPROVER/ }).count(), 0);
  await admin.page.getByText('VIEWER → APPROVER', { exact: true }).waitFor();
  await admin.page.getByText('服务器当前角色：查看者（版本 4）', { exact: true }).waitFor();
  await shot(admin.page, 'current-roles-and-permanent-history');
  await button(admin.page, '关闭').last().click();
  check('正式v8旧APPROVER升级为VIEWER；当前选项退出，历史含义和版本仍可读');
  const mutation = 'approval_final_browser_v1';
  await api(admin.context, base, 'POST', '/api/v1/mutation-policies', { code: mutation, name: '最终全链路验收', description: '', type_code: 'single_table_mutation', allow_add: true, allow_modify: true, allow_delete: true }, 201);
  await api(admin.context, base, 'POST', `/api/v1/mutation-policies/${mutation}/activate`, {});
  for (const table of tables) {
   const created = await api(admin.context, base, 'POST', '/api/v1/table-policies', { table_name: table, query_policy_code: 'notification_page_query_v1', mutation_policy_code: mutation }, 201);
   await api(admin.context, base, 'POST', `/api/v1/table-policies/${table}/enable`, {expected_version:created.version});
  }
  await configureFixtureReleaseTemplates(admin.context, base, tables);
  const createRole = async (name, member) => {
   await admin.page.goto(`${base}/platform/approval-roles`);
   await keyboard(button(admin.page, '新建审批角色'));
   await admin.page.getByLabel('角色名称', { exact: true }).fill(name);
   await admin.page.getByLabel('角色说明', { exact: true }).fill('最终交付：明确人员职责，保留真实身份');
   await admin.page.getByLabel('检索人员', { exact: true }).fill(member.identity.credentials.username);
   await button(admin.page, '查询人员').click();
   const choice = admin.page.getByRole('checkbox', { name: `选择成员 ${member.identity.credentials.username}`, exact: true });
   await choice.focus(); await choice.press('Space');
   const response = admin.page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/api/v1/approval-roles'));
   await keyboard(button(admin.page, '保存审批角色'));
   const saved = await response; assert.equal(saved.status(), 201); const role = await saved.json();
   await admin.page.getByRole('dialog', { name: '新建审批角色', exact: true }).waitFor({ state: 'hidden' }); return role;
  };
  const opsRole = await createRole('商品运营审批角色 · 长名称完整呈现与人员分工', ops);
  const priceRole = await createRole('财务价格审批角色 · 多表申请的第二份独立责任', finance);
  const assign = async (table, role) => {
   await admin.page.goto(`${base}/platform/table-policies/${table}?mode=approvals`);
   await admin.page.getByRole('dialog', { name: '表审批角色', exact: true }).waitFor();
   await admin.page.getByRole('region', { name: '已选择审批角色', exact: true }).waitFor();
   for (const remove of await admin.page.getByRole('button', { name: /^移除审批角色 / }).all()) await remove.click();
   if (role) { const choice = admin.page.getByRole('checkbox', { name: `选择审批角色 ${role.name}`, exact: true }); await choice.focus(); await choice.press('Space'); }
   else await admin.page.getByText('无角色 · 默认 ADMIN 审批', { exact: true }).waitFor();
   const response = admin.page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith(`/api/v1/table-policies/${table}/approval-roles`));
   await keyboard(button(admin.page, '保存表审批角色'));
   const saved = await response; assert.equal(saved.status(), 200); assert.deepEqual((await saved.json()).role_ids, role ? [role.id] : []);
   await admin.page.getByRole('dialog', { name: '表审批角色', exact: true }).waitFor({ state: 'hidden' });
  };
  await assign(tables[0], opsRole); await assign(tables[1], priceRole);
  check('正式角色页检索成员并保存；键盘完成两张表的独立角色分配');
  const approve = async (person, order, count, source) => {
   await person.page.goto(`${base}/configuration/notifications`);
   const row = person.page.getByRole('row').filter({ hasText: order.title });
   await row.getByText('未读', { exact: true }).waitFor();
   await keyboard(person.page.getByRole('link', { name: `查看详情：${order.title}`, exact: true }));
   await person.page.getByText('已读；本单仍待你审批。', { exact: true }).waitFor();
   const counts = await api(person.context, base, 'GET', '/api/v1/approval-notifications'); assert.equal(counts.pending_count, 1); assert.equal(counts.unread_count, 0);
   await keyboard(button(person.page, '批准发布单'));
   const scope = person.page.getByRole('region', { name: '本次审批范围', exact: true });
   assert.ok((await scope.innerText()).includes(source));
   await person.page.getByLabel('审批意见', { exact: true }).fill(`已核对完整申请，本次仅处理${source}`);
   await keyboard(button(person.page, '确认批准'));
   await person.page.getByRole('heading', { name: `已通过 ${count} / 2 表`, exact: true }).waitFor();
  };
  for (const [index, width] of [1440, 390].entries()) {
   for (const person of [admin, applicant, ops, finance]) await person.page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
   if (index) { await assign(tables[1], null); await shot(admin.page, 'mobile-default-assignment-saved'); }
   const draft = await api(applicant.context, base, 'POST', '/api/v1/release-orders', { title: `${width}px 完整多表申请：商品由运营处理，价格由${index ? '独立管理员默认接手' : '财务独立处理'}，通知已读与结果保持真实`, items: tables.map(table_name => ({ table_name, operation: 'ADD', content: { id: String(9800 + index), label: 'final accepted value' } })) }, 201);
   await applicant.page.goto(`${base}/configuration/release-orders/${draft.id}`);
   await keyboard(button(applicant.page, '提交审批'));
   await keyboard(button(applicant.page, '确认提交审批'));
   await applicant.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^待审批$/ }).waitFor();
   await approve(ops, draft, 1, tables[0]);
   assert.equal(await button(ops.page, '执行发布').count(), 0);
   await shot(ops.page, `partial-approval-${width}`);
   const second = index ? admin : finance;
   await approve(second, draft, 2, tables[1]);
   const approved = await api(applicant.context, base, 'GET', `/api/v1/release-orders/${draft.id}`);
   assert.equal(approved.state, 'APPROVED');
   if (index) assert.equal(approved.approvals.find(a => a.table_name === tables[1]).decision.source, 'ADMIN');
   await applicant.page.goto(`${base}/configuration/notifications?view=mine`);
   await keyboard(applicant.page.getByRole('link', { name: `查看详情：${draft.title}`, exact: true }));
   await keyboard(button(applicant.page, '执行发布'));
   await keyboard(button(applicant.page, '确认发布到数据库'));
   await applicant.page.getByLabel('发布单状态', { exact: true }).filter({ hasText: /^已发布待完结$/ }).waitFor();
   await shot(applicant.page, `publication-result-${width}`);
   await ops.page.goto(`${base}/configuration/notifications?view=handled`);
   await ops.page.getByRole('row').filter({ hasText: draft.title }).getByText('未读', { exact: true }).waitFor();
   await keyboard(ops.page.getByRole('link', { name: `查看详情：${draft.title}`, exact: true }));
   await ops.page.getByRole('region', { name: '发布结果', exact: true }).getByText('值：final accepted value', { exact: true }).first().waitFor();
   const result = await api(ops.context, base, 'GET', `/api/v1/release-orders/${draft.id}`);
   assert.equal(result.state, 'SUCCEEDED'); assert.equal(result.notification.pending, false);
   await shot(ops.page, `reviewer-result-${width}`);
   check(`${width}px 完整多表提交、${index ? '默认ADMIN' : '分工'}审批、已读不清待办、发布和参与者结果`);
  }
  assert.deepEqual(errors, []);
  if (output) writeFileSync(join(output, 'result.json'), JSON.stringify({ engine: process.env.RCC_E2E_ENGINE || 'chromium', checks, errors, geometry }, null, 2));
 } catch (error) {
  if (output) { for (let index = 0; index < pages.length; index++) await pages[index].screenshot({ path: join(output, `failure-${index}.png`), fullPage: true }).catch(() => {}); writeFileSync(join(output, 'failure.json'), JSON.stringify({ error: error.stack, checks, errors, geometry }, null, 2)); }
  throw error;
 } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
