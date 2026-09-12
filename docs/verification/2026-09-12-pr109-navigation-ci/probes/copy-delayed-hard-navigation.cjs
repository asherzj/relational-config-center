const { clickWithDiagnostics, diagnosticDeadline } = require('./click-diagnostics.cjs');
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
 const checks=[],errors=[],evidence={checks,click_diagnostics:[]};
 const timeline=[];let phase='setup',pageNumber=0,requestNumber=0;const started=Date.now();
 const event=(kind,detail)=>timeline.push({atMs:Date.now()-started,phase,kind,...detail});
 const check=name=>{checks.push(name);console.log('PASS',name);phase=name};
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
 const pageFor=async context=>{const page=await context.newPage(),number=pageNumber++;const ids=new WeakMap();page.setDefaultTimeout(20000);page.on('pageerror',error=>{errors.push(error.message);event('pageerror',{page:number,url:page.url(),message:error.message})});page.on('dialog',dialog=>dialog.accept());
  const detail=request=>({page:number,request:ids.get(request),path:new URL(request.url()).pathname,url:page.url(),method:request.method()});
  page.on('request',request=>{if(/^\/api\/v1\/(release-orders|approval-notifications)(\/|$)/.test(new URL(request.url()).pathname)){ids.set(request,requestNumber++);event('request',detail(request))}});
  page.on('response',response=>{if(ids.has(response.request()))event('response',{...detail(response.request()),status:response.status()})});
  page.on('requestfinished',request=>{if(ids.has(request))event('finished',detail(request))});
  page.on('requestfailed',request=>{if(ids.has(request))event('failed',{...detail(request),error:request.failure()?.errorText})});
  page.on('framenavigated',frame=>{if(frame===page.mainFrame())event('navigated',{page:number,url:frame.url()})});
  for(const method of ['goto','reload']){const original=page[method].bind(page);page[method]=async(...args)=>{event(method+'-start',{page:number,from:page.url(),to:args[0]});try{return await original(...args)}finally{event(method+'-end',{page:number,url:page.url()})}}}
  return page};
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
  phase='copy-loop';
  let source=await api(editor,'POST','/api/v1/release-orders',{title:'Navigation loop source',items:tables.map(table=>({table_name:table,operation:'MODIFY',id:'13',expected_record_version:'0',content:{label:'navigation-copy'}}))},201);
  source=await api(editor,'POST',`/api/v1/release-orders/${source.id}/submit`,{expected_version:source.version});
  source=await api(reviewer,'POST',`/api/v1/release-orders/${source.id}/reject`,await fixtureApprovalInput(reviewer,base,source.id,{expected_version:source.version,reason:'Navigation diagnostic source'}));
  event('copy-source',{id:source.id});
  await page.goto(`${base}/configuration/release-orders/${source.id}`);
  await page.route('**/api/v1/**',async route=>{
   const path=new URL(route.request().url()).pathname;
   if(route.request().method()!=='GET'||!(path==='/api/v1/approval-notifications'||(/^\/api\/v1\/release-orders\/[^/]+$/.test(path)&&!path.endsWith(source.id))))return route.continue();
   const response=await route.fetch();event('delay-response',{path,status:response.status(),delayMs:500});
   await new Promise(resolve=>setTimeout(resolve,500));
   try{await route.fulfill({response});event('delay-delivered',{path})}catch(error){event('delay-delivery-error',{path,error:String(error)})}
  });
  for(let iteration=0;iteration<3;iteration++){
   phase=`copy-loop-${iteration}`;
   await button(page,'复制新草稿').click();const drawer=page.getByRole('dialog',{name:'复制新草稿',exact:true});
   await drawer.getByRole('button',{name:'读取最新配置',exact:true}).click();await drawer.getByRole('button',{name:'确认最新基线并复制',exact:true}).click();
   await page.waitForURL(url=>url.pathname.startsWith('/configuration/release-orders/')&&!url.pathname.endsWith(source.id));
   const copied=await read(editor,'/api/v1/release-orders/'+new URL(page.url()).pathname.split('/').pop());event('copied',{id:copied.id,source:source.id});
   await page.goto(`${base}/configuration/release-orders/${source.id}`);const forward=page.getByRole('link',{name:copied.id,exact:true});await forward.waitFor();await forward.click();
   await page.getByText('复制自',{exact:true}).waitFor({state:'attached'});await page.getByText('基本信息',{exact:true}).click();const back=page.getByLabel('基本信息',{exact:true}).getByRole('link',{name:source.id,exact:true});await back.waitFor();
   assert.deepEqual(copied.items.map(item=>item.table_name),tables);
   await api(editor,'POST',`/api/v1/release-orders/${copied.id}/cancel`,{expected_version:copied.version,reason:'Diagnostic iteration cleanup'});
   await back.click();event('iteration-complete',{iteration});console.log('[DEBUG-navigation] iteration',iteration,'errors',errors.length);
   assert.deepEqual(errors,[]);
  }
  evidence.page_errors=errors;writeFileSync(join(output,'result.json'),JSON.stringify(evidence,null,2));
 }catch(error){
  evidence.failure={name:error.name,message:error.message,stack:error.stack};
  evidence.page_errors=errors;
  evidence.failure_pages=[];
  if(output){try{
   // Persist the original failure before best-effort renderer diagnostics.
   writeFileSync(join(output,'result.json'),JSON.stringify(evidence,null,2));
   for(const [index,surface] of browser.contexts().flatMap(context=>context.pages()).entries()){
    const snapshot={index,url:surface.url()};
    try{Object.assign(snapshot,await diagnosticDeadline(surface.locator('html').evaluate(()=>({visibility:document.visibilityState,hasFocus:document.hasFocus()}),undefined,{timeout:2000})));}catch(error){snapshot.stateError=String(error);}
    try{await surface.screenshot({path:join(output,`failure-${index}.png`),timeout:5000});snapshot.screenshot=`failure-${index}.png`;}catch(error){snapshot.screenshotError=String(error);}
    evidence.failure_pages.push(snapshot);
    writeFileSync(join(output,'result.json'),JSON.stringify(evidence,null,2));
   }
  }catch(diagnosticError){console.error('Failed to save multitable diagnostics:',String(diagnosticError));}}
  throw error;
 }finally{if(output)writeFileSync(join(output,'navigation-timeline.json'),JSON.stringify(timeline,null,2));await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
