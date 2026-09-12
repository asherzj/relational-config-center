const { createFixtureApprovalRole, fixtureApprovalInput } = require('./table-approval-fixture.cjs');
const {readAllReleaseDetailPages,executionCommands,applicationItems}=require('./release-detail-pages.cjs');
const {repeatReleaseAction,reopenDraftSave,repeatDraftSave}=require('./release-original-action.cjs');
// T3 real browser → same-origin Admin → one isolated MySQL database.
const playwright=require(process.env.RCC_PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
const {randomUUID,createHash}=require('node:crypto');
const {join}=require('node:path');
const {writeFileSync}=require('node:fs');
const {browserOptions,selectedBrowser,registerFixtureAccount,authenticatedRequest,setFixtureRoles}=require('./local-account.cjs');
const base=process.env.RCC_WEB_URL,output=process.env.RCC_E2E_OUTPUT;
const engine=process.env.RCC_E2E_ENGINE||'chromium',suffix=engine==='chromium'?'':`_${engine}`;
const tables=['multitable_browser_a','multitable_browser_b'].map(table=>table+suffix),largeTable='multitable_browser_large'+suffix,mutation=`multitable_browser_${engine}_v1`;
const button=(page,name)=>page.getByRole('button',{name,exact:true});
const digest=value=>createHash('sha256').update(value).digest('hex');
const draftItems=order=>order.items.map(item=>({detail_id:item.detail_id,table_name:item.table_name,operation:item.operation,...(item.operation==='ADD'?{}:{id:item.id}),expected_record_version:item.expected_record_version,content:item.content}));
(async()=>{
 const browser=await selectedBrowser(playwright).launch(browserOptions());
 const checks=[],errors=[],evidence={checks};
 const check=name=>{checks.push(name);console.log('PASS',name)};
 const api=async(context,method,path,data,status=200)=>{
  const response=await authenticatedRequest(context,base,path,{method,data,headers:{'Idempotency-Key':randomUUID()}});
  assert.equal(response.status(),status,`${method} ${path}: ${(await response.text()).slice(0,500)}`);return response.json();
 };
 const read=async(context,path)=>readAllReleaseDetailPages(context,base,await api(context,'GET',path));
 const reloadIdentity=async(page,person,roles)=>{
  const session=page.waitForResponse(response=>new URL(response.url()).pathname==='/api/v1/auth/session'&&response.request().method()==='GET');
  await page.reload();const response=await session;assert.equal(response.status(),200);const identity=await response.json();
  assert.equal(identity.account.id,person.accountID);assert.deepEqual([...identity.account.roles].sort(),[...roles].sort());
  await page.locator('.protected-workspace[data-session-status="ready"]').waitFor({state:'visible'});
  await page.getByText(person.credentials.username,{exact:true}).waitFor();
  await button(page,'本地账号入口').click();
  const labels={VIEWER:'查看者',EDITOR:'编辑者',PUBLISHER:'发布者'};
  await page.getByText('当前账号 · '+identity.account.roles.map(role=>labels[role]).join('、'),{exact:true}).waitFor();
  await page.keyboard.press('Escape');
  (evidence.session_checks??=[]).push({account_id:identity.account.id,roles:identity.account.roles,username:identity.account.username,workspace_ready:true});
 };

 const shot=async(page,name)=>{if(output)await page.screenshot({path:join(output,name),fullPage:false,animations:'disabled'})};
 const pageFor=async context=>{const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',error=>errors.push(error.message));page.on('dialog',dialog=>dialog.accept());return page};
 try{
  const admin=await browser.newContext({viewport:{width:1440,height:1000}}),adminPerson=await registerFixtureAccount(admin,base,{roles:['ADMIN']});
  await api(admin,'POST','/api/v1/mutation-policies',{code:mutation,name:'多表验收',description:'',type_code:'single_table_mutation',allow_add:true,allow_modify:true,allow_delete:true},201);
  await api(admin,'POST',`/api/v1/mutation-policies/${mutation}/activate`,{});
  for(const table of [...tables,largeTable]){
   await api(admin,'POST','/api/v1/table-policies',{table_name:table,query_policy_code:'notification_page_query_v1',mutation_policy_code:mutation},201);
   await api(admin,'POST',`/api/v1/table-policies/${table}/enable`,{expected_version:'1'});
   await api(admin,'PUT',`/api/v1/table-policies/${table}/release-templates/STANDARD`,{template_code:'default_standard_v1',enabled:true,expected_version:'0'});
  }
  const settings=await pageFor(admin);
  for(const table of tables){
   await settings.goto(`${base}/platform/table-policies/${table}?mode=replace`);await button(settings,'选择管控字段').click();await settings.getByLabel('添加管控字段',{exact:true}).selectOption('label');await button(settings,'检查并替换').click();await button(settings,'确认替换').click();await settings.getByText('并发管控键：label',{exact:true}).waitFor();
   assert.deepEqual((await api(admin,'GET',`/api/v1/table-policies/${table}`)).concurrency_key,['label']);
  }
  check('管理员在设置页为同一后续多表发布链路配置管控键');
  const editor=await browser.newContext({viewport:{width:1440,height:1000}}),person=await registerFixtureAccount(editor,base,{roles:['EDITOR','PUBLISHER']});
  const reviewer=await browser.newContext({viewport:{width:1440,height:1000}}),reviewerPerson=await registerFixtureAccount(reviewer,base,{roles:['VIEWER']});
  await createFixtureApprovalRole(admin,base,`Multi-table review ${randomUUID()}`,[reviewerPerson.accountID],[...tables,largeTable]);
  const page=await pageFor(editor),review=await pageFor(reviewer);
  const reviewPages=[];
  for(const surface of [page,review])surface.on('request',request=>{const url=new URL(request.url());if(url.pathname.endsWith('/details'))reviewPages.push({path:url.pathname,version:url.searchParams.get('expected_version'),offset:url.searchParams.get('offset'),limit:url.searchParams.get('limit')})});
  const iterations=Number(process.env.RCC_MULTI_ITERATIONS||20);
  for(let iteration=0;iteration<iterations;iteration++){
  await page.goto(`${base}/configuration/release-orders`);await button(page,'新建草稿').click();await page.getByLabel('发布单标题',{exact:true}).fill('多表整单浏览器验收');await button(page,'创建空草稿').click();await page.getByRole('heading',{name:'多表整单浏览器验收',exact:true}).waitFor();
  const id=new URL(page.url()).pathname.split('/').at(-1),path=`/api/v1/release-orders/${id}`;
  assert.deepEqual((await read(editor,path)).items,[]);
  for(const table of tables){
   await page.getByRole('link',{name:'添加变更',exact:true}).click();await page.getByLabel('Managed Table',{exact:true}).selectOption(table);
   await button(page,'修改记录 1').click();await page.getByLabel('label 值',{exact:true}).fill(`${table} proposal`);assert.equal(await page.getByLabel('包含 label',{exact:true}).count(),0);
   const started=Date.now(); await button(page,'查看 Change Set').click(); console.log('[DEBUG-multi] click',iteration,table,Date.now()-started); assert.equal(await page.getByLabel('保存到草稿',{exact:true}).inputValue(),id);await button(page,'确认并保存草稿').click();await page.getByRole('heading',{name:'多表整单浏览器验收',exact:true}).waitFor();
  }

   const order=await read(editor,path);assert.deepEqual(order.items.map(item=>item.table_name),tables);
   await api(editor,'POST',path+'/cancel',{expected_version:order.version,reason:'Diagnostic iteration complete'});
   console.log('[DEBUG-multi] completed',iteration);
  }
  assert.deepEqual(errors,[]);writeFileSync(join(output,'result.json'),JSON.stringify({ok:true,iterations,errors}));
 }catch(error){
  if(output){
   const snapshots=[];
   for(const [index,surface] of browser.contexts().flatMap(context=>context.pages()).entries()){
    try{snapshots.push(await surface.evaluate(()=>({url:location.href,visibility:document.visibilityState,body:document.body.innerText,animations:document.getAnimations().map(a=>({state:a.playState,time:a.currentTime,timing:a.effect?.getComputedTiming()})),buttons:[...document.querySelectorAll('button')].filter(e=>e.textContent==='查看 Change Set').map(e=>({rect:e.getBoundingClientRect().toJSON(),style:getComputedStyle(e).cssText}))})));await surface.screenshot({path:join(output,`failure-${index}.png`),timeout:5000});}catch(snapshotError){snapshots.push({snapshotError:String(snapshotError)});}
   }
   writeFileSync(join(output,'failure-diagnostic.json'),JSON.stringify({error:String(error),snapshots},null,2));
  }
  throw error;
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
