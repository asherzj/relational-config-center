// Destructive browser acceptance for a newly created disposable local fixture only.
const assert=require('node:assert/strict');
const {randomUUID}=require('node:crypto');
const {access,writeFile}=require('node:fs/promises');
const {join}=require('node:path');
const {chromium}=require('playwright');
const {authenticatedRequest}=require('./local-account.cjs');

const origin=process.env.RCC_WEB_URL;
const output=process.env.RCC_E2E_OUTPUT;
assert.equal(process.env.RCC_E2E_ISOLATED,'1','RCC_E2E_ISOLATED=1 is required for this destructive fixture');
assert.ok(origin&&output,'RCC_WEB_URL and a fresh RCC_E2E_OUTPUT directory are required');
const target=new URL(origin);
assert.equal(target.protocol,'http:','the disposable browser fixture must use local HTTP');
assert.ok(['127.0.0.1','localhost','::1'].includes(target.hostname),'the disposable browser fixture must use a loopback host');

const tables=['policy_alpha','policy_beta'];
const checks=[];
const snapshots={};
const pageErrors=[];
const button=(page,name)=>page.getByRole('button',{name,exact:true});
const basicInformation=async page=>{
 const details=page.locator('details[aria-label="基本信息"]');
 if(!await details.evaluate(node=>node.open))await details.locator('summary').click();
 return details;
};
const api=async(context,method,path,data,status=200,headers={})=>{
 const response=await authenticatedRequest(context,origin,path,{method,data,headers:{'Idempotency-Key':randomUUID(),...headers}});
 assert.equal(response.status(),status,`${method} ${path}: ${await response.text()}`);
 return response.json();
};
const shot=async(page,name)=>{
 const path=join(output,name);
 await assert.rejects(access(path),'browser evidence must not overwrite an earlier run');
 await page.mouse.move(0,0);
 await page.locator('[data-sonner-toast]').first().waitFor({state:'hidden'});
 await page.evaluate(()=>window.scrollTo(0,0));
 const viewportOnly=/reason-mobile|unsaved-mobile|session-interrupted-mobile/.test(name);
 await page.screenshot({path,fullPage:!viewportOnly,animations:'disabled'});
};
const template=(type,code,name,prefix)=>({
 code,name,description:'应急发布真实浏览器验收',type,
 node_list:[
  ...(type==='STANDARD'?[{code:'review',type:'APPROVAL',name:`${prefix}逐表审批`,required_role:'TABLE_APPROVER'}]:[]),
  {code:'publish',type:'PUBLICATION',name:`${prefix}当前发布人员手工发布`,required_role:'PUBLISHER'},
  {code:'finish',type:'COMPLETION',name:`${prefix}发布成功后人工完结`,required_role:'PUBLISHER'},
 ],
});

