const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const root=process.env.RCC_PROBE_REPO||'/Users/asher/Projects/relational-config-center/.worktrees/release-template-browser-ci';
const {webkit} = require(root+'/web/node_modules/playwright');
const identity={account:{id:'ab09850e-ef9a-4317-a000-d67465416b5b',username:'test.user',display_name:'Test',email:'test@example.invalid',email_verified:false,status:'enabled',roles:['ADMIN']},csrf_token:'fixture',expires_at:'2099-09-07T08:00:00Z',idle_expires_at:'2099-09-07T00:30:00Z'};
(async () => {
 let delayed=false, requested; const timeline=[]; const errors=[];
 const record=(type,data={})=>timeline.push({at:Date.now(),type,...data});
 const server = http.createServer((req,res) => {
  if(req.url==='/api/v1/auth/session') {
   record('session.start');const respond=()=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify(identity));record('session.respond')};
   if(delayed){delayed=false;requested?.();setTimeout(respond,1000)}else respond();return;
  }
  if(req.url.startsWith('/api/')){res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({unread_count:0,pending_count:0,tables:[]}));return}
  const file=req.url.startsWith('/assets/') ? path.join(root,'web/dist',req.url) : path.join(root,'web/dist/index.html');
  res.writeHead(200,{'Content-Type':file.endsWith('.js')?'application/javascript':file.endsWith('.css')?'text/css':'text/html'});res.end(fs.readFileSync(file));
 });
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 const base=`http://127.0.0.1:${server.address().port}`;
 const browser=await webkit.launch({headless:true}); const page=await browser.newPage();
 page.on('pageerror',e=>{const item={name:e.name,message:e.message,stack:e.stack};errors.push(item);record('pageerror',item);console.log('PAGEERROR',e.message)});
 page.on('requestfailed',r=>record('failed',{url:r.url(),failure:r.failure()}));
 await page.exposeFunction('reportProbe',data=>record('window',data));
 await page.addInitScript(()=>{
  window.addEventListener('error',e=>window.reportProbe({type:'error',message:e.message}));
  window.addEventListener('unhandledrejection',e=>window.reportProbe({type:'unhandledrejection',message:String(e.reason)}));
  for(const type of ['pagehide','pageshow','visibilitychange','beforeunload'])window.addEventListener(type,()=>window.reportProbe({type,visibility:document.visibilityState}));
 });
 try {
  for(let i=0;i<20;i++){
   await page.goto(base+'/configuration/managed-data');
   record('reload.start',{iteration:i});await page.reload();record('reload.end');
  }
  console.log(JSON.stringify({errors:errors.length}));assert.equal(errors.length,0);
 } finally {await browser.close();server.closeAllConnections();server.close();fs.writeFileSync(process.env.RCC_PROBE_OUTPUT||'/tmp/rcc-webkit-app-repro.json',JSON.stringify({errors,timeline},null,2))}
})().catch(e=>{console.error(e);process.exitCode=1});
