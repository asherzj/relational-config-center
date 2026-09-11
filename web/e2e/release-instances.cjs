// Real browser -> same-origin Web/Admin -> isolated policy_alpha/policy_beta MySQL fixture.
const assert = require('node:assert/strict');
const {randomUUID} = require('node:crypto');
const {writeFile, access} = require('node:fs/promises');
const {join} = require('node:path');
const {chromium} = require('playwright');
const {authenticatedRequest} = require('./local-account.cjs');
const {createFixtureApprovalRole, bindFixtureApprovalRoles} = require('./table-approval-fixture.cjs');
const origin = process.env.RCC_WEB_URL, output = process.env.RCC_E2E_OUTPUT;
assert.ok(origin && output, 'RCC_WEB_URL and a fresh RCC_E2E_OUTPUT directory are required');
const tables = ['policy_alpha', 'policy_beta'];
const checks = [], snapshots = {}, errors = [];
const button = (page, name) => page.getByRole('button', {name, exact:true});
const api = async (context, method, path, data, status=200) => {
 const response = await authenticatedRequest(context, origin, path, {method, data, headers:{'Idempotency-Key':randomUUID()}});
 assert.equal(response.status(), status, `${method} ${path}: ${await response.text()}`);
 return response.json();
};
const shot = async (page, name) => {
 const path=join(output,name);
 await assert.rejects(access(path), 'browser evidence must not overwrite an earlier run');
 await page.evaluate(()=>window.scrollTo(0,0));
 await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
 await page.screenshot({path, fullPage:true, animations:'disabled'});
};
const standard = (code, name, prefix) => ({code, name, description:'实例隔离浏览器验收', type:'STANDARD', node_list:[
 {code:'review',type:'APPROVAL',name:`${prefix}负责人核对`,required_role:'TABLE_APPROVER'},
 {code:'publish',type:'PUBLICATION',name:`${prefix}整单发布`,required_role:'PUBLISHER'},
 {code:'finish',type:'COMPLETION',name:`${prefix}结果完结`,required_role:'PUBLISHER'},
]});

