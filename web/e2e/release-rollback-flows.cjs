// #105: run only on a fresh disposable template fixture containing policy_alpha
// and policy_beta (id/value), plus template.browser.admin. No fixture SQL or
// private business entrypoint is used here; the parent runner owns MySQL/Admin.
const assert = require('node:assert/strict');
const {randomUUID} = require('node:crypto');
const {access, writeFile} = require('node:fs/promises');
const {join} = require('node:path');
const playwright = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const {authenticatedRequest, selectedBrowser, browserOptions, setFixtureRoles} = require('./local-account.cjs');
const {fixtureApprovalInput} = require('./table-approval-fixture.cjs');
const {readAllReleaseDetailPages, executionCommands} = require('./release-detail-pages.cjs');
const origin = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
assert.equal(process.env.RCC_E2E_ISOLATED, '1', 'an explicitly isolated, disposable fixture is required');
assert.ok(origin && output, 'RCC_WEB_URL and a fresh existing RCC_E2E_OUTPUT directory are required');
const target = new URL(origin);
assert.equal(target.protocol, 'http:');
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(target.hostname), 'fixture must use local HTTP');
const tables = ['policy_alpha', 'policy_beta'];
const button = (page, name) => page.getByRole('button', {name, exact:true});
const checks = [], snapshots = {}, requests = [], errors = [], contexts = [];
const check = label => {checks.push(label)};
const api = async (context, method, path, data, status=200) => {
 const response = await authenticatedRequest(context, origin, path, {method, data, headers:{'Idempotency-Key':randomUUID()}});
 assert.equal(response.status(), status, `${method} ${path}: ${await response.text()}`);
 return response.json();
};
const savedFile = async (name, value) => writeFile(join(output, name), value, {flag:'wx'});
const shot = async (page, name) => {
 await page.mouse.move(0, 0);
 await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
 await page.evaluate(() => Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {}))));
 const layout = await page.evaluate(() => ({
  viewport:{width:innerWidth,height:innerHeight}, document:document.documentElement.scrollWidth,
  dialogs:[...document.querySelectorAll('[role="dialog"][data-modal-surface="true"]')].filter(element => element.getBoundingClientRect().width > 0).map(element => {
   const bounds = element.getBoundingClientRect(); return {label:element.getAttribute('aria-labelledby'),left:bounds.left,right:bounds.right,top:bounds.top,bottom:bounds.bottom};
  }),
  controls:[...document.querySelectorAll('button,input,textarea,select')].filter(element => element.getBoundingClientRect().width > 0).map(element => {
   const bounds = element.getBoundingClientRect(); return {label:element.getAttribute('aria-label') || element.textContent.slice(0, 100),left:bounds.left,right:bounds.right};
  }),
 }));
 await savedFile(`${name}-layout.json`, JSON.stringify(layout, null, 2));
 assert.ok(layout.document <= layout.viewport.width, `${name}: document overflow`);
 for(const dialog of layout.dialogs) assert.ok(dialog.left >= -1 && dialog.right <= layout.viewport.width+1 && dialog.top >= -1 && dialog.bottom <= layout.viewport.height+1, `${name}: inaccessible dialog ${JSON.stringify(dialog)}`);
 const path = join(output, `${name}.png`); await assert.rejects(access(path), 'do not overwrite prior evidence');
 await page.screenshot({path,fullPage:layout.dialogs.length===0,animations:'disabled'});
};
const shots = async (page, name) => {
 await page.setViewportSize({width:1440,height:1000}); await shot(page, `${name}-desktop`);
 await page.setViewportSize({width:390,height:844}); await shot(page, `${name}-390`);
};
const template = (code, name, prefix) => ({code,name,description:'原单应急回滚逐表实例浏览器验收',type:'EMERGENCY',node_list:[
 {code:'restore',type:'PUBLICATION',name:`${prefix}整单恢复配置`,required_role:'PUBLISHER'},
 {code:'finish',type:'COMPLETION',name:`${prefix}人工完结节点`,required_role:'PUBLISHER'},
]});

