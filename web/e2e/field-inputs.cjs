const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const {randomUUID} = require('node:crypto');
const {chromium} = require(process.env.RCC_PLAYWRIGHT_MODULE || 'playwright');
const {browserOptions,registerFixtureAccount,authenticatedRequest} = require('./local-account.cjs');
const origin=process.env.RCC_WEB_URL,output=process.env.RCC_E2E_OUTPUT||'/tmp/rcc-field-inputs';
(async()=>{
 fs.mkdirSync(output,{recursive:true});const browser=await chromium.launch(browserOptions());
 const context=await browser.newContext({viewport:{width:1440,height:1000}});const checks=[],errors=[];
 try{
  await registerFixtureAccount(context,origin);const page=await context.newPage();page.setDefaultTimeout(15000);page.on('pageerror',e=>errors.push(e.message));
  const table='stage1_acceptance_items';const policyPath=`/api/v1/table-field-policies/${table}`;
  const read=await authenticatedRequest(context,origin,policyPath);assert.equal(read.status(),200);const fields=(await read.json()).fields;
  const policy=(name,patch)=>({...fields.find(f=>f.field_name===name).effective,enabled:true,...patch});
  let policies=[policy('name',{display_name:'配置名称',is_required:true}),policy('category',{ui_type:'select',ui_options:{options:[{label:'新渠道',value:'new-channel'}]},default_value:'new-channel'}),policy('note',{ui_type:'textarea',default_value:null}),policy('priority',{ui_type:'number',ui_options:{options:[],min:'0',max:'200',step:'1'},default_value:'0'}),policy('state',{editable_on_add:false,editable_on_modify:false,default_value:'should-not-submit'})];
  const savePolicies=async()=>{const response=await authenticatedRequest(context,origin,policyPath,{method:'PUT',data:{policies}});assert.equal(response.status(),200,await response.text());};await savePolicies();
  await page.goto(`${origin}/configuration/managed-data?table_name=${table}`);await page.getByRole('button',{name:'新增记录',exact:true}).click();
  await page.getByLabel('name 值',{exact:true}).waitFor();
  assert.equal(await page.getByLabel('category 值',{exact:true}).inputValue(),'0');assert.equal(await page.getByLabel('note 使用 NULL',{exact:true}).isChecked(),true);
  assert.equal(await page.getByLabel('包含 state',{exact:true}).count(),0);
  await page.getByRole('button',{name:'查看 Change Set',exact:true}).click();await page.getByRole('alert').filter({hasText:'请填写此字段'}).waitFor();
  await page.getByLabel('name 值',{exact:true}).fill('field-input-draft');
  await page.getByLabel('category 值',{exact:true}).selectOption('custom');await page.getByLabel('category 值 自定义值',{exact:true}).fill('custom-channel');
  await page.getByLabel('category 值 自定义值',{exact:true}).focus();await page.keyboard.press('Tab');assert.notEqual(await page.evaluate(()=>document.activeElement.tagName),'BODY');
  await page.screenshot({path:path.join(output,'desktop-add.png'),fullPage:false});
  await page.getByRole('button',{name:'关闭',exact:true}).last().click();await page.getByRole('button',{name:'继续编辑',exact:true}).click();assert.equal(await page.getByLabel('category 值 自定义值',{exact:true}).inputValue(),'custom-channel');
  // Updating configuration while open cannot overwrite this draft.
  policies=policies.map(p=>p.field_name==='category'?{...p,ui_type:'text',default_value:'changed-later',ui_options:{options:[]}}:p);await savePolicies();assert.equal(await page.getByLabel('category 值 自定义值',{exact:true}).inputValue(),'custom-channel');
  const addedResponse=page.waitForResponse(r=>r.url().endsWith('/api/v1/release-orders')&&r.request().method()==='POST'&&r.status()===201);
  await page.getByRole('button',{name:'查看 Change Set',exact:true}).click();await page.getByRole('button',{name:'确认并保存草稿',exact:true}).click();await page.getByRole('button',{name:'编辑草稿',exact:true}).waitFor();
  const added=await (await addedResponse).json();
  assert.ok(added?.id);const content=added.items[0].content;assert.equal(content.category,'custom-channel');assert.equal(content.priority,'0');assert.equal(content.note,null);assert.equal(Object.hasOwn(content,'state'),false);assert.equal(Object.hasOwn(content,'id'),false);
  checks.push('ADD uses configured controls, required errors, precise prefill, explicit NULL and omission; custom choice saved to real draft; unsaved close and opening-time configuration preserved');
  policies=policies.map(p=>p.field_name==='category'?{...p,ui_type:'select',ui_options:{options:[{label:'新渠道',value:'new-channel'}]}}:p);await savePolicies();
  await page.goto(`${origin}/configuration/managed-data?table_name=${table}`);
  const query=await authenticatedRequest(context,origin,`/api/v1/tables/${table}/query`,{method:'POST',data:{conditions:[],page_number:1}});const original=(await query.json()).rows[0];
  await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:`修改记录 ${original.id}`,exact:true}).click();await page.getByLabel('category 值 自定义值',{exact:true}).waitFor();
  assert.equal(await page.getByLabel('category 值 自定义值',{exact:true}).inputValue(),original.category??'');assert.equal(await page.getByLabel('state 值',{exact:true}).isDisabled(),true);assert.equal(await page.getByLabel('state 值',{exact:true}).inputValue(),original.state);
  await page.getByLabel('name 值',{exact:true}).fill('edited-other-field');await page.getByLabel('category 值 自定义值',{exact:true}).scrollIntoViewIfNeeded();assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:path.join(output,'mobile-modify.png'),fullPage:false});
  let modified;const response=page.waitForResponse(r=>r.url().endsWith('/api/v1/release-orders')&&r.request().method()==='POST'&&r.status()===201);
  await page.getByRole('button',{name:'查看 Change Set',exact:true}).click();await page.getByRole('button',{name:'确认并保存草稿',exact:true}).click();modified=await (await response).json();await page.getByRole('button',{name:'编辑草稿',exact:true}).waitFor();
  assert.equal(modified.items[0].content.category,original.category);assert.equal(Object.hasOwn(modified.items[0].content,'state'),false);assert.equal(modified.items[0].content.name,'edited-other-field');
  checks.push('390px MODIFY retains legacy custom value when another field changes, shows read-only original and saves real draft without readonly field');
  await page.goto(`${origin}/configuration/managed-data?table_name=${table}`);await page.getByRole('button',{name:'新增记录',exact:true}).click();await page.getByLabel('category 值 自定义值',{exact:true}).waitFor();assert.equal(await page.getByLabel('category 值 自定义值',{exact:true}).inputValue(),'changed-later');
  await page.getByRole('button',{name:'关闭',exact:true}).last().click();await page.getByRole('button',{name:'新增记录',exact:true}).waitFor();
  checks.push('reopening reads newest configuration; pristine prefill has no unsaved prompt');
  assert.notEqual(added.id,modified.id);
  const retired=[];
  for(const draft of [added,modified]){
   const cancelled=await authenticatedRequest(context,origin,`/api/v1/release-orders/${draft.id}/cancel`,{method:'POST',headers:{'Idempotency-Key':randomUUID()},data:{expected_version:draft.version,reason:'字段录入验收结束'}});
   assert.equal(cancelled.status(),200,await cancelled.text());const final=await cancelled.json();assert.equal(final.state,'CANCELLED');retired.push({id:final.id,state:final.state});
  }
  fs.writeFileSync(path.join(output,'retired-drafts.json'),JSON.stringify(retired,null,2));
  assert.deepEqual(errors,[]);fs.writeFileSync(path.join(output,'result.json'),JSON.stringify({browser:browser.version(),checks,errors},null,2));console.log(JSON.stringify({checks,errors}));
 }catch(error){const page=context.pages()[0];if(page){await page.screenshot({path:path.join(output,'failure.png'),fullPage:true}).catch(()=>{});fs.writeFileSync(path.join(output,'failure.txt'),await page.locator('body').innerText().catch(()=>''));}throw error;}finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1});
