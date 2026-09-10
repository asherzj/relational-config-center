// #88 browser acceptance assertions deliberately traverse the real HTTP pages.
// UI requests remain observable separately and use one review page at a time.
const assert=require('node:assert/strict');
const {authenticatedRequest}=require('./local-account.cjs');
async function readAllReleaseDetailPages(context,base,header){
 assert.equal(header.items,undefined,'normal detail GET must return only a header');
 const items=[];
 for(let offset=0;offset<header.item_count;){
  const response=await authenticatedRequest(context,base,`/api/v1/release-orders/${header.id}/details?expected_version=${header.version}&offset=${offset}&limit=100`);
  assert.equal(response.status(),200,await response.text());
  const page=await response.json();assert.equal(page.version,header.version);assert.equal(page.order_id,header.id);assert.equal(page.offset,offset);assert.equal(page.item_count,header.item_count);
  assert.equal(page.items.length,Math.min(100,header.item_count-offset));items.push(...page.items);offset+=page.items.length;assert.equal(page.next_offset,offset<header.item_count?offset:null);
 }
 return {...header,items};
}
function executionCommands(order,kind='PUBLICATION'){
 const items=kind==='ROLLBACK'?[...order.items].reverse():order.items;
 return items.map(item=>kind==='ROLLBACK'?item.rollback:item.publication).filter(Boolean);
}
function applicationItems(order){return order.items.map(({publication,rollback,...intent})=>intent)}
module.exports={readAllReleaseDetailPages,executionCommands,applicationItems};