(async () => {
 const browser = await selectedBrowser(playwright).launch(browserOptions());
 let page;
 try {
  const admin = await browser.newContext({viewport:{width:1440,height:1000}}), adminPage = await admin.newPage();
  contexts.push({name:'admin',context:admin});
  await adminPage.goto(origin+'/login');
  await adminPage.getByLabel('用户名').fill(process.env.RCC_E2E_ADMIN_USERNAME || 'template.browser.admin');
  await adminPage.getByLabel('密码',{exact:true}).fill(process.env.RCC_E2E_ADMIN_PASSWORD || 'template browser password long enough');
  await button(adminPage,'登录').click();await adminPage.getByRole('heading',{name:'登录本地账号'}).waitFor({state:'hidden'});
  const person = async (name, roles) => {
   const context = await browser.newContext({viewport:{width:1440,height:1000}});
   const csrfResponse = await context.request.get(origin+'/api/v1/auth/csrf');assert.equal(csrfResponse.status(),200);
   const csrf = (await csrfResponse.json()).csrf_token;
   const username = `r105.${name[0]}.${randomUUID().replaceAll('-','').slice(0,20)}`;
   const registration = await context.request.post(origin+'/api/v1/auth/register',{headers:{Origin:origin,'X-CSRF-Token':csrf},data:{username,email:`${username}@example.invalid`,password:`${randomUUID()}-fixture-password`,display_name:`应急恢复验收${name}`}});
   assert.equal(registration.status(),201,await registration.text());const identity = await registration.json();
   await setFixtureRoles(admin,origin,identity.account.id,roles);
   await context.tracing.start({screenshots:true,snapshots:true,sources:true});
   contexts.push({name,context,tracing:true});
   const personalPage = await context.newPage();personalPage.setDefaultTimeout(20000);personalPage.on('pageerror',error=>errors.push(`${name}: ${error.message}`));
   return {context,page:personalPage,accountID:identity.account.id};
  };
  const applicant = await person('applicant',['EDITOR']), publisher = await person('publisher',['PUBLISHER']);page = publisher.page;
  await api(admin,'POST','/api/v1/query-policies',{code:'rollback_flows_query_v1',name:'恢复流程验收查询',description:'',type_code:'page_query',default_order_field:'id',default_order_direction:'ASC',default_page_size:20,max_page_size:200},201);
  await api(admin,'POST','/api/v1/query-policies/rollback_flows_query_v1/activate');
  await api(admin,'POST','/api/v1/mutation-policies',{code:'rollback_flows_mutation_v1',name:'恢复流程验收变更',description:'',type_code:'single_table_mutation',allow_add:true,allow_modify:true,allow_delete:true},201);
  await api(admin,'POST','/api/v1/mutation-policies/rollback_flows_mutation_v1/activate');
  const definitions = [template('rollback_flows_alpha_v1','资金配置应急恢复流程（冻结后模板修改不影响本次恢复）','资金'),template('rollback_flows_beta_v1','渠道配置独立应急恢复流程（真实节点与整单恢复结果）','渠道')];
  const createdTemplates = [];
  for(const definition of definitions)createdTemplates.push(await api(admin,'POST','/api/v1/release-templates',definition,201));
  for(const [index,table] of tables.entries()){
   await api(admin,'POST','/api/v1/table-policies',{table_name:table,query_policy_code:'rollback_flows_query_v1',mutation_policy_code:'rollback_flows_mutation_v1'},201);
   await api(admin,'POST',`/api/v1/table-policies/${table}/enable`,{expected_version:'1'});
   const associations = (await api(admin,'GET',`/api/v1/table-policies/${table}/release-templates`)).associations;
   for(const type of ['STANDARD','EMERGENCY'])await api(admin,'PUT',`/api/v1/table-policies/${table}/release-templates/${type}`,{template_code:type==='STANDARD'?'default_standard_v1':definitions[index].code,enabled:true,expected_version:associations.find(entry=>entry.type===type)?.version??'0'});
  }
  const read = id => api(publisher.context,'GET',`/api/v1/release-orders/${id}`);
  const query = (table, id) => api(applicant.context,'POST',`/api/v1/tables/${table}/query`,{conditions:[{field:'id',operator:'exact',value:String(id)}]});
  const publish = async (title, release_type, firstID) => {
   let order = await api(applicant.context,'POST','/api/v1/release-orders',{title,release_type,items:tables.map((table_name,index)=>({table_name,operation:'ADD',content:{id:String(firstID+index),value:`恢复验收已发布值 ${firstID+index}`}}))},201);
   const path = `/api/v1/release-orders/${order.id}`;
   order = await api(applicant.context,'POST',path+'/submit',{expected_version:order.version,...(release_type==='EMERGENCY'?{emergency_reason:'验证原单固定应急恢复'}:{})});
   if(release_type==='STANDARD')order = await api(admin,'POST',path+'/approve',await fixtureApprovalInput(admin,origin,order.id,{expected_version:order.version,reason:'管理员独立核对两张表'}));
   await page.goto(origin+`/configuration/release-orders/${order.id}`);await button(page,'执行发布').click();await button(page,'确认发布到数据库').click();
   await page.getByLabel('发布单状态',{exact:true}).filter({hasText:'已发布待完结'}).waitFor();
   const published = await read(order.id);assert.equal(published.state,'SUCCEEDED');assert.deepEqual(published.rollback_table_flows,[]);
   return published;
  };
  const unknownPreview = async order => {
   const path = `/api/v1/release-orders/${order.id}/quick-rollback/preview`, routePath = `**${path}`, sent = [];
   let first = true, committed;
   await page.route(routePath,async route=>{
    sent.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
    if(!first)return route.continue();first=false;
    const response = await route.fetch();assert.equal(response.status(),200,await response.text());committed = await response.json();
    await route.abort('connectionreset');
   });
   await button(page,'快速回滚').click();
   await page.getByText('保存恢复预览的结果尚未确认，原发布单版本、正文与请求标识已保留；请手动再次保存恢复预览。',{exact:true}).waitFor();
   assert.equal(sent.length,1);assert.ok(sent[0].key);assert.deepEqual(JSON.parse(sent[0].body),{expected_version:order.version});
   assert.equal(committed.order_id,order.id);assert.equal(committed.release_type,'EMERGENCY');assert.equal(committed.expected_version,String(BigInt(order.version)+1n));
   assert.equal(committed.table_flows.length,2);assert.ok(committed.table_flows.every(flow=>flow.release_type==='EMERGENCY'&&flow.node_list.every(node=>node.type!=='APPROVAL')));
   return {path,routePath,sent,committed};
  };
  const closeUnknown = async () => {
   await button(page,'取消快速回滚').click();await page.getByRole('alertdialog',{name:'放弃未保存的修改？',exact:true}).waitFor();
   await button(page,'放弃修改并离开').click();await page.getByRole('dialog',{name:/^快速回滚 ·/}).waitFor({state:'hidden'});
  };
  const reopenAndReplay = async (order, original) => {
   page.once('dialog',dialog=>dialog.accept());await page.reload();await page.getByRole('heading',{name:order.title,exact:true}).waitFor();
   await button(page,'快速回滚').click();await button(page,'保存恢复预览').waitFor();
   assert.equal(original.sent.length,1,'reload and reopening must not automatically resend the unknown preview');
   assert.equal(await button(page,'确认整单快速回滚').isDisabled(),true);
   await button(page,'保存恢复预览').focus();assert.equal(await button(page,'保存恢复预览').evaluate(node=>node===document.activeElement),true);
   const replay = page.waitForResponse(response=>new URL(response.url()).pathname===original.path&&response.request().method()==='POST');
   await page.keyboard.press('Enter');const response = await replay;assert.equal(response.status(),200,await response.text());
   assert.deepEqual(await response.json(),original.committed);assert.equal(original.sent.length,2);assert.deepEqual(original.sent[1],original.sent[0]);
   await page.getByRole('region',{name:'整单恢复预览',exact:true}).waitFor();
   requests.push({order_id:order.id,preview:original.sent});
  };

  const order = await publish('资金与渠道配置的常规原单应急恢复（持久实例与原包重推）','STANDARD',10501);
  await shots(page,'01-published');
  const original = await unknownPreview(order);
  const saved = await read(order.id);assert.deepEqual(saved.rollback_table_flows,original.committed.table_flows);snapshots.first_saved = saved;
  await shots(page,'02-preview-unknown');
  for(const [index,definition] of definitions.entries())await api(admin,'PUT',`/api/v1/release-templates/${definition.code}`,{...definition,name:`后续修改不应覆盖：${definition.name}`,node_list:definition.node_list.map(node=>({...node,name:`后续修改${node.name}`})),expected_version:createdTemplates[index].version});
  assert.deepEqual((await read(order.id)).rollback_table_flows,saved.rollback_table_flows);
  await closeUnknown();await shots(page,'03-saved-detail-after-template-edit');
  await reopenAndReplay(order,original);
  const dialog = page.getByRole('dialog',{name:`快速回滚 · ${order.title}`,exact:true});
  for(const [index,table] of tables.entries()){
   const region=dialog.getByRole('region',{name:`${table} 恢复流程`,exact:true});await region.getByText(definitions[index].name,{exact:true}).waitFor();
   for(const node of definitions[index].node_list)await region.getByRole('heading',{name:node.name,exact:true}).waitFor();
   assert.equal(await region.getByText(/^后续修改/).count(),0);
  }
  assert.equal(await dialog.getByRole('combobox').count(),0);assert.equal(await dialog.getByLabel('快速回滚原因（选填）',{exact:true}).getAttribute('required'),null);
  await shots(page,'04-original-preview-replayed');
  // Reopening after an acknowledged preview creates a new write intent, while
  // reusing the exact persisted table instances and post-save order version.
  await button(page,'取消快速回滚').click();await page.unroute(original.routePath);
  const freshPreview = page.waitForResponse(response=>new URL(response.url()).pathname===original.path&&response.request().method()==='POST');
  await button(page,'快速回滚').click();const freshResponse=await freshPreview;assert.equal(freshResponse.status(),200,await freshResponse.text());
  const fresh = await freshResponse.json();assert.deepEqual(fresh.table_flows,saved.rollback_table_flows);assert.equal(fresh.expected_version,saved.version);assert.notEqual(freshResponse.request().headers()['idempotency-key'],original.sent[0].key);
  await page.getByRole('region',{name:'整单恢复预览',exact:true}).waitFor();
  check('AC-019/023: committed preview response loss survives close/reload, manual original-key replay, template edits, and a fresh-key reopen with unchanged persisted per-table emergency instances');

  const path=`/api/v1/release-orders/${order.id}`, rollbackResponse=page.waitForResponse(response=>new URL(response.url()).pathname===path+'/quick-rollback'&&response.request().method()==='POST');
  await button(page,'确认整单快速回滚').focus();await page.keyboard.press('Enter');const response=await rollbackResponse;assert.equal(response.status(),200,await response.text());
  assert.deepEqual(response.request().postDataJSON(),{expected_version:fresh.expected_version,preview_digest:fresh.preview_digest,reason:''});requests.push({order_id:order.id,rollback:{body:response.request().postData(),key:response.request().headers()['idempotency-key']}});
  await page.getByLabel('发布单状态',{exact:true}).filter({hasText:'已回滚'}).waitFor();await page.getByRole('dialog',{name:/^快速回滚 ·/}).waitFor({state:'hidden'});
  const rolled=await read(order.id);assert.equal(rolled.id,order.id);assert.equal(rolled.state,'ROLLED_BACK');assert.equal(rolled.history.filter(event=>event.action==='COMPLETE').length,0);assert.equal(rolled.history.filter(event=>event.action==='QUICK_ROLLBACK').length,1);
  for(const flow of rolled.rollback_table_flows){
   assert.deepEqual(flow.node_list.map(node=>node.state),['COMPLETED','STOPPED']);assert.equal(flow.node_list[0].actor_id,publisher.accountID);assert.ok(flow.node_list[0].at);assert.ok(!flow.node_list[1].actor_id&&!flow.node_list[1].at);
   const region=page.getByRole('region',{name:`${flow.table_name} 恢复流程`,exact:true});await region.getByText('已完成',{exact:true}).waitFor();await region.getByText('已终止',{exact:true}).waitFor();
  }
  assert.equal(await button(page,'完结发布单').count(),0);assert.equal(await button(page,'快速回滚').count(),0);
  for(const [index,table] of tables.entries())assert.deepEqual((await query(table,10501+index)).rows,[]);
  const details=await readAllReleaseDetailPages(publisher.context,origin,rolled), commands=executionCommands(details,'ROLLBACK');assert.deepEqual(commands.map(command=>command.table_name),[...tables].reverse());
  // Command cursors belong to each table; they cannot establish cross-table
  // execution order. The HTTP constraint suite proves actual inverse ordering.
  for(const item of details.items){
   assert.equal(BigInt(item.rollback.sequence),BigInt(item.publication.sequence)+1n);
   assert.equal(BigInt(item.rollback.table_version),BigInt(item.publication.table_version)+1n);
  }
  assert.equal(details.executions.length,2);assert.equal(details.executions[1].item_count,2);snapshots.rolled=details;
  await shots(page,'05-directly-ended-result');
  await button(page,'补填回滚原因').click();const reason='先恢复服务，再补充资金与渠道配置应急恢复原因；不追加人工完结。';await page.getByLabel('回滚原因（选填）',{exact:true}).fill(reason);await shots(page,'06-reason-editor');
  await button(page,'保存回滚原因').click();await page.getByRole('region',{name:'回滚原因',exact:true}).getByText(reason,{exact:true}).waitFor();
  const afterReason=await read(order.id);assert.equal(afterReason.history.at(-1).action,'ROLLBACK_REASON');assert.equal(afterReason.history.at(-1).reason,reason);assert.deepEqual(afterReason.executions,rolled.executions);
  await shots(page,'07-reason-saved');
  check('AC-020: keyboard confirmation with an empty reason reverses both tables once, ends the original order, records real restoration actors, stops completion nodes without a fictitious completion, and supports later reason correction');

  const notification=(await api(applicant.context,'GET',path)).notification;assert.equal(notification.unread,true);assert.equal(notification.pending,false);
  await applicant.page.goto(origin+`/configuration/notifications?view=mine&unread=true&id=${order.id}`);
  const row=applicant.page.getByRole('row').filter({hasText:order.title});await row.getByText('未读',{exact:true}).waitFor();assert.equal(await row.count(),1);
  await shots(applicant.page,'08-applicant-notification-list');
  const notificationLink=row.getByRole('link',{name:`查看详情：${order.title}`,exact:true});await notificationLink.focus();await applicant.page.keyboard.press('Enter');
  await applicant.page.getByLabel('发布单状态',{exact:true}).filter({hasText:'已回滚'}).waitFor();assert.equal(new URL(applicant.page.url()).pathname,`/configuration/notifications/${order.id}`);
  await applicant.page.getByRole('region',{name:'逐表应急恢复流程',exact:true}).waitFor();await shots(applicant.page,'09-notification-current-result');
  check('AC-021: the real applicant result notification opens the same original order and persisted per-table restoration details, without a pending approval');

  for(const [index,terminal] of ['COMPLETED','ROLLED_BACK'].entries()){
   const later=await publish(`应急原单历史预览不能重新开放恢复 ${terminal}`,'EMERGENCY',10521+index*10), retained=await unknownPreview(later);
   const current=await read(later.id), terminalPath=`/api/v1/release-orders/${later.id}`;
   const terminalOrder=terminal==='COMPLETED'?await api(publisher.context,'POST',terminalPath+'/complete',{expected_version:current.version}):await api(publisher.context,'POST',terminalPath+'/quick-rollback',{expected_version:retained.committed.expected_version,preview_digest:retained.committed.preview_digest,reason:''});
   assert.equal(terminalOrder.state,terminal);await closeUnknown();await reopenAndReplay(later,retained);
   await page.getByText(`这是原请求已保存的历史恢复预览。原单${terminal==='COMPLETED'?'已完结':'已回滚'}，不能用于新的回滚。`,{exact:true}).waitFor();assert.equal(await button(page,'确认整单快速回滚').isDisabled(),true);
   assert.equal((await read(later.id)).state,terminal);await shots(page,`10-historical-preview-${terminal.toLowerCase()}`);
   await button(page,'取消快速回滚').click();assert.equal(await button(page,'快速回滚').count(),0);await page.unroute(retained.routePath);snapshots[terminal.toLowerCase()]=await read(later.id);
  }
  check('AC-023/024: historical preview replay acknowledges the original successful save after completion or rollback, while authoritative header reads keep both terminal orders ineligible for another recovery');
  assert.deepEqual(errors,[]);await savedFile('release-rollback-flows-browser-evidence.json',JSON.stringify({checks,requests,snapshots,errors},null,2));
  console.log(JSON.stringify({checks,order_id:order.id}));
 } catch(error) {
  if(page)await shot(page,'release-rollback-flows-failure').catch(()=>{});
  await savedFile('release-rollback-flows-failure.txt',String(error.stack||error)).catch(()=>{});throw error;
 } finally {
  for(const entry of contexts)if(entry.tracing)await entry.context.tracing.stop({path:join(output,`release-rollback-flows-${entry.name}-trace.zip`)}).catch(()=>{});
  await browser.close();
 }
})().catch(error=>{console.error(error);process.exitCode=1});
