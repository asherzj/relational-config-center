// Test data builders use the current header/detail contract. They never turn a
// server response into a legacy aggregate in production code.
export function releaseFixture(value:unknown):unknown {
 if(!value||typeof value!=="object")return value;
 if("orders" in value&&Array.isArray(value.orders))return {...value,orders:value.orders.map(releaseFixture)};
 if(!("id" in value)||!("items" in value)||!Array.isArray(value.items))return value;
 const items=value.items.map((item,index)=>({...item,detail_id:item.detail_id??String(index+1).padStart(32,"0")}));
 const operation_counts:Record<string,number>={};for(const item of items)operation_counts[item.operation]=(operation_counts[item.operation]??0)+1;
 const executions="executions" in value&&Array.isArray(value.executions)?value.executions.map(execution=>({...execution,operation_counts:execution.operation_counts??operation_counts,notifications:execution.notifications??Object.fromEntries(Object.entries(execution.table_versions as Record<string,string>).map(([table,version])=>[table,{id:execution.id,table_name:table,table_version:version,status:"NOT_CONNECTED"}]))})):[];
 const tables=[...new Set(items.map(item=>item.table_name))];
 const context="approval_context" in value?value.approval_context:{revision:"fixture-approval-1",tables:tables.map(table_name=>({table_name,mode:"ROLE",reason:"审批角色成员",can_approve:true})),approvable_tables:tables};
 const approvals="approvals" in value?value.approvals:tables.map(table_name=>({table_name,roles:[],state:"state" in value&&["APPROVED","SUCCEEDED","COMPLETED","ROLLED_BACK"].includes(String(value.state))?"APPROVED":"PENDING"}));
 return {...value,approvals,approval_context:context,items,table_names:[...new Set(items.map(item=>item.table_name))],item_count:items.length,operation_counts,executions};
}

// In-memory HTTP fixture router: a normal GET serves only its header. Detail
// requests are separate, version checked and bounded, just like the real API.
const readRouters=new WeakSet<typeof fetch>();
export function withReleaseReadRoutes(request:typeof fetch):typeof fetch {
 if(readRouters.has(request))return request;
 const snapshots=new Map<string,Record<string,unknown>>();
 const router:typeof fetch=async(input,init)=>{
  const path=String(input),url=new URL(path,"https://fixture.test");
  const detail=url.pathname.match(/^\/api\/v1\/release-orders\/([^/]+)\/details$/);
  if(detail){
   const value=snapshots.get(detail[1]!);
   if(!value)throw new Error(`No header fixture for ${detail[1]}`);
   if(url.searchParams.get("expected_version")!==value.version)return Response.json({error:{code:"release_version_conflict",message:"changed",request_id:"fixture-conflict"}},{status:409});
   const offset=Number(url.searchParams.get("offset")),limit=Number(url.searchParams.get("limit")),items=value.items as unknown[];
   const next=Math.min(offset+limit,items.length);
   return Response.json({order_id:value.id,version:value.version,item_count:items.length,offset,next_offset:next<items.length?next:null,items:items.slice(offset,next)});
  }
  const response=await request(input,init);
  if((!init?.method||init.method==="GET")&&/^\/api\/v1\/release-orders\/[^/]+$/.test(url.pathname)&&response.ok){
   const value=await response.clone().json();
   if(value.id&&Array.isArray(value.items)){
    snapshots.set(value.id,value);
    const {items:_,...header}=value;
    return new Response(JSON.stringify(header),{status:response.status,headers:response.headers});
   }
  }
  return response;
 };
 readRouters.add(router);return router;
}

export function withExecution<T extends {id:string;items:Record<string,unknown>[];updated_at:string;applicant_id:string}>(order:T,kind:"PUBLICATION"|"ROLLBACK",commands:(Record<string,unknown>&{table_name:string;table_version:string;operation:string})[],actor=order.applicant_id,at=order.updated_at){
 const id=`${order.id}:${kind}`,table_versions=Object.fromEntries(commands.map(command=>[command.table_name,command.table_version]));
 const operation_counts:Record<string,number>={};for(const command of commands)operation_counts[command.operation]=(operation_counts[command.operation]??0)+1;
 const execution={id,kind,actor_id:actor,executed_at:at,table_versions,operation_counts,item_count:commands.length,outcome:"SUCCEEDED",notifications:Object.fromEntries(Object.entries(table_versions).map(([table,version])=>[table,{id,table_name:table,table_version:version,status:"NOT_CONNECTED"}]))};
 const executions="executions" in order&&Array.isArray(order.executions)?order.executions:[];
 return {...order,executions:[...executions,execution],items:order.items.map((item,index)=>({...item,[kind==="PUBLICATION"?"publication":"rollback"]:{...commands[index],detail_id:item.detail_id??String(index+1).padStart(32,"0"),order_id:order.id,execution_id:id,execution_kind:kind}}))};
}
