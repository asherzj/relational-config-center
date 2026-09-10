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
   await api(admin,'POST',`/api/v1/table-policies/${table}/enable`,{});
  }
  const settings=await pageFor(admin);
  for(const table of tables){
   await settings.goto(`${base}/platform/table-policies/${table}?mode=replace`);await button(settings,'选择管控字段').click();await settings.getByLabel('添加管控字段',{exact:true}).selectOption('label');await button(settings,'检查并替换').click();await button(settings,'确认替换').click();await settings.getByText('并发管控键：label',{exact:true}).waitFor();
   assert.deepEqual((await api(admin,'GET',`/api/v1/table-policies/${table}`)).concurrency_key,['label']);
  }
  check('管理员在设置页为同一后续多表发布链路配置管控键');
  const editor=await browser.newContext({viewport:{width:1440,height:1000}}),person=await registerFixtureAccount(editor,base,{roles:['EDITOR','PUBLISHER']});
  const reviewer=await browser.newContext({viewport:{width:1440,height:1000}});await registerFixtureAccount(reviewer,base,{roles:['APPROVER']});
  const page=await pageFor(editor),review=await pageFor(reviewer);
  const reviewPages=[];
  for(const surface of [page,review])surface.on('request',request=>{const url=new URL(request.url());if(url.pathname.endsWith('/details'))reviewPages.push({path:url.pathname,version:url.searchParams.get('expected_version'),offset:url.searchParams.get('offset'),limit:url.searchParams.get('limit')})});
  await page.goto(`${base}/configuration/release-orders`);await button(page,'新建草稿').click();await page.getByLabel('发布单标题',{exact:true}).fill('多表整单浏览器验收');await button(page,'创建空草稿').click();await page.getByRole('heading',{name:'多表整单浏览器验收',exact:true}).waitFor();
  const id=new URL(page.url()).pathname.split('/').at(-1),path=`/api/v1/release-orders/${id}`;
  assert.deepEqual((await read(editor,path)).items,[]);
  for(const table of tables){
   await page.getByRole('link',{name:'添加变更',exact:true}).click();await page.getByLabel('Managed Table',{exact:true}).selectOption(table);
   await button(page,'修改记录 1').click();await page.getByLabel('label 值',{exact:true}).fill(`${table} proposal`);assert.equal(await page.getByLabel('包含 label',{exact:true}).count(),0);
   await button(page,'查看 Change Set').click();assert.equal(await page.getByLabel('保存到草稿',{exact:true}).inputValue(),id);await button(page,'确认并保存草稿').click();await page.getByRole('heading',{name:'多表整单浏览器验收',exact:true}).waitFor();
  }
  let draft=await read(editor,path);assert.deepEqual(draft.items.map(item=>item.table_name),tables);assert.deepEqual(draft.items.map(item=>item.id),['1','1']);
  const additional=Array.from({length:23},(_,index)=>({table_name:tables[index%2],operation:'MODIFY',id:String(2+Math.floor(index/2)),expected_record_version:'0',content:{label:`page-detail-${index+3}`}}));
  draft=await api(editor,'PUT',path,{title:draft.title,expected_version:draft.version,changes:{upserts:additional}});
  await page.reload();await button(page,'编辑草稿').click();await page.getByLabel('定位编辑明细',{exact:true}).fill('25');await page.getByLabel('label 申请值',{exact:true}).fill('last global detail');
  await page.setViewportSize({width:390,height:844});await button(page,'上移明细').focus();await page.keyboard.press('Enter');
  // Wait for the real sheet's entrance/resize animation before measuring its
  // controls; document width alone cannot detect clipped inner form fields.
  await page.waitForFunction(()=>{const r=document.querySelector('[role="dialog"][data-modal-surface="true"]')?.getBoundingClientRect();return r&&r.left>=0&&r.right<=innerWidth});
  await shot(page,'multitable-editor-mobile-settled.png');
  for(const label of ['label 申请值','编辑明细']){
   const bounds=await page.getByLabel(label,{exact:true}).evaluate(element=>{const r=element.getBoundingClientRect();return {left:r.left,right:r.right,width:innerWidth}});assert.ok(bounds.left>=0&&bounds.right<=bounds.width,`${label}: ${JSON.stringify(bounds)}`);
  }
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await shot(page,'multitable-editor-mobile.png');
  await button(page,'保存草稿修改').click();await page.getByRole('heading',{name:'编辑多表草稿',exact:true}).waitFor({state:'hidden'});
  const reordered=await read(editor,path);assert.equal(reordered.items[23].detail_id,draft.items[24].detail_id);assert.equal(reordered.items[23].content.label,'last global detail');
  check('无起始表空单从明细加入两表同ID，25项分页编辑和390px键盘全局排序');
  for(const [index,table] of tables.entries())await api(admin,'PUT',`/api/v1/table-field-policies/${table}`,{policies:[{field_name:'label',display_name:`表${index?'乙':'甲'}名称`,description:'多表当前标签',display_order:1,is_visible:false,is_queryable:true,query_operators:['exact'],ui_type:'text',ui_options:{options:[]},editable_on_add:true,editable_on_modify:true,is_required:false,enabled:true}]});
  await page.reload();
  await button(page,'提交审批').click();await button(page,'确认提交审批').click();await page.getByLabel('发布单状态',{exact:true}).filter({hasText:/^待审批$/}).waitFor();
  await review.goto(`${base}/configuration/release-orders/${id}`);await button(review,'批准发布单').click();const approval=review.getByRole('dialog',{name:'批准发布单',exact:true});await approval.getByLabel('定位明细',{exact:true}).fill('25');await approval.getByRole('region',{name:'明细 25',exact:true}).waitFor();await review.getByLabel('审批意见',{exact:true}).fill('reviewed all tables and pages');await button(review,'确认批准').click();await review.getByLabel('发布单状态',{exact:true}).filter({hasText:/^已批准$/}).waitFor();
  await page.reload();await button(page,'执行发布').click();await button(page,'确认发布到数据库').click();await page.getByRole('heading',{name:'数据库发布结果',exact:true}).waitFor();
  let published=await read(editor,path);assert.equal(executionCommands(published).length,25);assert.equal(Object.keys(published.executions[0].table_versions).length,2);assert.equal(Object.keys(published.executions[0].notifications).length,2);
  const secondTableResult=page.getByRole('article').filter({has:page.getByRole('heading',{name:`明细 2 · ${tables[1]} · MODIFY · 记录 1`,exact:true})});
  await secondTableResult.getByRole('rowheader',{name:/表乙名称/}).waitFor();assert.equal(await secondTableResult.getByText('表甲名称',{exact:true}).count(),0);
  check('同名字段按实际所属表展示当前标签，列表隐藏规则不隐藏实际 before/final');
  await page.setViewportSize({width:1440,height:1000});await page.getByLabel('定位结果',{exact:true}).fill('25');await page.getByRole('heading',{name:/^明细 25 ·/}).waitFor();await shot(page,'multitable-publication-desktop.png');
  await button(page,'快速回滚').click();await button(page,'确认整单快速回滚').click();await page.getByLabel('发布单状态',{exact:true}).filter({hasText:/^已回滚$/}).waitFor();
  const restored=await read(editor,path);assert.equal(restored.id,id);assert.equal(restored.executions.length,2);assert.deepEqual(applicationItems(restored),applicationItems(published));assert.deepEqual(executionCommands(restored),executionCommands(published));assert.deepEqual(executionCommands(restored,"ROLLBACK").map(command=>command.table_name),executionCommands(published).map(command=>command.table_name).reverse());
  for(const label of ['申请内容','实际发布结果','恢复结果']){await button(page,label).click();if(label==='恢复结果')await page.getByRole('heading',{name:'数据库恢复结果',exact:true}).waitFor()}
  await page.setViewportSize({width:390,height:844});await page.getByLabel('主导航',{exact:true}).waitFor({state:'hidden'});await page.getByLabel('定位结果',{exact:true}).fill('1');
  const firstRestored=restored.items[0].rollback;
  const restoredArticle=page.getByRole('article').filter({has:page.getByRole('heading',{name:`明细 1 · ${firstRestored.table_name} · ${firstRestored.operation} · 记录 ${firstRestored.id}`,exact:true})});
  await restoredArticle.evaluate(element=>element.scrollIntoView({block:'start'}));await page.evaluate(()=>scrollBy(0,-80));
  const restoredValue=restoredArticle.getByText(`值：before-${firstRestored.id}`,{exact:true});await restoredValue.waitFor();
  const restoredRegion=restoredArticle.getByRole('region');await restoredRegion.focus();for(let step=0;step<8;step++)await page.keyboard.press('ArrowRight');assert.ok(await restoredRegion.evaluate(element=>element.scrollLeft>0));
  assert.ok(await restoredValue.evaluate(element=>{const r=element.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight}));
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await shot(page,'multitable-restoration-mobile-final.png');
  check('独立审批后25项整单发布，原单倒序恢复并切换申请/原发布/恢复三类结果');
  await page.setViewportSize({width:1440,height:1000});await button(page,'补填回滚原因').click();await page.getByLabel('回滚原因（选填）',{exact:true}).fill('多表原单恢复后的可选原因补填');await button(page,'保存回滚原因').click();await page.getByRole('region',{name:'回滚原因',exact:true}).getByText('多表原单恢复后的可选原因补填',{exact:true}).waitFor();
  const reasonSaved=await read(editor,path);assert.equal(reasonSaved.id,id);assert.equal(reasonSaved.state,'ROLLED_BACK');assert.deepEqual(reasonSaved.executions,restored.executions);assert.deepEqual(applicationItems(reasonSaved),applicationItems(published));assert.deepEqual(executionCommands(reasonSaved),executionCommands(published));assert.deepEqual(executionCommands(reasonSaved,'ROLLBACK'),executionCommands(restored,'ROLLBACK'));assert.equal(reasonSaved.history.at(-1).execution_id,restored.executions[1].id);assert.equal(reasonSaved.history.at(-1).actor_id,person.accountID);
  assert.ok(reviewPages.some(page=>page.offset==='20'&&page.limit==='20'));evidence.original_order_journey={order_id:id,publication_execution:published.executions[0].id,rollback_execution:restored.executions[1].id,reason_actor:person.accountID,configured_keys:Object.fromEntries(tables.map(table=>[table,['label']])),review_pages:reviewPages};await shot(page,'multitable-original-order-reason.png');check('同一原单贯通管理员管控键、真实分页多表草稿、独立审批发布、原单恢复和实际执行人原因补填');


  for(const terminal of ['cancel','reject','approved-cancel','complete']){
   let current=await api(editor,'POST','/api/v1/release-orders',{title:`lifecycle ${terminal}`,items:tables.map(table=>({table_name:table,operation:'ADD',content:{id:'501',label:terminal}}))},201);
   const target=`/api/v1/release-orders/${current.id}`;
   if(terminal!=='cancel')current=await api(editor,'POST',target+'/submit',{expected_version:current.version});
   if(['approved-cancel','complete'].includes(terminal))current=await api(reviewer,'POST',target+'/approve',{expected_version:current.version,reason:'independent review'});
   if(terminal==='complete')current=await api(editor,'POST',target+'/execute',{expected_version:current.version});
   const action=terminal==='approved-cancel'?'cancel':terminal;
   current=await api(action==='reject'?reviewer:admin,'POST',target+'/'+action,{expected_version:current.version,...(action==='complete'?{}:{reason:'end all table references'})});
   assert.equal(current.applicant_id,person.accountID);assert.equal(current.state,{cancel:'CANCELLED',reject:'REJECTED','approved-cancel':'CANCELLED',complete:'COMPLETED'}[terminal]);
   if(terminal==='complete')assert.equal(current.allowed_actions.includes('quick-rollback'),false);
  }
  check('两表目标在草稿取消、拒绝、已批准管理员取消后可重新占用，整单完结关闭回滚');

  await page.setViewportSize({width:1440,height:1000});
  const copyRecord='13',copyVersions=['2','0'];
  let copySource=await api(editor,'POST','/api/v1/release-orders',{title:'复制核对两表当前基线',items:tables.map((table,index)=>({table_name:table,operation:'MODIFY',id:copyRecord,expected_record_version:copyVersions[index],content:{label:`copy-intent-${index?'b':'a'}`}}))},201);
  const copySourcePath=`/api/v1/release-orders/${copySource.id}`;
  copySource=await api(editor,'POST',copySourcePath+'/submit',{expected_version:copySource.version});
  copySource=await api(reviewer,'POST',copySourcePath+'/reject',{expected_version:copySource.version,reason:'核对最新两表配置后复制'});
  let baseline=await api(editor,'POST','/api/v1/release-orders',{title:'更新复制基线',items:tables.map((table,index)=>({table_name:table,operation:'MODIFY',id:copyRecord,expected_record_version:copyVersions[index],content:{label:`fresh-copy-${index?'b':'a'}`}}))},201);
  const baselinePath=`/api/v1/release-orders/${baseline.id}`;
  baseline=await api(editor,'POST',baselinePath+'/submit',{expected_version:baseline.version});
  baseline=await api(reviewer,'POST',baselinePath+'/approve',{expected_version:baseline.version,reason:'独立确认新基线'});
  baseline=await api(editor,'POST',baselinePath+'/execute',{expected_version:baseline.version});
  await api(editor,'POST',baselinePath+'/complete',{expected_version:baseline.version});
  let blocker=await api(admin,'POST','/api/v1/release-orders',{title:'占用第二张表',items:[{table_name:tables[1],operation:'MODIFY',id:copyRecord,expected_record_version:'1',content:{label:'later-table-blocker'}}]},201);
  const blockerPath=`/api/v1/release-orders/${blocker.id}`;
  await page.goto(`${base}/configuration/release-orders/${copySource.id}`);await button(page,'复制新草稿').click();const copyDrawer=page.getByRole('dialog',{name:'复制新草稿',exact:true});await copyDrawer.getByRole('button',{name:'读取最新配置',exact:true}).click();
  await copyDrawer.getByText(`明细 2 · ${tables[1]} · MODIFY · 记录 ${copyRecord}`,{exact:true}).click();
  await copyDrawer.getByText('fresh-copy-b',{exact:true}).waitFor();await copyDrawer.getByText('copy-intent-b',{exact:true}).waitFor();
  await copyDrawer.getByRole('button',{name:'确认最新基线并复制',exact:true}).click();const conflictTable=copyDrawer.getByText(`冲突表：${tables[1]}`,{exact:true});await conflictTable.waitFor();
  await copyDrawer.getByRole('link',{name:`占用发布单：${blocker.id}（新窗口查看）`,exact:true}).waitFor();await copyDrawer.getByText('申请人：Browser acceptance',{exact:true}).waitFor();assert.match(adminPerson.accountID,/^[a-f0-9-]{36}$/);
  assert.equal(await copyDrawer.getByText('fresh-copy-b',{exact:true}).isVisible(),true);assert.equal(await copyDrawer.getByText('copy-intent-b',{exact:true}).isVisible(),true);await conflictTable.scrollIntoViewIfNeeded();await shot(page,'multitable-copy-conflict.png');
  await copyDrawer.getByRole('button',{name:'关闭',exact:true}).last().click();blocker=await api(admin,'POST',blockerPath+'/cancel',{expected_version:blocker.version,reason:'解除第二表占用'});
  await button(page,'复制新草稿').click();await page.getByRole('dialog',{name:'复制新草稿',exact:true}).getByRole('button',{name:'读取最新配置',exact:true}).click();
  await page.getByRole('dialog',{name:'复制新草稿',exact:true}).getByRole('button',{name:'确认最新基线并复制',exact:true}).click();
  await page.waitForURL(url=>url.pathname.startsWith('/configuration/release-orders/')&&!url.pathname.endsWith(copySource.id));
  const copied=await read(editor,'/api/v1/release-orders/'+new URL(page.url()).pathname.split('/').pop());
  await page.goto(`${base}/configuration/release-orders/${copySource.id}`);const copyBack=page.getByRole('link',{name:copied.id,exact:true});await copyBack.waitFor();await copyBack.click();
  await page.getByText('复制自',{exact:true}).waitFor({state:'attached'});await page.getByText('基本信息',{exact:true}).click();const copiedFrom=page.getByLabel('基本信息',{exact:true});await copiedFrom.getByText('复制自',{exact:true}).waitFor();await copiedFrom.getByRole('link',{name:copySource.id,exact:true}).waitFor();
  assert.deepEqual(copied.items.map(item=>item.table_name),tables);assert.deepEqual(copied.items.map(item=>item.detail_id),copySource.items.map(item=>item.detail_id));
  check('复制核对两表当前基线，晚表冲突显示表/占用单/申请人并保留确认内容，成功后双向关联');

  const reprepareRecord='12';
  let reprepareSource=await api(editor,'POST','/api/v1/release-orders',{title:'浏览器多表重新准备',items:tables.map((table,index)=>({table_name:table,operation:'MODIFY',id:reprepareRecord,expected_record_version:'2',content:{label:`reprepare-${index?'b':'a'}`}}))},201);
  const reprepareSourcePath=`/api/v1/release-orders/${reprepareSource.id}`;
  reprepareSource=await api(editor,'POST',reprepareSourcePath+'/submit',{expected_version:reprepareSource.version});
  reprepareSource=await api(reviewer,'POST',reprepareSourcePath+'/approve',{expected_version:reprepareSource.version,reason:'原审批只属于原单'});
  await page.goto(`${base}/configuration/release-orders/${reprepareSource.id}`);await button(page,'重新准备').click();const reprepareDrawer=page.getByRole('dialog',{name:'重新准备',exact:true});await reprepareDrawer.getByRole('button',{name:'读取最新配置',exact:true}).click();
  await reprepareDrawer.getByText(`明细 2 · ${tables[1]} · MODIFY · 记录 ${reprepareRecord}`,{exact:true}).waitFor();await reprepareDrawer.getByRole('button',{name:'继续重新准备',exact:true}).click();await page.getByRole('alertdialog',{name:'取消旧单并创建新草稿？',exact:true}).getByRole('button',{name:'取消旧单并创建新草稿',exact:true}).click();
  await page.getByText('重新准备自',{exact:true}).waitFor({state:'attached'});await page.getByText('基本信息',{exact:true}).click();const repreparedFrom=page.getByLabel('基本信息',{exact:true});await repreparedFrom.getByText('重新准备自',{exact:true}).waitFor();const repreparedID=new URL(page.url()).pathname.split('/').at(-1);assert.notEqual(repreparedID,reprepareSource.id);
  await repreparedFrom.getByRole('link',{name:reprepareSource.id,exact:true}).click();await page.getByLabel('发布单状态',{exact:true}).filter({hasText:/^已取消$/}).waitFor();const reprepareForward=page.getByRole('link',{name:repreparedID,exact:true});await reprepareForward.waitFor();await reprepareForward.click();
  await button(page,'提交审批').click();await button(page,'确认提交审批').click();await page.getByLabel('发布单状态',{exact:true}).filter({hasText:/^待审批$/}).waitFor();
  await review.goto(`${base}/configuration/release-orders/${repreparedID}`);await button(review,'批准发布单').click();await review.getByLabel('审批意见',{exact:true}).fill('重新独立核对全部两表明细');await button(review,'确认批准').click();await review.getByLabel('发布单状态',{exact:true}).filter({hasText:/^已批准$/}).waitFor();
  const reprepared=await read(editor,`/api/v1/release-orders/${repreparedID}`);assert.equal(reprepared.applicant_id,person.accountID);assert.deepEqual(reprepared.items.map(item=>item.detail_id),reprepareSource.items.map(item=>item.detail_id));
  await shot(review,'multitable-reprepare-approved.png');check('重新准备从确认页原子生成实际操作者草稿，新旧双向关联且新草稿重新独立审批');

  // A large text value uses the administrator's multiline control. WebKit's
  // native single-line input caps inserted text even without a maxlength.
  await settings.goto(`${base}/platform/table-policies/${largeTable}?mode=fields`);
  await settings.getByRole('combobox',{name:'真实字段',exact:true}).selectOption('payload');await button(settings,'配置此字段').click();
  await settings.getByLabel('录入控件',{exact:true}).selectOption('textarea');
  const fieldSave=settings.waitForResponse(response=>new URL(response.url()).pathname===`/api/v1/table-field-policies/${largeTable}`&&response.request().method()==='PUT'&&response.status()===200);
  await button(settings,'保存全部字段配置').click();await fieldSave;await button(settings,'关闭').last().click();
  const largeFieldPolicy=await api(admin,'GET',`/api/v1/table-field-policies/${largeTable}`);
  assert.equal(largeFieldPolicy.fields.find(field=>field.field_name==='payload').effective.ui_type,'textarea');
  const largePage=await pageFor(editor);await largePage.goto(`${base}/configuration/managed-data?table_name=${largeTable}`);await button(largePage,'新增记录').click();
  for(const field of ['id','payload'])await largePage.getByLabel(`包含 ${field}`,{exact:true}).check();await largePage.getByLabel('id 值',{exact:true}).fill('1');
  const payload='宽'.repeat(3<<20);await largePage.getByLabel('payload 值',{exact:true}).fill(payload);
  const inputPayload=await largePage.getByLabel('payload 值',{exact:true}).inputValue();
  const inputControl=await largePage.getByLabel('payload 值',{exact:true}).evaluate(element=>({tag:element.tagName,type:element.type,maxLength:element.maxLength}));assert.equal(inputControl.tag,'TEXTAREA');
  const largeLayout=await largePage.getByLabel('payload 值',{exact:true}).evaluate(element=>({width:element.getBoundingClientRect().width,height:element.getBoundingClientRect().height,scrollHeight:element.scrollHeight,documentHeight:document.documentElement.scrollHeight,viewportHeight:innerHeight}));
  if(output)writeFileSync(join(output,'large-input-layout.json'),JSON.stringify({engine,control:inputControl,characters:inputPayload.length,layout:largeLayout},null,2));
  assert.ok(largeLayout.height<=largeLayout.viewportHeight*0.42+1,'large text editor must remain bounded within the viewport');assert.ok(largeLayout.scrollHeight>largeLayout.height,'complete large text must remain available by scrolling');
  await shot(largePage,'large-text-editor.png');
  const largeReviewStarted=Date.now();
  await button(largePage,'查看 Change Set').click();await largePage.getByLabel('发布单标题',{exact:true}).fill('大值原请求恢复');
  if(output)writeFileSync(join(output,'large-review-duration.json'),JSON.stringify({engine,milliseconds:Date.now()-largeReviewStarted},null,2));
  const writes=[];editor.on('request',request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1/release-orders')writes.push({body:request.postData(),key:request.headers()['idempotency-key']})});
  let saved;await largePage.route('**/api/v1/release-orders',async route=>{if(route.request().method()!=='POST')return route.continue();const response=await route.fetch();assert.equal(response.status(),201);saved=await response.json();await route.abort('failed')});
  await button(largePage,'确认并保存草稿').click();await largePage.getByText('Admin 连接或响应传输中断。',{exact:true}).waitFor();assert.equal(writes.length,1);
  const valueEvidence=value=>({characters:value.length,bytes:Buffer.byteLength(value),sha256:digest(value)});
  const persisted=await read(editor,`/api/v1/release-orders/${saved.id}`);
  const largeDiagnostic={engine,control:inputControl,effective_ui_type:'textarea',expected:valueEvidence(payload),input:valueEvidence(inputPayload),captured_body:valueEvidence(writes[0].body),captured_payload:valueEvidence(JSON.parse(writes[0].body).items[0].content.payload),saved_payload:valueEvidence(persisted.items[0].content.payload)};
  if(output)writeFileSync(join(output,'large-request-diagnostic.json'),JSON.stringify(largeDiagnostic,null,2));
  assert.ok(inputPayload===payload,'large text control must retain the complete input');assert.ok(persisted.items[0].content.payload===payload,'saved payload must match the complete input');assert.ok(Buffer.byteLength(writes[0].body)>(9<<20));await largePage.unroute('**/api/v1/release-orders');
  await reloadIdentity(largePage,person,['EDITOR','PUBLISHER']);await reopenDraftSave(largePage);await button(largePage,'确认并保存草稿').waitFor();
  await setFixtureRoles(admin,base,person.accountID,['VIEWER']);await reloadIdentity(largePage,person,['VIEWER']);assert.equal(await button(largePage,'新建草稿').count(),0);assert.equal(writes.length,1);
  await setFixtureRoles(admin,base,person.accountID,['EDITOR','PUBLISHER']);
  const switched=await registerFixtureAccount(editor,base,{roles:['VIEWER']});await reloadIdentity(largePage,switched,['VIEWER']);assert.equal(await button(largePage,'新建草稿').count(),0);assert.equal(writes.length,1);
  const pre=await (await editor.request.get(base+'/api/v1/auth/csrf')).json();const login=await editor.request.post(base+'/api/v1/auth/login',{headers:{Origin:base,'X-CSRF-Token':pre.csrf_token},data:{username:person.credentials.username,password:person.credentials.password}});assert.equal(login.status(),200);
  await reloadIdentity(largePage,person,['EDITOR','PUBLISHER']);await repeatDraftSave(largePage);await largePage.getByRole('heading',{name:'大值原请求恢复',exact:true}).waitFor();assert.equal(writes.length,2);assert.equal(writes[1].key,writes[0].key);assert.equal(digest(writes[1].body),digest(writes[0].body));
  const reloaded=await read(editor,`/api/v1/release-orders/${saved.id}`);assert.equal(reloaded.version,'1');assert.equal(reloaded.items[0].content.payload,payload);
  evidence.large_request={bytes:Buffer.byteLength(writes[0].body),sha256:digest(writes[0].body),same_key:true,same_order:true,account_isolation:switched.accountID!==person.accountID};
  check('真实应用9MiB正文保存丢响应，刷新/撤权/账号切换后原key与全文恢复同一草稿');
  await button(largePage,'编辑草稿').click();await largePage.getByLabel('payload 申请值',{exact:true}).fill('retained storage failure input');
  await largePage.evaluate(()=>{IDBObjectStore.prototype.put=function(){throw new DOMException('injected quota','QuotaExceededError')}});
  let editWrites=0;largePage.on('request',request=>{if(request.method()==='PUT')editWrites++});await button(largePage,'保存草稿修改').click();await largePage.getByText(/浏览器无法保存完整请求/).waitFor();assert.equal(editWrites,0);assert.equal(await largePage.getByLabel('payload 申请值',{exact:true}).inputValue(),'retained storage failure input');await shot(largePage,'journal-storage-error.png');
  await largePage.addInitScript(()=>{window.__rccIDBOpenCalls=0;IDBFactory.prototype.open=function injectedUnavailableOpen(){window.__rccIDBOpenCalls++;throw new DOMException('injected unavailable','InvalidStateError')}});await largePage.goto(base+'/platform/query-policies');await largePage.getByRole('heading',{name:'查询规则定义',exact:true}).waitFor();
  await largePage.goto(base+'/configuration/release-orders');try{await largePage.waitForFunction(()=>window.__rccIDBOpenCalls>0);await largePage.getByText(/浏览器无法读取原发布请求/).waitFor()}catch(error){if(output)writeFileSync(join(output,'storage-unavailable-diagnostic.json'),JSON.stringify(await largePage.evaluate(()=>({url:location.href,openCalls:window.__rccIDBOpenCalls,openFunction:String(indexedDB.open),body:document.body.innerText})),null,2));throw error};await largePage.getByRole('heading',{name:'发布单',exact:true}).waitFor();assert.equal(editWrites,0);
  evidence.storage_unavailable=await largePage.evaluate(()=>({calls:window.__rccIDBOpenCalls,injected:indexedDB.open===IDBFactory.prototype.open&&indexedDB.open.name==='injectedUnavailableOpen'}));assert.ok(evidence.storage_unavailable.calls>0);assert.equal(evidence.storage_unavailable.injected,true);
  check('持久化失败前零业务发送且保留输入，日志读取失败不阻断账号或无关只读页面');
  assert.deepEqual(errors,[]);if(output)writeFileSync(join(output,'result.json'),JSON.stringify(evidence,null,2));console.log(JSON.stringify(evidence));
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