(async()=>{
 const browser=await chromium.launch(process.env.RCC_BROWSER_EXECUTABLE?{executablePath:process.env.RCC_BROWSER_EXECUTABLE,headless:true}:{channel:'chrome',headless:true});
 let page;
 try {
  const admin=await browser.newContext({viewport:{width:1440,height:1000}});
  page=await admin.newPage();page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));
  await page.goto(origin+'/login');await page.getByLabel('用户名').fill('template.browser.admin');await page.getByLabel('密码',{exact:true}).fill('template browser password long enough');await button(page,'登录').click();await page.waitForURL(url=>!url.pathname.endsWith('/login'));
  await api(admin,'POST','/api/v1/query-policies',{code:'flow_browser_query_v1',name:'流程实例查询',description:'',type_code:'page_query',default_order_field:'id',default_order_direction:'ASC',default_page_size:20,max_page_size:200},201);
  await api(admin,'POST','/api/v1/query-policies/flow_browser_query_v1/activate');
  await api(admin,'POST','/api/v1/mutation-policies',{code:'flow_browser_mutation_v1',name:'流程实例变更',description:'',type_code:'single_table_mutation',allow_add:true,allow_modify:true,allow_delete:true},201);
  await api(admin,'POST','/api/v1/mutation-policies/flow_browser_mutation_v1/activate');
  for(const table of tables){await api(admin,'POST','/api/v1/table-policies',{table_name:table,query_policy_code:'flow_browser_query_v1',mutation_policy_code:'flow_browser_mutation_v1'},201);await api(admin,'POST',`/api/v1/table-policies/${table}/enable`,{expected_version:'1'});}
  const definitions=[standard('flow_alpha_v1','资金配置负责人核对及整单结果确认流程','资金'),standard('flow_beta_v1','渠道负责人独立复核与整单发布流程','渠道'),standard('flow_replacement_v1','新的关联流程','新配置')];
  const templates=[];for(const definition of definitions)templates.push(await api(admin,'POST','/api/v1/release-templates',definition,201));
  const associate=async(table,code,enabled=true)=>{
   const associations=(await api(admin,'GET',`/api/v1/table-policies/${table}/release-templates`)).associations;
   const current=associations.find(item=>item.type==='STANDARD');
   return api(admin,'PUT',`/api/v1/table-policies/${table}/release-templates/STANDARD`,{template_code:code,enabled,expected_version:current?.version??'0'});
  };
  for(const [index,table] of tables.entries())await associate(table,definitions[index].code);
  const reviewers=[];
  for(let index=0;index<2;index++){
   const context=await browser.newContext({viewport:{width:1440,height:1000}});
   const csrf=(await(await context.request.get(origin+'/api/v1/auth/csrf')).json()).csrf_token;
   const response=await context.request.post(origin+'/api/v1/auth/register',{headers:{Origin:origin,'X-CSRF-Token':csrf},data:{username:`flow.reviewer.${index}`,email:`flow.reviewer.${index}@example.invalid`,password:'flow browser reviewer password long enough',display_name:index?'渠道复核人':'资金复核人'}});
   assert.equal(response.status(),201);const identity=await response.json();
   const role=await createFixtureApprovalRole(admin,origin,index?'渠道审核角色':'资金审核角色',[identity.account.id],[tables[index]]);
   const surface=await context.newPage();surface.setDefaultTimeout(20000);surface.on('pageerror',error=>errors.push(error.message));
   reviewers.push({context,page:surface,identity,role});
  }
  await page.goto(origin+'/configuration/release-orders');await button(page,'新建草稿').click();await page.getByLabel('发布单标题',{exact:true}).fill('两张表保留各自流程与审批事实');await button(page,'创建空草稿').click();await page.getByRole('heading',{name:'两张表保留各自流程与审批事实',exact:true}).waitFor();
  const id=new URL(page.url()).pathname.split('/').at(-1), path=`/api/v1/release-orders/${id}`;
  for(const table of tables){
   await page.getByRole('link',{name:'添加明细',exact:true}).click();await page.getByLabel('Managed Table',{exact:true}).selectOption(table);await button(page,'新增记录').click();
   for(const field of ['id','value']){await page.getByLabel(`包含 ${field}`,{exact:true}).check();await page.getByLabel(`${field} 值`,{exact:true}).fill(field==='id'?'11':`${table} saved value`);}
   await button(page,'查看 Change Set').click();assert.equal(await page.getByLabel('保存到草稿',{exact:true}).inputValue(),id);await button(page,'确认并保存草稿').click();await page.getByRole('heading',{name:'两张表保留各自流程与审批事实',exact:true}).waitFor();
  }
  const saved=await api(admin,'GET',path);snapshots.saved=saved.table_flows;
  assert.equal(saved.release_type,'STANDARD');assert.deepEqual(saved.missing_flow_tables,[]);assert.equal(saved.table_flows.length,2);
  for(const [index,table] of tables.entries()){
   assert.equal(saved.table_flows.find(flow=>flow.table_name===table).template_code,definitions[index].code);
   await page.getByRole('list',{name:`${table} 流程节点`,exact:true}).getByText(definitions[index].node_list[0].name,{exact:true}).waitFor();
  }
  const readOnlyWrites=[];
  const observeRead=request=>{if(new URL(request.url()).pathname===path&&request.method()!=='GET')readOnlyWrites.push(request.method());};
  page.on('request',observeRead);await page.reload();await page.getByRole('region',{name:'逐表发布流程',exact:true}).waitFor();
  assert.deepEqual((await api(admin,'GET',path)).table_flows,saved.table_flows);assert.deepEqual(readOnlyWrites,[]);page.off('request',observeRead);
  const alphaFlow=page.getByRole('region',{name:`${tables[0]} 发布流程`,exact:true});
  await alphaFlow.getByText('查看实例来源与版本',{exact:true}).focus();await page.keyboard.press('Enter');
  await alphaFlow.getByText(`${definitions[0].code} · 版本 1`,{exact:true}).waitFor();
  await shot(page,'flow-source-keyboard-desktop.png');
  await page.keyboard.press('Enter');await alphaFlow.getByText(`${definitions[0].code} · 版本 1`,{exact:true}).waitFor({state:'hidden'});
  await shot(page,'different-table-flows-desktop.png');
  checks.push('AC-009: real Web creates a two-table draft; distinct persisted templates render, source disclosure opens/closes by keyboard, and refresh performs no write');

  await api(admin,'PUT',`/api/v1/release-templates/${definitions[0].code}`,{...definitions[0],name:'源模板已经更新',node_list:definitions[0].node_list.map(node=>({...node,name:`新定义${node.name}`})),expected_version:templates[0].version});
  await api(admin,'POST',`/api/v1/release-templates/${definitions[0].code}/disable`,{expected_version:'2'});
  await associate(tables[0],definitions[2].code);
  await page.reload();await page.getByRole('list',{name:`${tables[0]} 流程节点`,exact:true}).getByText('资金负责人核对',{exact:true}).waitFor();
  assert.deepEqual((await api(admin,'GET',path)).table_flows,saved.table_flows);
  checks.push('AC-010: editing/disabling source template and switching association leave the saved draft instance intact');

  await associate(tables[1],definitions[1].code,false);
  const incomplete=await api(admin,'POST','/api/v1/release-orders',{title:'配置补齐后明确保存的双表草稿',items:tables.map(table_name=>({table_name,operation:'ADD',content:{id:'12',value:'explicit repair'}}))},201);
  const repairPath=`/api/v1/release-orders/${incomplete.id}`;assert.deepEqual(incomplete.missing_flow_tables,[tables[1]]);assert.equal(incomplete.table_flows.length,1);assert.equal(incomplete.table_flows[0].template_code,definitions[2].code);
  const blocked=await api(admin,'POST',repairPath+'/submit',{expected_version:incomplete.version},409);assert.equal(blocked.error.code,'release_flow_incomplete');
  await page.setViewportSize({width:390,height:844});await page.goto(origin+'/configuration/release-orders/'+incomplete.id);
  await page.getByRole('region',{name:'流程配置未完成',exact:true}).waitFor();assert.equal(await button(page,'提交审批').count(),0);assert.equal(await page.getByRole('list',{name:`${tables[1]} 流程节点`,exact:true}).count(),0);
  await shot(page,'missing-flow-mobile.png');
  await associate(tables[1],definitions[1].code,true);await page.reload();await page.getByRole('region',{name:'流程配置未完成',exact:true}).waitFor();
  assert.deepEqual((await api(admin,'GET',repairPath)).missing_flow_tables,[tables[1]],'reading silently filled missing instance');
  await button(page,'保存草稿以补齐流程').focus();await page.keyboard.press('Enter');await page.getByRole('heading',{name:'编辑多表草稿',exact:true}).waitFor();
  const originalWrites=[];let loseOnce=true;
  await page.route(`**${repairPath}`,async route=>{
   if(route.request().method()!=='PUT')return route.continue();
   originalWrites.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
   if(!loseOnce)return route.continue();loseOnce=false;
   const response=await route.fetch();assert.equal(response.status(),200);await route.abort('connectionreset');
  });
  await button(page,'保存草稿修改').click();await page.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();assert.equal(originalWrites.length,1);
  assert.deepEqual(JSON.parse(originalWrites[0].body),{title:incomplete.title,expected_version:incomplete.version,changes:{upserts:[],delete_detail_ids:[]}});
  page.once('dialog',dialog=>dialog.accept());await page.reload();await page.getByRole('heading',{name:incomplete.title,exact:true}).waitFor();
  await button(page,'编辑草稿').click();await button(page,'保存草稿修改').click();await page.getByRole('heading',{name:'编辑多表草稿',exact:true}).waitFor({state:'hidden'});
  assert.equal(originalWrites.length,2);assert.deepEqual(originalWrites[0],originalWrites[1]);await page.unroute(`**${repairPath}`);
  const repaired=await api(admin,'GET',repairPath);snapshots.repaired=repaired.table_flows;
  assert.deepEqual(repaired.missing_flow_tables,[]);assert.deepEqual(repaired.table_flows.find(flow=>flow.table_name===tables[0]),incomplete.table_flows[0]);assert.equal(repaired.table_flows.length,2);
  await page.getByRole('list',{name:`${tables[1]} 流程节点`,exact:true}).getByText('渠道负责人核对',{exact:true}).waitFor();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
  await shot(page,'repaired-flows-mobile.png');
  snapshots.manual_replay={same_key:true,same_body:true,original_version:incomplete.version,stored_version:repaired.version};
  checks.push('AC-010/023/025: 390px missing-flow block; GET stays read-only after repair; explicit unchanged save fills only missing instance; committed lost response survives refresh and exact manual replay');

  await page.setViewportSize({width:1440,height:1000});await page.goto(origin+'/configuration/release-orders/'+id);
  await page.getByRole('region',{name:'提交审批安排',exact:true}).getByText('当前分配：资金审核角色',{exact:true}).waitFor();
  await button(page,'提交审批').click();await button(page,'确认提交审批').click();await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'待审批',exact:true}).waitFor();
  let submitted=await api(admin,'GET',path);snapshots.submitted=submitted.table_flows;
  assert.deepEqual(submitted.table_flows.map(flow=>flow.instance_id),saved.table_flows.map(flow=>flow.instance_id));
  assert.deepEqual(submitted.table_flows.map(flow=>flow.node_list.map(node=>node.state)),[['ACTIVE','PENDING','PENDING'],['ACTIVE','PENDING','PENDING']]);
  await bindFixtureApprovalRoles(admin,origin,tables[0],[reviewers[1].role.id]);await page.reload();
  await page.getByRole('region',{name:'逐表审批进度',exact:true}).getByText('提交时分配：资金审核角色',{exact:true}).waitFor();
  for(const [index,reviewer] of reviewers.entries()){
   const review=reviewer.page;await review.goto(origin+'/configuration/release-orders/'+id);await button(review,'批准发布单').click();
   const scope=review.getByRole('region',{name:'本次审批范围',exact:true});await scope.getByText(tables[index],{exact:true}).waitFor();assert.equal(await scope.getByText(tables[1-index],{exact:true}).count(),0);
   await review.getByLabel('审批意见',{exact:true}).fill(index?'渠道独立核对完成':'资金独立核对完成');await button(review,'确认批准').click();await review.getByRole('dialog',{name:'批准发布单',exact:true}).waitFor({state:'hidden'});await page.reload();
   await page.getByRole('region',{name:'逐表审批进度',exact:true}).getByRole('heading',{name:`已通过 ${index+1} / 2 表`,exact:true}).waitFor();
   const current=await api(admin,'GET',path);
   if(index===0){
    assert.equal(current.state,'PENDING_APPROVAL');assert.deepEqual(current.table_flows.map(flow=>flow.node_list.map(node=>node.state)),[['COMPLETED','PENDING','PENDING'],['ACTIVE','PENDING','PENDING']]);
    await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'待审批',exact:true}).waitFor();assert.equal(await button(page,'执行发布').count(),0);snapshots.partial_approval=current.table_flows;
    await shot(page,'partial-approval-desktop.png');
   }else{
    assert.equal(current.state,'APPROVED');assert.deepEqual(current.table_flows.map(flow=>flow.node_list.map(node=>node.state)),[['COMPLETED','ACTIVE','PENDING'],['COMPLETED','ACTIVE','PENDING']]);
    await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'待执行发布',exact:true}).waitFor();snapshots.all_approved=current.table_flows;
   }
  }
  await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await shot(page,'all-approved-mobile.png');
  await button(page,'执行发布').click();await button(page,'确认发布到数据库').click();await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'数据库已发布，待人工完结',exact:true}).waitFor();
  const published=await api(admin,'GET',path);assert.deepEqual(published.table_flows.map(flow=>flow.node_list.map(node=>node.state)),[['COMPLETED','COMPLETED','ACTIVE'],['COMPLETED','COMPLETED','ACTIVE']]);
  await button(page,'完结发布单').click();await button(page,'确认完结').click();await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'已完结',exact:true}).waitFor();
  const completed=await api(admin,'GET',path);snapshots.completed=completed.table_flows;assert.ok(completed.table_flows.every(flow=>flow.node_list.every(node=>node.state==='COMPLETED'&&node.actor_id&&node.at)));
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await shot(page,'completed-flows-mobile.png');
  checks.push('AC-014: frozen table roles survive later assignment change; independent partial/all approvals drive real nodes; whole-order publication and completion retain actual actors');
  assert.deepEqual(errors,[]);
  const evidence={order_id:id,repair_order_id:incomplete.id,checks,snapshots};await writeFile(join(output,'release-instances-browser-evidence.json'),JSON.stringify(evidence,null,2),{flag:'wx'});process.stdout.write(JSON.stringify({checks,order_id:id,repair_order_id:incomplete.id}));
 }catch(error){if(page){await shot(page,'failure.png');await writeFile(join(output,'failure.txt'),String(error),{flag:'wx'});}throw error;}finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