(async()=>{
 const browser=await chromium.launch(process.env.RCC_BROWSER_EXECUTABLE?{executablePath:process.env.RCC_BROWSER_EXECUTABLE,headless:true}:{channel:'chrome',headless:true});
 let page;
 try{
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',error=>pageErrors.push(error.message));
  const login=async()=>{await page.getByLabel('用户名').fill('template.browser.admin');await page.getByLabel('密码',{exact:true}).fill('template browser password long enough');await button(page,'登录').click();await page.getByRole('heading',{name:'登录本地账号'}).waitFor({state:'hidden'});};
  await page.goto(origin+'/login');await login();

  await api(context,'POST','/api/v1/query-policies',{code:'emergency_browser_query_v1',name:'应急发布查询',description:'',type_code:'page_query',default_order_field:'id',default_order_direction:'ASC',default_page_size:20,max_page_size:200},201);
  await api(context,'POST','/api/v1/query-policies/emergency_browser_query_v1/activate');
  await api(context,'POST','/api/v1/mutation-policies',{code:'emergency_browser_mutation_v1',name:'应急发布变更',description:'',type_code:'single_table_mutation',allow_add:true,allow_modify:true,allow_delete:true},201);
  await api(context,'POST','/api/v1/mutation-policies/emergency_browser_mutation_v1/activate');
  for(const table of tables){
   await api(context,'POST','/api/v1/table-policies',{table_name:table,query_policy_code:'emergency_browser_query_v1',mutation_policy_code:'emergency_browser_mutation_v1'},201);
   await api(context,'POST',`/api/v1/table-policies/${table}/enable`,{expected_version:'1'});
  }
  const definitions=[
   template('STANDARD','emergency_browser_standard_v1','常规发布流程','常规'),
   template('EMERGENCY','emergency_browser_emergency_v1','跨区域资金与渠道配置应急发布及发布成功后人工完结流程','应急'),
  ];
  for(const definition of definitions)await api(context,'POST','/api/v1/release-templates',definition,201);
  const associate=async(table,type,code)=>{
   const current=(await api(context,'GET',`/api/v1/table-policies/${table}/release-templates`)).associations.find(item=>item.type===type);
   await api(context,'PUT',`/api/v1/table-policies/${table}/release-templates/${type}`,{template_code:code,enabled:true,expected_version:current?.version??'0'});
  };
  for(const table of tables)for(const definition of definitions)await associate(table,definition.type,definition.code);

  const title='跨区域资金与渠道配置紧急止损发布单（浏览器真实系统路径）';
  let order=await api(context,'POST','/api/v1/release-orders',{title,release_type:'EMERGENCY',items:tables.map((table_name,index)=>({table_name,operation:'ADD',content:{id:String(10401+index),value:`emergency value ${index+1}`}}))},201);
  const path=`/api/v1/release-orders/${order.id}`;
  assert.equal(order.release_type,'EMERGENCY');assert.equal(order.table_flows.length,2);
  assert.ok(order.table_flows.every(flow=>flow.node_list.every(node=>node.type!=='APPROVAL')));snapshots.initial_flows=order.table_flows;
  await page.goto(origin+`/configuration/release-orders/${order.id}`);
  await page.getByRole('heading',{name:title,exact:true}).waitFor();
  let information=await basicInformation(page);await information.getByText('应急发布',{exact:true}).waitFor();
  for(const table of tables)await page.getByRole('region',{name:`${table} 发布流程`,exact:true}).getByText(/当前发布人员手工发布/).waitFor();

  const editWrites=[];let loseEdit=true;
  await page.route(`**${path}`,async route=>{
   if(route.request().method()!=='PUT')return route.continue();
   editWrites.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
   if(!loseEdit)return route.continue();loseEdit=false;
   const response=await route.fetch();assert.equal(response.status(),200);await route.abort('connectionreset');
  });
  await button(page,'编辑草稿').click();await page.getByLabel('发布方式',{exact:true}).selectOption('STANDARD');await button(page,'保存草稿修改').click();
  await page.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();
  const switchedStandard=await api(context,'GET',path);assert.equal(switchedStandard.release_type,'STANDARD');snapshots.standard_flows=switchedStandard.table_flows;for(const table of tables)assert.notEqual(switchedStandard.table_flows.find(flow=>flow.table_name===table).instance_id,snapshots.initial_flows.find(flow=>flow.table_name===table).instance_id);
  page.once('dialog',dialog=>dialog.accept());await page.reload();await page.getByRole('heading',{name:title,exact:true}).waitFor();
  await button(page,'编辑草稿').click();assert.equal(await page.getByLabel('发布方式',{exact:true}).inputValue(),'STANDARD');
  await button(page,'保存草稿修改').click();await page.getByRole('heading',{name:'编辑多表草稿',exact:true}).waitFor({state:'hidden'});
  assert.equal(editWrites.length,2);assert.deepEqual(editWrites[0],editWrites[1]);assert.deepEqual((await api(context,'GET',path)).table_flows,snapshots.standard_flows);await page.unroute(`**${path}`);
  await button(page,'编辑草稿').click();await page.getByLabel('发布方式',{exact:true}).selectOption('EMERGENCY');await button(page,'保存草稿修改').click();
  information=await basicInformation(page);await information.getByText('应急发布',{exact:true}).waitFor();order=await api(context,'GET',path);
  assert.equal(order.release_type,'EMERGENCY');assert.equal(order.table_flows.length,2);
  assert.ok(order.table_flows.every(flow=>flow.template_code==='emergency_browser_emergency_v1'));snapshots.emergency_flows=order.table_flows;for(const table of tables){const current=order.table_flows.find(flow=>flow.table_name===table);assert.notEqual(current.instance_id,snapshots.initial_flows.find(flow=>flow.table_name===table).instance_id);assert.notEqual(current.instance_id,snapshots.standard_flows.find(flow=>flow.table_name===table).instance_id);}
  await shot(page,'emergency-draft-desktop.png');
  checks.push('AC-012/023: explicit whole-order type switches rebuild persisted instances; failed response retains the selected STANDARD request and exact key/body for manual replay');

  await button(page,'提交应急发布').click();
  const reason=page.getByLabel('应急原因',{exact:true}),confirm=button(page,'确认提交待发布');
  const invalidReasonWrites=[];const observeInvalid=request=>{if(new URL(request.url()).pathname===`${path}/submit`&&request.method()==='POST')invalidReasonWrites.push(request.postData());};page.on('request',observeInvalid);
  assert.equal(await confirm.isDisabled(),true);await reason.fill('   ');assert.equal(await confirm.isDisabled(),true);
  await reason.fill('🚨'.repeat(2001));await page.getByText('应急原因不能超过 2,000 个字符。',{exact:true}).waitFor();assert.equal(await confirm.isDisabled(),true);
  const emergencyReason='生产支付路由异常，需立即统一修正资金与渠道配置 🚨';await reason.fill(emergencyReason);assert.equal(await confirm.isEnabled(),true);
  assert.deepEqual(invalidReasonWrites,[]);page.off('request',observeInvalid);
  await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await reason.focus();await shot(page,'emergency-reason-mobile.png');
  await page.keyboard.press('Escape');await page.getByRole('heading',{name:'放弃未保存的修改？',exact:true}).waitFor();assert.equal(await button(page,'继续编辑').evaluate(node=>node===document.activeElement),true);await shot(page,'emergency-unsaved-mobile.png');await button(page,'继续编辑').click();assert.equal(await reason.inputValue(),emergencyReason);
  const submitWrites=[];let loseSubmit=true;
  await page.route(`**${path}/submit`,async route=>{
   submitWrites.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
   if(!loseSubmit)return route.continue();loseSubmit=false;
   const response=await route.fetch();assert.equal(response.status(),200);await route.abort('connectionreset');
  });
  await confirm.click();await page.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();
  order=await api(context,'GET',path);assert.equal(order.state,'PENDING_PUBLICATION');assert.equal(order.emergency_reason,emergencyReason);
  assert.deepEqual(order.approvals,[]);assert.deepEqual(order.approval_context.tables,[]);assert.deepEqual(order.approval_context.approvable_tables,[]);
  assert.equal(order.history.some(event=>event.action==='APPROVE'),false);assert.equal(order.history.find(event=>event.action==='SUBMIT').reason,emergencyReason);
  assert.deepEqual(JSON.parse(submitWrites[0].body),{expected_version:'3',emergency_reason:emergencyReason});
  await page.getByRole('dialog',{name:'提交应急发布',exact:true}).getByText('查看原申请内容',{exact:true}).click();
  await page.getByText(`应急原因：${emergencyReason}`,{exact:true}).waitFor();
  const before=[];for(const [index,table] of tables.entries())before.push((await api(context,'POST',`/api/v1/tables/${table}/query`,{conditions:[{field:'id',operator:'exact',value:String(10401+index)}]})).rows);
  assert.deepEqual(before,[[],[]]);
  checks.push('AC-015/016: UI rejects blank and 2,001-codepoint reasons; committed submit persists the exact reason, enters PENDING_PUBLICATION, records no approval, and changes no business row');

  const session=await context.request.get(origin+'/api/v1/auth/session');const csrf=(await session.json()).csrf_token;
  const logout=await context.request.post(origin+'/api/v1/auth/logout',{headers:{Origin:origin,'X-CSRF-Token':csrf}});assert.equal(logout.status(),204);
  page.once('dialog',dialog=>dialog.accept());await page.reload();await page.getByLabel('用户名').waitFor();await shot(page,'emergency-session-interrupted-mobile.png');await login();
  await page.getByRole('heading',{name:title,exact:true}).waitFor();information=await basicInformation(page);await information.getByText('应急原因',{exact:true}).waitFor();
  await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
  assert.equal(await page.getByText('逐表审批进度',{exact:true}).count(),0);assert.equal(await information.getByText('审批人',{exact:true}).count(),0);assert.equal(await page.getByText('尚无批准记录',{exact:true}).count(),0);assert.equal(await button(page,'批准发布单').count(),0);
  await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'待手动发布',exact:true}).waitFor();
  await button(page,'执行发布').focus();assert.equal(await button(page,'执行发布').evaluate(node=>node===document.activeElement),true);
  await shot(page,'emergency-pending-publication-mobile.png');
  checks.push('AC-023: same-account session recovery restores the unresolved reason; 390px has no overflow, no approval UI, visible long flow names, and keyboard-reachable PUBLISHER action');

  const conflict=await api(context,'POST','/api/v1/release-orders',{title:'占用期间不得创建的冲突单',release_type:'EMERGENCY',items:[{table_name:tables[0],operation:'ADD',content:{id:'10401',value:'conflict'}}]},409);
  assert.equal(conflict.error.code,'release_target_conflict');
  const publisher=await browser.newContext({viewport:{width:1440,height:1000},storageState:await context.storageState()});
  const publishPage=await publisher.newPage();publishPage.setDefaultTimeout(20000);publishPage.on('pageerror',error=>pageErrors.push(error.message));
  await publishPage.goto(origin+`/configuration/release-orders/${order.id}`);await button(publishPage,'执行发布').click();await button(publishPage,'确认发布到数据库').click();
  await publishPage.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'数据库已发布，待人工完结',exact:true}).waitFor();
  order=await api(context,'GET',path);assert.equal(order.state,'SUCCEEDED');assert.ok(order.allowed_actions.includes('complete'));
  const recordVersions=[];
  for(const [index,table] of tables.entries()){
   const result=await api(context,'POST',`/api/v1/tables/${table}/query`,{conditions:[{field:'id',operator:'exact',value:String(10401+index)}]});
   assert.equal(result.rows[0].value,`emergency value ${index+1}`);recordVersions.push(result.record_versions[0]);
  }
  await publisher.close();
  page.once('dialog',dialog=>dialog.accept());await page.reload();await page.getByRole('heading',{name:title,exact:true}).waitFor();
  await button(page,'提交应急发布').click();assert.equal(await page.getByLabel('应急原因',{exact:true}).inputValue(),emergencyReason);
  await Promise.all([page.waitForResponse(response=>new URL(response.url()).pathname===`${path}/submit`&&response.request().method()==='POST'&&response.status()===200),button(page,'确认提交待发布').click()]);await page.getByRole('dialog',{name:'提交应急发布',exact:true}).waitFor({state:'hidden'});await page.getByText('已发布待完结',{exact:true}).first().waitFor();
  assert.equal(submitWrites.length,2);assert.deepEqual(submitWrites[0],submitWrites[1]);assert.equal((await api(context,'GET',path)).state,'SUCCEEDED');await page.unroute(`**${path}/submit`);
  assert.equal(await button(page,'快速回滚').isVisible(),true);await shot(page,'emergency-published-mobile.png');
  checks.push('AC-016/017/023: the current applicant/PUBLISHER executes both tables in another same-account profile, remains pending manual completion, and manual replay of the original submit never replaces the current SUCCEEDED detail');

  await button(page,'完结发布单').click();await button(page,'确认完结').click();await page.getByRole('region',{name:'发布阶段',exact:true}).getByRole('heading',{name:'已完结',exact:true}).waitFor();
  order=await api(context,'GET',path);assert.equal(order.state,'COMPLETED');assert.equal(order.allowed_actions.includes('quick-rollback'),false);assert.equal(await button(page,'快速回滚').count(),0);for(const [index,table] of tables.entries()){const result=await api(context,'POST',`/api/v1/tables/${table}/query`,{conditions:[{field:'id',operator:'exact',value:String(10401+index)}]});assert.equal(result.rows[0].value,`emergency value ${index+1}`);assert.equal(result.record_versions[0],recordVersions[index]);}
  const next=await api(context,'POST','/api/v1/release-orders',{title:'完结后可重新占用原目标',release_type:'EMERGENCY',items:tables.map((table_name,index)=>({table_name,operation:'MODIFY',id:String(10401+index),expected_record_version:recordVersions[index],content:{value:`next ${index+1}`}}))},201);
  assert.equal(next.state,'DRAFT');await shot(page,'emergency-completed-mobile.png');
  checks.push('AC-018: manual completion closes the order and releases both persisted targets for a later draft without changing the published values');
  assert.deepEqual(pageErrors,[]);
  snapshots.final_order={id:order.id,state:order.state,release_type:order.release_type,emergency_reason:order.emergency_reason,history:order.history,table_flows:order.table_flows};
  const evidence={order_id:order.id,next_order_id:next.id,checks,snapshots,original_requests:{edit:editWrites,submit:submitWrites}};
  await writeFile(join(output,'release-emergency-browser-evidence.json'),JSON.stringify(evidence,null,2),{flag:'wx'});
  process.stdout.write(JSON.stringify({order_id:order.id,next_order_id:next.id,checks}));
 }catch(error){
  if(page){await shot(page,'release-emergency-failure.png').catch(()=>{});await writeFile(join(output,'release-emergency-failure.txt'),String(error),{flag:'wx'}).catch(()=>{});}
  throw error;
 }finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1});
