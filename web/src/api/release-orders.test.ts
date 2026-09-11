import {afterEach,expect,it,vi} from "vitest";
import {loadReleaseForEdit,releaseOrders,type ReleaseHeader} from "./release-orders";
const header:ReleaseHeader={id:"original",title:"多表草稿",table_names:["items"],release_type:"STANDARD",table_flows:[],missing_flow_tables:[],applicant_id:"author",state:"DRAFT",version:"3",created_at:"now",updated_at:"now",allowed_actions:["edit"],item_count:101,operation_counts:{ADD:101},executions:[],history:[],approvals:[],approval_context:{revision:"test",tables:[],approvable_tables:[]}};
const detail=(index:number)=>({detail_id:String(index),table_name:"items",operation:"ADD",id:null,expected_record_version:"",content:{label:`value ${index}`},before:null,fields:[]});
afterEach(()=>vi.unstubAllGlobals());
it("详情保留服务端已经保存的逐表流程身份、模板版本和真实节点事实",async()=>{
 const saved={...header,release_type:"STANDARD",missing_flow_tables:["unconfigured"],table_flows:[{instance_id:"saved-instance-a",table_name:"items",release_type:"STANDARD",template_code:"careful_review",template_name:"资金配置核对",template_version:"7",association_version:"3",instantiated_at:"2026-09-11T01:00:00Z",node_list:[{code:"review_funds",type:"APPROVAL",name:"资金负责人确认",required_role:"TABLE_APPROVER",state:"COMPLETED",actor_id:"reviewer-a",at:"2026-09-11T02:00:00Z"},{code:"publish_funds",type:"PUBLICATION",name:"整单生效",required_role:"PUBLISHER",state:"ACTIVE"},{code:"finish_funds",type:"COMPLETION",name:"确认资金配置",required_role:"PUBLISHER",state:"PENDING"}]}]};
 vi.stubGlobal("fetch",vi.fn(async()=>Response.json(saved)));
 expect(await releaseOrders.get(header.id)).toEqual(saved);
});
it("普通 get 只读取 header，显式编辑按固定整单版本收集每页",async()=>{
 const reads:string[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input)=>{
  const url=new URL(String(input),"https://example.test");reads.push(url.pathname+url.search);
  if(!url.pathname.endsWith("/details"))return Response.json(header);
  const offset=Number(url.searchParams.get("offset"));
  expect(url.searchParams.get("expected_version")).toBe("3");expect(url.searchParams.get("limit")).toBe("100");
  const items=Array.from({length:Math.min(100,101-offset)},(_,index)=>detail(offset+index));
  return Response.json({order_id:header.id,version:"3",item_count:101,offset,next_offset:offset===0?100:null,items});
 }));
 const current=await releaseOrders.get(header.id);expect(reads).toHaveLength(1);expect(current).not.toHaveProperty("items");
 const input=await loadReleaseForEdit(current);expect(input.items).toHaveLength(101);expect(input.items[100]!.content.label).toBe("value 100");expect(reads).toHaveLength(3);
});
it.each(["conflict","mixed-version","mixed-total","wrong-order","wrong-offset","missing-items","invalid-continuation"])("编辑第 2 页 %s 时不返回部分输入",async mode=>{
 let reads=0;
 vi.stubGlobal("fetch",vi.fn(async()=>{
  reads++;
  if(reads===1)return Response.json({order_id:header.id,version:"3",item_count:101,offset:0,next_offset:100,items:Array.from({length:100},(_,index)=>detail(index))});
  if(mode==="conflict")return Response.json({error:{code:"release_version_conflict",message:"changed",request_id:"second-page"}},{status:409});
  return Response.json({order_id:mode==="wrong-order"?"another":header.id,version:mode==="mixed-version"?"4":"3",item_count:mode==="mixed-total"?102:101,offset:mode==="wrong-offset"?99:100,next_offset:mode==="invalid-continuation"?101:null,items:mode==="missing-items"?[]:[detail(100)]});
 }));
 await expect(loadReleaseForEdit(header)).rejects.toMatchObject({code:"release_version_conflict"});expect(reads).toBe(2);
});
