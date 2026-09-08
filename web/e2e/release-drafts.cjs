// Real browser → same-origin Admin → isolated MySQL. Run by make test-browser.
const playwright=require(process.env.RCC_PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
const {join}=require('node:path');
const {writeFileSync}=require('node:fs');
const {browserOptions,selectedBrowser,registerFixtureAccount}=require('./local-account.cjs');
const base=process.env.RCC_WEB_URL;
(async()=>{
 const browser=await selectedBrowser(playwright).launch(browserOptions());
 const context=await browser.newContext({viewport:{width:1440,height:1000}});
 const errors=[],checks=[];
 const check=name=>{checks.push(name);console.log('PASS',name)};
 const session=async ctx=>(await (await ctx.request.get(`${base}/api/v1/auth/session`)).json());
 try{
  await registerFixtureAccount(context,base,{roles:['EDITOR']});
  const identity=await session(context);assert.deepEqual(identity.account.roles,['EDITOR']);
  const page=await context.newPage();page.setDefaultTimeout(12000);page.on('pageerror',error=>errors.push(error.message));
  const writes=[];context.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1/release-orders')writes.push({body:r.postData(),key:r.headers()['idempotency-key']})});
  await page.goto(`${base}/configuration/managed-data`);
  await page.getByLabel('Managed Table',{exact:true}).selectOption('stage1_acceptance_items');
  await page.getByRole('button',{name:'修改记录 1',exact:true}).click();
  await page.getByLabel('包含 name',{exact:true}).check();await page.getByLabel('name 值',{exact:true}).fill('browser draft intent');
  await page.getByRole('button',{name:'查看 Change Set',exact:true}).click();
  let committed;
  await page.route('**/api/v1/release-orders',async route=>{
   if(route.request().method()!=='POST')return route.continue();
   const response=await route.fetch();assert.equal(response.status(),201);committed=await response.json();await route.abort('failed');
  });
  await page.getByRole('button',{name:'确认并保存草稿',exact:true}).click();
  await page.getByRole('button',{name:'使用原请求重试',exact:true}).waitFor();
  assert.ok(committed?.id);assert.equal(committed.applicant_id,identity.account.id);
  await page.unroute('**/api/v1/release-orders');
  page.once('dialog',dialog=>dialog.accept());await page.reload();
  await page.getByRole('button',{name:'恢复原发布请求',exact:true}).click();
  await page.waitForURL(`**/configuration/release-orders/${committed.id}`);
  await page.getByText('browser draft intent',{exact:true}).waitFor();
  assert.equal(writes.length,2);assert.deepEqual(writes[0],writes[1]);
  check('EDITOR saves from data page; lost response survives refresh and original-key retry');
  await page.reload();await page.getByText('browser draft intent',{exact:true}).waitFor();
  const current=await session(context);
  const queried=await context.request.post(`${base}/api/v1/tables/stage1_acceptance_items/query`,{headers:{Origin:base,'X-CSRF-Token':current.csrf_token},data:{conditions:[{field:'id',operator:'exact',value:'1'}]}});
  assert.equal(queried.status(),200);const data=await queried.json();assert.equal(data.rows[0].name,'Alpha');assert.equal(data.record_versions[0],'0');
  check('durable detail reload retains server diff; business row and version remain unchanged');
  await page.getByRole('button',{name:'编辑草稿',exact:true}).click();await page.getByLabel('name 申请值',{exact:true}).fill('retained first window');
  const other=await context.newPage();await other.goto(page.url());await other.getByRole('button',{name:'编辑草稿',exact:true}).click();await other.getByLabel('name 申请值',{exact:true}).fill('second window saved');await other.getByRole('button',{name:'保存草稿修改',exact:true}).click();await other.getByText('second window saved',{exact:true}).waitFor();
  await page.getByRole('button',{name:'保存草稿修改',exact:true}).click();await page.getByText('发布单已被其他窗口修改。你的输入已保留，请先查看最新发布单。',{exact:true}).waitFor();assert.equal(await page.getByLabel('name 申请值',{exact:true}).inputValue(),'retained first window');
  await page.getByRole('button',{name:'查看最新发布单',exact:true}).click();await page.getByRole('button',{name:'基于最新发布单重建',exact:true}).click();await page.getByRole('button',{name:'保存草稿修改',exact:true}).click();await page.getByText('retained first window',{exact:true}).waitFor();
  check('two real windows retain input on CAS conflict and require explicit reconstruction');
  await page.getByRole('button',{name:'编辑草稿',exact:true}).click();await page.getByLabel('name 申请值',{exact:true}).fill('recovered rejected intent');
  await page.route(`**/api/v1/release-orders/${committed.id}`,route=>route.request().method()==='PUT'?route.abort('failed'):route.continue());
  await page.getByRole('button',{name:'保存草稿修改',exact:true}).click();await page.getByRole('button',{name:'使用原请求重试',exact:true}).waitFor();
  await page.unroute(`**/api/v1/release-orders/${committed.id}`);
  await other.reload();await other.getByRole('button',{name:'编辑草稿',exact:true}).click();await other.getByLabel('name 申请值',{exact:true}).fill('newer concurrent draft');await other.getByRole('button',{name:'保存草稿修改',exact:true}).click();await other.getByText('newer concurrent draft',{exact:true}).waitFor();
  page.once('dialog',dialog=>dialog.accept());await page.reload();await page.getByRole('button',{name:'恢复原发布请求',exact:true}).click();
  await page.getByRole('button',{name:'查看最新状态与配置',exact:true}).waitFor();await page.reload();
  await page.getByText('查看原申请内容',{exact:true}).click();await page.getByText('值：recovered rejected intent',{exact:true}).waitFor();
  await page.getByRole('button',{name:'查看最新状态与配置',exact:true}).click();await page.getByRole('button',{name:'确认重建并保存草稿',exact:true}).click();await page.getByText('recovered rejected intent',{exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'恢复原发布请求',exact:true}).count(),0);
  check('unknown edit rejected after concurrent change survives two refreshes and explicitly rebuilds original intent');

  await page.getByRole('button',{name:'取消草稿',exact:true}).click();await page.getByLabel('取消原因',{exact:true}).fill('browser cancellation');await page.getByRole('button',{name:'确认取消草稿',exact:true}).click();await page.getByRole('heading',{name:'stage1_acceptance_items · 已取消',exact:true}).waitFor();
  await page.reload();await page.getByText('browser cancellation',{exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:'编辑草稿',exact:true}).count(),0);
  const cancelledDraftLookups=[];
  const findCancelledDraft=async()=>{
   // Returning from detail remounts the list with its default filters/page.
   await page.getByLabel('表名',{exact:true}).fill('stage1_acceptance_items');
   await page.getByLabel('申请人账号 ID',{exact:true}).fill(identity.account.id);
   await page.getByLabel('状态',{exact:true}).selectOption('CANCELLED');
   const lookup=page.waitForResponse(response=>{
    const url=new URL(response.url());
    return response.request().method()==='GET'&&url.pathname==='/api/v1/release-orders'
     &&url.searchParams.get('table_name')==='stage1_acceptance_items'
     &&url.searchParams.get('applicant_id')===identity.account.id
     &&url.searchParams.get('state')==='CANCELLED';
   });
   await page.getByRole('button',{name:'查询发布单',exact:true}).click();
   const response=await lookup;assert.equal(response.status(),200);
   const returnedIDs=(await response.json()).orders.map(order=>order.id);
   assert.deepEqual(returnedIDs,[committed.id]);
   cancelledDraftLookups.push({status:response.status(),applicantID:identity.account.id,state:'CANCELLED',expectedOrderID:committed.id,returnedIDs});
   if(process.env.RCC_E2E_OUTPUT)writeFileSync(join(process.env.RCC_E2E_OUTPUT,'cancelled-draft-lookups.json'),JSON.stringify(cancelledDraftLookups,null,2));
   await page.getByRole('link',{name:committed.id,exact:true}).waitFor();
  };
  await page.getByRole('link',{name:'返回发布单列表',exact:true}).click();await findCancelledDraft();
  if(process.env.RCC_E2E_OUTPUT){await page.getByRole('link',{name:committed.id,exact:true}).click();await page.getByRole('heading',{name:'stage1_acceptance_items · 已取消',exact:true}).waitFor();await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'release-drafts-detail.png'),fullPage:true});await page.getByRole('link',{name:'返回发布单列表',exact:true}).click();await findCancelledDraft()}
  await page.setViewportSize({width:390,height:844});await page.getByLabel('主导航',{exact:true}).waitFor({state:'hidden'});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'release list overflows narrow viewport');
  if(process.env.RCC_E2E_OUTPUT)await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'release-drafts-mobile.png'),fullPage:true});
  check('cancelled drafts remain searchable with permanent applicant and history on narrow screens');
  const viewer=await browser.newContext();await registerFixtureAccount(viewer,base,{roles:['VIEWER']});const view=await viewer.newPage();await view.goto(`${base}/configuration/release-orders/${committed.id}`);await view.getByRole('heading',{name:'stage1_acceptance_items · 已取消',exact:true}).waitFor();assert.equal(await view.getByRole('button',{name:'编辑草稿',exact:true}).count(),0);assert.equal(await view.getByRole('button',{name:'取消草稿',exact:true}).count(),0);await viewer.close();
  check('current VIEWER can read release history without edit/cancel actions');
  assert.deepEqual(errors,[]);console.log(JSON.stringify({checks}));
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
