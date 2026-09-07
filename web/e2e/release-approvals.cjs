// Real applicant/reviewer browsers against the isolated Admin + MySQL fixture.
const {chromium}=require('playwright');
const assert=require('node:assert/strict');
const {randomUUID}=require('node:crypto');
const {join}=require('node:path');
const {browserOptions,registerFixtureAccount}=require('./local-account.cjs');
const base=process.env.RCC_WEB_URL;
(async()=>{
 const browser=await chromium.launch(browserOptions());const errors=[],checks=[];
 const check=name=>{checks.push(name);console.log('PASS',name)};
 const identity=async ctx=>(await (await ctx.request.get(`${base}/api/v1/auth/session`)).json());
 const call=async(ctx,path,data)=>{const session=await identity(ctx);return ctx.request.post(`${base}${path}`,{headers:{Origin:base,'X-CSRF-Token':session.csrf_token,'Idempotency-Key':randomUUID()},data})};
 const account=async roles=>{const ctx=await browser.newContext({viewport:{width:1440,height:1000}});await registerFixtureAccount(ctx,base);const session=await identity(ctx);const r=await ctx.request.put(`${base}/api/v1/account-roles/${session.account.id}`,{headers:{Origin:base,'X-CSRF-Token':session.csrf_token,'Idempotency-Key':randomUUID()},data:{roles,expected_version:'2'}});assert.equal(r.status(),200);return ctx};
 try{
  const applicant=await account(['EDITOR']),reviewer=await account(['APPROVER']);
  const page=await applicant.newPage(),review=await reviewer.newPage();for(const p of [page,review]){p.setDefaultTimeout(12000);p.on('pageerror',e=>errors.push(e.message))}
  const create=await call(applicant,'/api/v1/release-orders',{table_name:'stage1_acceptance_items',items:[{operation:'MODIFY',id:'2',expected_record_version:'0',content:{name:'approval browser intent'}}]});assert.equal(create.status(),201);const draft=await create.json();
  await page.goto(`${base}/configuration/release-orders/${draft.id}`);await page.getByRole('button',{name:'提交审批',exact:true}).click();await page.getByRole('button',{name:'确认提交审批',exact:true}).click();await page.getByRole('heading',{name:'stage1_acceptance_items · 待审批',exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'编辑草稿',exact:true}).count(),0);assert.equal(await page.getByRole('button',{name:'批准发布单',exact:true}).count(),0);
  check('EDITOR submits and sees frozen read-only differences with no self-approval action');
  await review.goto(page.url());await review.getByRole('button',{name:'拒绝发布单',exact:true}).click();assert.equal(await review.getByRole('button',{name:'确认拒绝',exact:true}).isEnabled(),false);await review.getByLabel('审批意见',{exact:true}).fill('Please recheck the current baseline');await review.getByRole('button',{name:'确认拒绝',exact:true}).click();await review.getByRole('heading',{name:'stage1_acceptance_items · 已拒绝',exact:true}).waitFor();
  await page.reload();await page.getByText('Please recheck the current baseline',{exact:true}).waitFor();await page.getByRole('button',{name:'复制新草稿',exact:true}).click();assert.equal(await page.getByRole('button',{name:'确认最新基线并复制',exact:true}).isEnabled(),false);await page.getByRole('button',{name:'读取最新配置',exact:true}).click();await page.getByRole('button',{name:'确认最新基线并复制',exact:true}).click();await page.waitForURL(url=>url.pathname.startsWith('/configuration/release-orders/')&&!url.pathname.endsWith(draft.id));
  const copyID=new URL(page.url()).pathname.split('/').pop();await page.getByRole('heading',{name:'stage1_acceptance_items · 草稿',exact:true}).waitFor();assert.equal(await page.getByText('Please recheck the current baseline',{exact:true}).count(),0);
  check('APPROVER rejects with a required reason; applicant explicitly reads and confirms a new draft without inherited approval');
  await page.getByRole('button',{name:'提交审批',exact:true}).click();await page.getByRole('button',{name:'确认提交审批',exact:true}).click();await page.getByRole('heading',{name:'stage1_acceptance_items · 待审批',exact:true}).waitFor();await review.goto(page.url());
  const writes=[];reviewer.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith(`/${copyID}/approve`))writes.push({body:r.postData(),key:r.headers()['idempotency-key']})});
  await review.route(`**/api/v1/release-orders/${copyID}/approve`,async route=>{const result=await route.fetch();assert.equal(result.status(),200);await route.abort('failed')});
  await review.getByRole('button',{name:'批准发布单',exact:true}).click();await review.getByLabel('审批意见',{exact:true}).fill('Verified independently');await review.getByRole('button',{name:'确认批准',exact:true}).click();await review.getByRole('button',{name:'使用原请求重试',exact:true}).waitFor();await review.unroute(`**/api/v1/release-orders/${copyID}/approve`);
  review.once('dialog',d=>d.accept());await review.reload();await review.getByRole('button',{name:'恢复原发布请求',exact:true}).click();await review.getByRole('heading',{name:'stage1_acceptance_items · 已批准',exact:true}).waitFor();await review.getByText('Verified independently',{exact:true}).waitFor();assert.equal(writes.length,2);assert.deepEqual(writes[0],writes[1]);
  check('APPROVER-only account recovers a committed lost approval response after refresh using original key and state version');
  await page.reload();await page.getByRole('button',{name:'取消发布单',exact:true}).click();await page.getByLabel('取消原因',{exact:true}).fill('Withdraw before publication');await page.getByRole('button',{name:'确认取消发布单',exact:true}).click();await page.getByRole('heading',{name:'stage1_acceptance_items · 已取消',exact:true}).waitFor();await page.reload();await page.getByText('Verified independently',{exact:true}).waitFor();await page.getByText('Withdraw before publication',{exact:true}).waitFor();
  const query=await call(applicant,'/api/v1/tables/stage1_acceptance_items/query',{conditions:[{field:'id',operator:'exact',value:'2'}]});assert.equal(query.status(),200);const record=await query.json();assert.notEqual(record.rows[0].name,'approval browser intent');assert.equal(record.record_versions[0],'0');
  await page.setViewportSize({width:390,height:844});await page.getByLabel('主导航',{exact:true}).waitFor({state:'hidden'});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));if(process.env.RCC_E2E_OUTPUT)await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'release-approvals-mobile.png'),fullPage:true});
  check('applicant cancels an approved order; history survives refresh and actual business data remains unchanged on narrow viewport');
  assert.deepEqual(errors,[]);console.log(JSON.stringify({checks}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
