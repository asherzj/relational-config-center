const fs=require('node:fs');
const assert=require('node:assert/strict');
const events=JSON.parse(fs.readFileSync(process.argv[2],'utf8'));
const copies=events.filter(event=>event.kind==='copied');
assert.ok(copies.length>0,'must observe real copied order IDs');
const results=copies.map(copy=>({copied:copy.id,source:copy.source,phase:copy.phase,cancelled:events.filter(event=>event.phase===copy.phase&&event.atMs>=copy.atMs&&event.kind==='failed'&&(event.path==='/api/v1/approval-notifications'||event.path===`/api/v1/release-orders/${copy.id}`))}));
console.log(JSON.stringify({copies:results.length,results},null,2));
assert.equal(results.reduce((n,result)=>n+result.cancelled.length,0),0,'copy back-navigation must not cancel pending notification/new-order reads');
