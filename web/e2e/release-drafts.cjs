const {repeatReleaseAction,reopenDraftSave,repeatDraftSave}=require('./release-original-action.cjs');
// Real browser → same-origin Admin → isolated MySQL. Run by make test-browser.
const playwright=require(process.env.RCC_PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
const {join}=require('node:path');
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
  await page.getByLabel('name 值',{exact:true}).fill('browser draft intent');
  await page.getByRole('button',{name:'查看 Change Set',exact:true}).click();
  const titleInput=page.getByLabel('发布单标题',{exact:true});
  assert.equal(await titleInput.inputValue(),'stage1_acceptance_items 配置变更');assert.equal(await titleInput.evaluate(element=>element.required),true);assert.equal(await titleInput.getAttribute('aria-invalid'),'false');
  await titleInput.fill('   ');assert.equal(await titleInput.getAttribute('aria-invalid'),'true');await page.getByText('发布单标题必填。',{exact:true}).waitFor();assert.ok((await titleInput.getAttribute('aria-describedby')).includes('new-release-title-error'));assert.equal(await page.getByRole('button',{name:'确认并保存草稿',exact:true}).isEnabled(),false);
  await titleInput.fill('浏览器发布草稿标题');assert.equal(await titleInput.getAttribute('aria-invalid'),'false');
  let committed;
  await page.route('**/api/v1/release-orders',async route=>{
   if(route.request().method()!=='POST')return route.continue();
   const response=await route.fetch();assert.equal(response.status(),201);committed=await response.json();await route.abort('failed');
  });
  await page.getByRole('button',{name:'确认并保存草稿',exact:true}).click();
  await page.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();
  assert.ok(committed?.id);assert.equal(committed.applicant_id,identity.account.id);assert.equal(committed.title,'浏览器发布草稿标题');
  await page.unroute('**/api/v1/release-orders');
  page.once('dialog',dialog=>dialog.accept());await page.reload();
  await repeatDraftSave(page);
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
  const other=await context.newPage();await other.goto(page.url());await other.getByRole('button',{name:'编辑草稿',exact:true}).click();await other.getByLabel('name 申请值',{exact:true}).fill('second window saved');const otherSaved=other.waitForResponse(response=>response.request().method()==='PUT'&&response.url().endsWith('/'+committed.id));await other.getByRole('button',{name:'保存草稿修改',exact:true}).click();assert.equal((await otherSaved).status(),200);await other.getByRole('heading',{name:'编辑多表草稿',exact:true}).waitFor({state:'hidden'});await other.getByText('second window saved',{exact:true}).waitFor();
  const staleSave=page.waitForResponse(response=>response.request().method()==='PUT'&&response.url().endsWith('/'+committed.id));await page.getByRole('button',{name:'保存草稿修改',exact:true}).click();const staleResult=await staleSave;assert.equal(staleResult.status(),409,await staleResult.text());await page.getByText('发布单已被其他窗口修改。你的输入已保留，请先查看最新发布单。',{exact:true}).waitFor();assert.equal(await page.getByLabel('name 申请值',{exact:true}).inputValue(),'retained first window');
  await page.getByRole('button',{name:'查看最新发布单',exact:true}).click();await page.getByRole('button',{name:'基于最新发布单重建',exact:true}).click();await page.getByRole('button',{name:'保存草稿修改',exact:true}).click();await page.getByText('retained first window',{exact:true}).waitFor();
  check('two real windows retain input on CAS conflict and require explicit reconstruction');
  await page.getByRole('button',{name:'编辑草稿',exact:true}).click();await page.getByLabel('name 申请值',{exact:true}).fill('recovered rejected intent');
  await page.route(`**/api/v1/release-orders/${committed.id}`,route=>route.request().method()==='PUT'?route.abort('failed'):route.continue());
  await page.getByRole('button',{name:'保存草稿修改',exact:true}).click();await page.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();
  await page.unroute(`**/api/v1/release-orders/${committed.id}`);
  // Same-browser tabs share the durable unresolved request. An independent browser
  // profile can still race through the server CAS, without overwriting that journal.
  await other.reload();await other.getByRole('button',{name:'编辑草稿',exact:true}).click();await other.getByRole('button',{name:'保存草稿修改',exact:true}).waitFor();
  const independent=await browser.newContext({storageState:await context.storageState()});
  const concurrent=await independent.newPage();await concurrent.goto(page.url());await concurrent.getByRole('button',{name:'编辑草稿',exact:true}).click();await concurrent.getByLabel('name 申请值',{exact:true}).fill('newer concurrent draft');const concurrentResponse=concurrent.waitForResponse(response=>response.request().method()==='PUT'&&response.url().endsWith('/'+committed.id));await concurrent.getByRole('button',{name:'保存草稿修改',exact:true}).click();assert.equal((await concurrentResponse).status(),200);await concurrent.getByRole('heading',{name:'编辑多表草稿',exact:true}).waitFor({state:'hidden'});await concurrent.getByText('newer concurrent draft',{exact:true}).waitFor();await independent.close();
  page.once('dialog',dialog=>dialog.accept());await page.reload();const rejectedReplay=page.waitForResponse(response=>response.request().method()==='PUT'&&response.url().endsWith('/'+committed.id));await repeatReleaseAction(page,'编辑草稿','保存草稿修改');const rejectedResponse=await rejectedReplay;assert.equal(rejectedResponse.status(),409,await rejectedResponse.text());
  await page.getByRole('button',{name:'查看最新状态与配置',exact:true}).waitFor();await page.reload();
  await page.getByText('查看原申请内容',{exact:true}).click();await page.getByText('值：recovered rejected intent',{exact:true}).waitFor();
  await page.getByRole('button',{name:'查看最新状态与配置',exact:true}).click();await page.getByRole('button',{name:'确认重建并保存草稿',exact:true}).click();await page.getByText('recovered rejected intent',{exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'恢复原发布请求',exact:true}).count(),0);
  check('unknown edit rejected after concurrent change survives two refreshes and explicitly rebuilds original intent');

  await page.getByRole('button',{name:'更多操作',exact:true}).click();await page.getByRole('menuitem',{name:'取消草稿',exact:true}).click();await page.getByLabel('取消原因',{exact:true}).fill('browser cancellation');await page.getByRole('button',{name:'确认取消草稿',exact:true}).click();await page.getByLabel('发布单状态',{exact:true}).filter({hasText:/^已取消$/}).waitFor();
  await page.reload();await page.getByText('browser cancellation',{exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:'编辑草稿',exact:true}).count(),0);
  const filterCancelledDraft=async()=>{await page.getByLabel('表名',{exact:true}).fill('stage1_acceptance_items');await page.getByLabel('申请人账号 ID',{exact:true}).fill(identity.account.id);await page.getByLabel('状态',{exact:true}).selectOption('CANCELLED');await page.getByRole('button',{name:'查询发布单',exact:true}).click();await page.getByRole('link',{name:committed.title,exact:true}).waitFor()};
  await page.getByRole('link',{name:'返回发布单列表',exact:true}).click();await filterCancelledDraft();
  if(process.env.RCC_E2E_OUTPUT){await page.getByRole('link',{name:committed.title,exact:true}).click();await page.getByRole('heading',{name:'浏览器发布草稿标题',exact:true}).waitFor();await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'release-drafts-detail.png'),fullPage:true});await page.getByRole('link',{name:'返回发布单列表',exact:true}).click();await filterCancelledDraft()}
  await page.setViewportSize({width:390,height:844});await page.getByLabel('主导航',{exact:true}).waitFor({state:'hidden'});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'release list overflows narrow viewport');
  if(process.env.RCC_E2E_OUTPUT)await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'release-drafts-mobile.png'),fullPage:true});
  check('cancelled drafts remain searchable with permanent applicant and history on narrow screens');
  const viewer=await browser.newContext();await registerFixtureAccount(viewer,base,{roles:['VIEWER']});const view=await viewer.newPage();await view.goto(`${base}/configuration/release-orders/${committed.id}`);await view.getByRole('heading',{name:'浏览器发布草稿标题',exact:true}).waitFor();assert.equal(await view.getByRole('button',{name:'编辑草稿',exact:true}).count(),0);assert.equal(await view.getByRole('button',{name:'更多操作',exact:true}).count(),0);assert.equal(await view.getByRole('menuitem',{name:'取消草稿',exact:true}).count(),0);await viewer.close();
  check('current VIEWER can read release history without edit/cancel actions');
  assert.deepEqual(errors,[]);console.log(JSON.stringify({checks}));
 }catch(error){if(process.env.RCC_E2E_OUTPUT){const fs=require('node:fs/promises');for(const [index,page] of context.pages().entries()){if(page.isClosed())continue;await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,`draft-failure-${index}.png`),fullPage:true}).catch(()=>{});await fs.writeFile(join(process.env.RCC_E2E_OUTPUT,`draft-failure-${index}.txt`),await page.locator('body').innerText()).catch(()=>{})}}throw error;}finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
