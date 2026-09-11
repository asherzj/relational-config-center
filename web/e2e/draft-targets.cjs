const {readAllReleaseDetailPages,executionCommands,applicationItems}=require('./release-detail-pages.cjs');
// T2 public browser → Admin → isolated MySQL acceptance. No business test route.
const playwright=require(process.env.RCC_PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
const {randomUUID}=require('node:crypto');
const {join}=require('node:path');
const {browserOptions,selectedBrowser,registerFixtureAccount,authenticatedRequest}=require('./local-account.cjs');
const engine=process.env.RCC_E2E_ENGINE||'chromium',suffix=engine==='chromium'?'':`_${engine}`,mutation=`draft_browser_${engine}_v1`;
const base=process.env.RCC_WEB_URL,table='draft_browser_items'+suffix,longField='long_'+'x'.repeat(59),output=process.env.RCC_E2E_OUTPUT;
(async()=>{
 const browser=await selectedBrowser(playwright).launch(browserOptions());
 const checks=[],errors=[],check=name=>{checks.push(name);console.log('PASS',name)};
 const api=async(context,method,path,data,status=200)=>{
  const response=await authenticatedRequest(context,base,path,{method,data,headers:{'Idempotency-Key':randomUUID()}});
  assert.equal(response.status(),status,`${method} ${path}: ${await response.text()}`);return response.json();
 };
 const read=async(context,path)=>readAllReleaseDetailPages(context,base,await api(context,'GET',path));
 const button=(page,name)=>page.getByRole('button',{name,exact:true});
 const shot=async(page,name)=>{if(output)await page.screenshot({path:join(output,name),fullPage:false})};
 try{
  const admin=await browser.newContext({viewport:{width:1440,height:1000}});await registerFixtureAccount(admin,base,{roles:['ADMIN']});
  await api(admin,'POST','/api/v1/mutation-policies',{code:mutation,name:'草稿管控验收',description:'',type_code:'single_table_mutation',allow_add:true,allow_modify:true,allow_delete:true,create_time_field:'stamp'},201);
  await api(admin,'POST',`/api/v1/mutation-policies/${mutation}/activate`,{});
  await api(admin,'POST','/api/v1/table-policies',{table_name:table,query_policy_code:'notification_page_query_v1',mutation_policy_code:mutation},201);
  await api(admin,'POST',`/api/v1/table-policies/${table}/enable`,{expected_version:'1'});
  await api(admin,'PUT',`/api/v1/table-policies/${table}/release-templates/STANDARD`,{template_code:'default_standard_v1',enabled:true,expected_version:'0'});
  const settings=await admin.newPage();settings.setDefaultTimeout(12000);settings.on("dialog",dialog=>dialog.accept());settings.on('pageerror',error=>errors.push(error.message));
  await settings.goto(`${base}/platform/table-policies/${table}?mode=replace`);
  let fieldReads=0;await settings.route('**/concurrency-key-fields?*',async route=>{if(++fieldReads===1)await route.abort();else await route.continue()});
  await button(settings,'选择管控字段').click();await button(settings,'重试').click();assert.equal(await button(settings,'确认替换').count(),0);const fields=settings.getByLabel('添加管控字段',{exact:true});
  await fields.locator('option[value="code"]').waitFor({state:'attached'});
  for(const name of ['id','generated_label','stamp'])assert.equal(await fields.locator(`option[value="${name}"]`).getAttribute("disabled")!==null,true,`${name} should be ineligible`);
  assert.equal(fieldReads,2);await settings.unroute('**/concurrency-key-fields?*');await fields.selectOption('code');await fields.selectOption('label');await fields.selectOption(longField);await settings.setViewportSize({width:390,height:844});assert.ok(await settings.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await shot(settings,'draft-key-config-mobile.png');await button(settings,`移除字段 ${longField}`).click();await settings.setViewportSize({width:1440,height:1000});assert.equal(await button(settings,'确认替换').count(),0);
  await button(settings,'检查并替换').click();await button(settings,'确认替换').click();await settings.getByText('并发管控键：code + label',{exact:true}).waitFor();
  await button(settings,'替换所选规则').click();await button(settings,'移除字段 label').click();assert.equal(await button(settings,'确认替换').count(),0);await button(settings,'检查并替换').click();await button(settings,'确认替换').click();await settings.getByText('并发管控键：code',{exact:true}).waitFor();
  check('管理员按真实字段配置组合及单字段键；自增、生成和自动字段不可选');

  const editor=await browser.newContext({viewport:{width:1440,height:1000}});const person=await registerFixtureAccount(editor,base,{roles:['EDITOR']});
  const page=await editor.newPage();page.setDefaultTimeout(12000);page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`${base}/configuration/release-orders`);await button(page,'新建草稿').click();assert.equal(await page.getByLabel('起始表（可选）',{exact:true}).count(),0);await page.getByLabel('发布单标题',{exact:true}).fill('   ');assert.equal(await page.getByLabel('发布单标题',{exact:true}).getAttribute('aria-invalid'),'true');assert.equal(await button(page,'创建空草稿').isDisabled(),true);await page.getByLabel('发布单标题',{exact:true}).fill('T2 分页草稿');await button(page,'创建空草稿').click();await page.getByRole('heading',{name:'T2 分页草稿',exact:true}).waitFor();
  const id=new URL(page.url()).pathname.split('/').at(-1);let draft=await read(editor,`/api/v1/release-orders/${id}`);assert.deepEqual(draft.items,[]);assert.equal(await button(page,'提交审批').count(),0);
  draft=await api(editor,'PUT',`/api/v1/release-orders/${id}`,{title:draft.title,expected_version:draft.version,changes:{upserts:Array.from({length:25},(_,index)=>({table_name:table,operation:'MODIFY',id:String(index+1),expected_record_version:'0',content:{label:`proposal-${index+1}`}}))}});
  check('编辑者创建空草稿，增量添加25项后使用同一整单版本');
  const blocker=await api(admin,'POST','/api/v1/release-orders',{title:'T2 冲突负责人',items:[{table_name:table,operation:'ADD',content:{id:'500',code:'occupied',label:'owner'}}]},201);
  await page.reload();await button(page,'编辑草稿').click();await page.getByLabel('定位编辑明细',{exact:true}).fill('21');assert.equal(await page.getByLabel('label 申请值',{exact:true}).inputValue(),'proposal-21');
  await page.getByLabel('code 提交方式',{exact:true}).selectOption('value');await page.getByLabel('code 申请值',{exact:true}).fill('OCCUPIED');
  const writes=[];page.on('request',request=>{if(request.method()==='PUT'&&new URL(request.url()).pathname===`/api/v1/release-orders/${id}`)writes.push(JSON.parse(request.postData()))});
  await button(page,'保存草稿修改').click();await page.getByText(`冲突表：${table}`,{exact:true}).waitFor();await page.getByRole('link',{name:`占用发布单：${blocker.id}（新窗口查看）`,exact:true}).waitFor();assert.equal(await page.getByLabel('code 申请值',{exact:true}).inputValue(),'OCCUPIED');
  assert.equal(writes[0].items,undefined);assert.equal(writes[0].changes.upserts.length,1);assert.equal(writes[0].changes.upserts[0].detail_id,draft.items[20].detail_id);
  await page.getByText(`冲突表：${table}`,{exact:true}).scrollIntoViewIfNeeded();await shot(page,'draft-target-conflict-desktop.png');
  await page.getByLabel('code 申请值',{exact:true}).fill('available');await button(page,'查看最新发布单').click();await button(page,'基于最新发布单重建').click();await button(page,'保存草稿修改').click();await page.getByRole('heading',{name:'T2 分页草稿',exact:true}).waitFor();await page.getByRole('heading',{name:`编辑多表草稿`,exact:true}).waitFor({state:'hidden'});
  draft=await read(editor,`/api/v1/release-orders/${id}`);assert.equal(draft.items[20].content.code,'available');assert.equal(draft.items[0].content.label,'proposal-1');
  check('第2页只提交指定明细；数据库等价冲突显示表、单号、申请人并保留输入');

  const other=await editor.newPage();await other.goto(page.url());await button(other,'编辑草稿').click();await other.getByLabel('编辑明细',{exact:true}).selectOption('1');await other.getByLabel('label 申请值',{exact:true}).fill('other window detail two');
  await button(page,'编辑草稿').click();await page.getByLabel('label 申请值',{exact:true}).fill('retained detail one');await button(other,'保存草稿修改').click();await other.getByRole('heading',{name:`编辑多表草稿`,exact:true}).waitFor({state:'hidden'});
  await button(page,'保存草稿修改').click();await page.getByText('发布单已被其他窗口修改。你的输入已保留，请先查看最新发布单。',{exact:true}).waitFor();assert.equal(await page.getByLabel('label 申请值',{exact:true}).inputValue(),'retained detail one');
  await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));for(const field of ['label',longField])assert.ok(await page.getByLabel(`${field} 申请值`,{exact:true}).evaluate(element=>{const box=element.getBoundingClientRect();return box.left>=0&&box.right<=innerWidth}),`${field} input should fit the drawer viewport`);await page.getByText('发布单已被其他窗口修改。你的输入已保留，请先查看最新发布单。',{exact:true}).scrollIntoViewIfNeeded();await shot(page,'draft-version-conflict-mobile.png');
  await button(page,'查看最新发布单').click();await button(page,'基于最新发布单重建').click();await button(page,'保存草稿修改').click();await page.getByRole('heading',{name:`编辑多表草稿`,exact:true}).waitFor({state:'hidden'});
  draft=await read(editor,`/api/v1/release-orders/${id}`);assert.equal(draft.items[0].content.label,'retained detail one');assert.equal(draft.items[1].content.label,'other window detail two');
  await button(page,'编辑草稿').click();await page.getByLabel('定位编辑明细',{exact:true}).fill('21');await button(page,'上移明细').focus();await page.keyboard.press('Enter');await button(page,'保存草稿修改').click();await page.getByRole('heading',{name:`编辑多表草稿`,exact:true}).waitFor({state:'hidden'});
  const reordered=await read(editor,`/api/v1/release-orders/${id}`);assert.equal(reordered.items[19].detail_id,draft.items[20].detail_id);
  check('两个真实窗口冲突重建保留本次输入及他人未触及明细；390px可键盘排序');

  await settings.goto(`${base}/platform/table-policies/${table}?mode=replace`);await button(settings,'移除字段 code').click();await button(settings,'检查并替换').click();await button(settings,'确认替换').click();await settings.getByText('此表仍有未结束发布单的明细。取消、拒绝、完结或回滚后才能更改管控键。',{exact:true}).waitFor();
  await settings.goto(`${base}/configuration/release-orders/${id}`);await button(settings,'取消草稿').click();await settings.getByLabel('取消原因',{exact:true}).fill('管理员清理遗留草稿');await button(settings,'确认取消草稿').click();await settings.getByText(`${table} · 已取消`,{exact:true}).waitFor();await shot(settings,'draft-admin-cancel.png');
  const cancelled=await api(admin,'GET',`/api/v1/release-orders/${id}`);assert.equal(cancelled.applicant_id,person.accountID);assert.equal(cancelled.state,'CANCELLED');
  await api(admin,'POST',`/api/v1/release-orders/${blocker.id}/cancel`,{expected_version:blocker.version,reason:'end blocker fixture'});
  const policy=await api(admin,'GET',`/api/v1/table-policies/${table}`);await api(admin,'PUT',`/api/v1/table-policies/${table}`,{table_name:table,query_policy_code:policy.query_policy_code,mutation_policy_code:policy.mutation_policy_code,concurrency_key:[],expected_version:policy.version});
  check('未结束明细阻止配置变更；管理员取消他人的遗留草稿后释放引用');
  assert.deepEqual(errors,[]);console.log(JSON.stringify({checks}));
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
