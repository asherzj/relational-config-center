const {mkdirSync,writeFileSync}=require('node:fs');
const assert=require('node:assert/strict');
const {webkit}=require('/Users/asher/Projects/relational-config-center/.worktrees/release-template-multitable-ci/web/node_modules/playwright');
const {clickWithDiagnostics}=require('/Users/asher/Projects/relational-config-center/.worktrees/release-template-multitable-ci/web/e2e/click-diagnostics.cjs');
(async()=>{
 const output='/tmp/rcc-pr109-multitable-diagnostics-final-check';mkdirSync(output,{recursive:true});
 const browser=await webkit.connect('ws://127.0.0.1:32770/');
 try{
  const page=await browser.newPage();page.setDefaultTimeout(900);
  await page.setContent('<style>@keyframes move{from{transform:translateX(0)}to{transform:translateX(300px)}}[role=dialog]{animation:move .2s linear infinite alternate;width:200px;padding:20px;background:#eee}</style><div role="dialog"><button onclick="document.body.dataset.clicked=1">查看 Change Set</button></div>');
  await page.evaluate(()=>{
   const intervals=new Set(),frames=new Set();
   const si=window.setInterval.bind(window),ci=window.clearInterval.bind(window),raf=window.requestAnimationFrame.bind(window),caf=window.cancelAnimationFrame.bind(window);
   window.setInterval=(...args)=>{const id=si(...args);intervals.add(id);return id};window.clearInterval=id=>{intervals.delete(id);ci(id)};
   window.requestAnimationFrame=callback=>{const id=raf(time=>{frames.delete(id);callback(time)});frames.add(id);return id};window.cancelAnimationFrame=id=>{frames.delete(id);caf(id)};
   window.probeResources=()=>({intervals:intervals.size,frames:frames.size});
  });
  const observations=[];let originalError;
  try{await clickWithDiagnostics(page.getByRole('button',{name:'查看 Change Set',exact:true}),observations)}catch(error){originalError=error}
  assert.equal(originalError?.name,'TimeoutError');assert.match(originalError.message,/element is not stable/);
  assert.equal(observations[0].failure.message,originalError.message);
  assert.ok(new Set(observations[0].samples.map(sample=>sample.rect.x)).size>1);assert.ok(observations[0].frames>0);
  const afterFailure=await page.evaluate(()=>probeResources());assert.deepEqual(afterFailure,{intervals:0,frames:0});
  await page.screenshot({path:output+'/synthetic-moving-button.png'});
  await page.evaluate(()=>document.querySelector('[role=dialog]').style.animation='none');
  await clickWithDiagnostics(page.getByRole('button',{name:'查看 Change Set',exact:true}),observations);
  assert.equal(await page.getAttribute('body','data-clicked'),'1');assert.equal(observations[1].failure,null);
  const afterSuccess=await page.evaluate(()=>probeResources());assert.deepEqual(afterSuccess,{intervals:0,frames:0});
  const auxiliaryChecks=[];
  for(const mode of ['setup-rejected','stop-rejected','stop-timeout','setup-late']){
   const sentinel=new Error('original click failure');let stopped=false,disposed=false;
   const handle={evaluate:()=>{stopped=true;return mode==='stop-timeout'?new Promise(()=>{}):mode==='stop-rejected'?Promise.reject(new Error('sampling unavailable')):Promise.resolve({samples:[]})},dispose:()=>{disposed=true;return Promise.resolve()}};
   const locator={evaluateHandle:()=>mode==='setup-rejected'?Promise.reject(new Error('setup unavailable')):mode==='setup-late'?new Promise(resolve=>setTimeout(()=>resolve(handle),2100)):Promise.resolve(handle),click:()=>Promise.reject(sentinel),toString:()=>mode};
   const records=[];const started=Date.now();let caught;
   try{await clickWithDiagnostics(locator,records)}catch(error){caught=error}
   assert.equal(caught,sentinel);assert.ok(Date.now()-started<2500);assert.equal(records.length,1);
   if(mode==='setup-late')await new Promise(resolve=>setTimeout(resolve,250));
   if(mode!=='setup-rejected'){assert.equal(stopped,true);assert.equal(disposed,true)}
   auxiliaryChecks.push({mode,originalErrorPreserved:true,elapsedMs:Date.now()-started,stopped,disposed,record:records[0]});
  }
  const result={ok:true,auxiliaryChecks,scope:'Synthetic moving button tests diagnostic capture/error propagation/cleanup only; NOT reproduction of the intermittent application failure',afterFailure,afterSuccess,observations};
  writeFileSync(output+'/result.json',JSON.stringify(result,null,2));console.log(JSON.stringify({ok:true,auxiliaryChecks:auxiliaryChecks.map(c=>({mode:c.mode,elapsedMs:c.elapsedMs,originalErrorPreserved:c.originalErrorPreserved})),afterFailure,afterSuccess,observations:observations.map(o=>({failure:o.failure?.name,samples:o.samples.length,frames:o.frames}))}));
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
