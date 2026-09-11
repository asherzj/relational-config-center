import {approvalNotificationSchema} from "./approval-notifications";
import {z} from "zod";
import {approvalRoleIdentitySchema} from "./table-approval";
import {ApiError,request} from "./client";

const version=z.string().regex(/^(0|[1-9][0-9]*)$/);
const content=z.record(z.string(),z.string().nullable());
export const releaseTypeSchema=z.enum(["STANDARD","EMERGENCY"]);
export type ReleaseType=z.infer<typeof releaseTypeSchema>;
export const draftItemSchema=z.object({detail_id:z.string().optional(),table_name:z.string().min(1),operation:z.enum(["ADD","MODIFY","DELETE"]),id:z.string().nullable().optional(),expected_record_version:z.string().optional(),content});
export type DraftItem=z.infer<typeof draftItemSchema>;
export type DraftContentInput={items:DraftItem[]};
export type DraftInput=DraftContentInput&{title:string;expected_version?:string;release_type?:ReleaseType};
export const defaultReleaseTitle=(table:string)=>Array.from(`${table} 配置变更`).slice(0,100).join("");
export const releaseTitleError=(title:string)=>{
 if(!title.trim())return "发布单标题必填。";
 if(Array.from(title).length>100)return "发布单标题不能超过 100 个字符。";
 return undefined;
};
export const validReleaseTitle=(title:string)=>!releaseTitleError(title);
export const releaseFieldSchema=z.object({name:z.string(),type:z.string(),nullable:z.boolean(),editable:z.boolean(),before_state:z.enum(["value","sql_null","absent"]),before:z.string().nullable(),proposed_state:z.enum(["value","sql_null","omitted","absent","automatic","generated"]),proposed:z.string().nullable()});
const canonicalRowSchema=z.object({format:z.literal("rcc-admin-mysql-row-v1"),schema_digest:z.string().regex(/^[a-f0-9]{64}$/),deleted:z.boolean(),fields:z.array(z.object({name:z.string(),type:z.string(),encoding:z.enum(["text","json","base64","sql_null"]),value:z.string().nullable()})),checksum:z.string().regex(/^[a-f0-9]{64}$/)});
const notificationSchema=z.object({id:z.string(),table_name:z.string(),table_version:version,status:z.literal("NOT_CONNECTED")});
export const publicationCommandSchema=z.object({detail_id:z.string().min(1),execution_id:z.string(),execution_kind:z.enum(["PUBLICATION","ROLLBACK"]),order_id:z.string(),sequence:version,table_name:z.string(),table_version:version,operation:z.enum(["ADD","MODIFY","DELETE"]),id:z.string(),record_version:version,before:canonicalRowSchema,final:canonicalRowSchema});
export const executionSchema=z.object({id:z.string(),kind:z.enum(["PUBLICATION","ROLLBACK"]),actor_id:z.string(),executed_at:z.string(),table_versions:z.record(z.string(),version),notifications:z.record(z.string(),notificationSchema),operation_counts:z.record(z.string(),z.number().int().nonnegative()),item_count:z.number().int().min(1).max(1000),outcome:z.literal("SUCCEEDED")});
export type ReleaseExecution=z.infer<typeof executionSchema>;
export type PublicationCommand=z.infer<typeof publicationCommandSchema>;
const approvalSourceSchema=z.object({source:z.enum(["ROLE","ADMIN"]),roles:z.array(approvalRoleIdentitySchema)});
const tableApprovalSchema=z.object({table_name:z.string(),roles:z.array(approvalRoleIdentitySchema),state:z.enum(["PENDING","APPROVED","REJECTED"]),decision:approvalSourceSchema.extend({actor_id:z.string(),at:z.string(),reason:z.string()}).optional()});
const approvalContextSchema=z.object({revision:z.string(),tables:z.array(z.object({table_name:z.string(),mode:z.enum(["ROLE","ADMIN","UNAVAILABLE","COMPLETED"]),reason:z.string(),can_approve:z.boolean()})),approvable_tables:z.array(z.string())});
export type ApprovalContext=z.infer<typeof approvalContextSchema>;
const releaseFlowNodeSchema=z.object({code:z.string(),type:z.enum(["APPROVAL","PUBLICATION","COMPLETION"]),name:z.string(),required_role:z.enum(["TABLE_APPROVER","PUBLISHER"]),state:z.enum(["PENDING","ACTIVE","COMPLETED","REJECTED","STOPPED"]),actor_id:z.string().optional(),at:z.string().optional()});
const tableReleaseFlowSchema=z.object({instance_id:z.string(),table_name:z.string(),release_type:releaseTypeSchema,template_code:z.string(),template_name:z.string(),template_version:version,association_version:version,instantiated_at:z.string(),node_list:z.array(releaseFlowNodeSchema)});
export type TableReleaseFlow=z.infer<typeof tableReleaseFlowSchema>;
export const releaseSummarySchema=z.object({notification:approvalNotificationSchema,id:z.string(),title:z.string(),table_names:z.array(z.string()),release_type:releaseTypeSchema,emergency_reason:z.string(),table_flows:z.array(tableReleaseFlowSchema),missing_flow_tables:z.array(z.string()),applicant_id:z.string(),state:z.enum(["DRAFT","PENDING_APPROVAL","PENDING_PUBLICATION","APPROVED","SUCCEEDED","COMPLETED","REJECTED","CANCELLED","ROLLED_BACK"]),version,created_at:z.string(),updated_at:z.string(),allowed_actions:z.array(z.string()),approvals:z.array(tableApprovalSchema),approval_context:approvalContextSchema,item_count:z.number().int().min(0).max(1000),operation_counts:z.record(z.string(),z.number().int().nonnegative())});
export const releaseHeaderSchema=releaseSummarySchema.extend({copied_from_id:z.string().optional(),frozen_digest:z.string().optional(),executions:z.array(executionSchema),history:z.array(z.object({action:z.string(),actor_id:z.string(),at:z.string(),version,reason:z.string(),related_order_id:z.string().optional(),execution_id:z.string().optional(),table_names:z.array(z.string()).optional(),approval_sources:z.array(approvalSourceSchema.extend({table_name:z.string()})).optional()}))});
export const releaseItemSchema=draftItemSchema.extend({detail_id:z.string(),id:z.string().nullable(),expected_record_version:z.string(),before:content.nullable(),fields:z.array(releaseFieldSchema),publication:publicationCommandSchema.optional(),rollback:publicationCommandSchema.optional()});
export const releaseOrderSchema=releaseHeaderSchema.omit({notification:true}).extend({items:z.array(releaseItemSchema)});
export type ReleaseHeader=z.infer<typeof releaseHeaderSchema>;
export type ReleaseOrder=z.infer<typeof releaseOrderSchema>;
export const releaseDetailPageSchema=z.object({order_id:z.string(),version,item_count:z.number().int().min(0).max(1000),offset:z.number().int().nonnegative(),next_offset:z.number().int().nonnegative().nullable(),items:z.array(releaseItemSchema).max(100)});
export const quickRollbackPreviewSchema=z.object({order_id:z.string(),expected_version:version,preview_digest:z.string().regex(/^[a-f0-9]{64}$/),items:z.array(releaseItemSchema)});
export type QuickRollbackPreview=z.infer<typeof quickRollbackPreviewSchema>;
export type ReleaseField=z.infer<typeof releaseFieldSchema>;
export const releaseOrders={
 quickRollbackPreview:(id:string,expectedVersion:string)=>request(`/api/v1/release-orders/${encodeURIComponent(id)}/quick-rollback/preview`,{method:"POST",body:JSON.stringify({expected_version:expectedVersion}),schema:quickRollbackPreviewSchema}),
 list:(filters:Record<string,string>)=>request(`/api/v1/release-orders?${new URLSearchParams(filters)}`,{schema:z.object({orders:z.array(releaseSummarySchema),next_cursor:z.string()})}),
 preview:(input:DraftContentInput)=>request("/api/v1/release-orders/preview",{method:"POST",body:JSON.stringify(input),schema:z.object({items:releaseOrderSchema.shape.items})}),
 get:(id:string)=>request(`/api/v1/release-orders/${encodeURIComponent(id)}`,{schema:releaseHeaderSchema}),
 details:async(header:ReleaseHeader,offset:number,limit=20)=>{
  const page=await request(`/api/v1/release-orders/${encodeURIComponent(header.id)}/details?${new URLSearchParams({expected_version:header.version,offset:String(offset),limit:String(limit)})}`,{schema:releaseDetailPageSchema});
  const next=offset+page.items.length;
  if(page.order_id!==header.id||page.version!==header.version||page.item_count!==header.item_count||page.offset!==offset||page.items.length!==Math.min(limit,header.item_count-offset)||page.next_offset!==(next<header.item_count?next:null))throw new ApiError("release_version_conflict","发布单已变化，请重新读取当前发布单。",409);
  for(const item of page.items){
   for(const kind of ["PUBLICATION","ROLLBACK"] as const){
    const execution=header.executions.find(entry=>entry.kind===kind),command=kind==="PUBLICATION"?item.publication:item.rollback;
    if(Boolean(execution)!==Boolean(command)||command&&execution&&(command.detail_id!==item.detail_id||command.order_id!==header.id||command.table_name!==item.table_name||command.execution_id!==execution.id||command.execution_kind!==kind||command.table_version!==execution.table_versions[item.table_name]))throw new ApiError("contract_mismatch","明细执行结果与发布单不一致。",200);
   }
  }
  return page;
 },
 people:(id:string)=>request(`/api/v1/release-orders/${encodeURIComponent(id)}/people`,{schema:z.object({people:z.record(z.string(),z.string())})}),
 write:(path:string,method:string,body:string,key:string)=>request(path,{method,body,headers:{"Idempotency-Key":key},schema:releaseOrderSchema}),
};
export function draftFromOrder(order:ReleaseOrder):DraftInput{
 return {title:order.title,expected_version:order.version,release_type:order.release_type,items:order.items.map(item=>({detail_id:item.detail_id,table_name:item.table_name,operation:item.operation,...(item.operation!=="ADD"?{id:item.id}:{}),expected_record_version:item.expected_record_version,content:{...item.content}}))};
}

// Transport envelopes are serialized here once and retained unchanged for retries.
export type ReleaseRequestEnvelope={path:string;method:"POST"|"PUT";body:string};
const draftChangesSchema=z.object({upserts:z.array(draftItemSchema).optional(),delete_detail_ids:z.array(z.string()).optional(),detail_order:z.array(z.string()).optional()});
export type DraftChanges=z.infer<typeof draftChangesSchema>;
const incrementalDraftSchema=z.object({title:z.string(),expected_version:z.string(),release_type:releaseTypeSchema.optional(),changes:draftChangesSchema});
export type IncrementalDraftInput=z.infer<typeof incrementalDraftSchema>;
const draftInputSchema=z.object({title:z.string(),items:z.array(draftItemSchema),expected_version:z.string().optional(),release_type:releaseTypeSchema.optional()});
const approvalInputSchema=z.object({expected_version:z.string(),reason:z.string(),confirmed_tables:z.array(z.string()),expected_approval_revision:z.string()});
const cancelInputSchema=z.object({expected_version:z.string(),reason:z.string()});
const quickRollbackInputSchema=z.object({expected_version:version,preview_digest:z.string().regex(/^[a-f0-9]{64}$/),reason:z.string()});
const rollbackReasonInputSchema=z.object({reason:z.string()});
const submitInputSchema=z.object({expected_version:z.string(),emergency_reason:z.string().optional()});
const copyInputSchema=z.object({expected_version:z.string(),confirmed:z.literal(true),items:z.array(draftItemSchema)});
export type ReleaseStateAction="submit"|"approve"|"reject"|"cancel"|"execute"|"complete";
export const releaseActionLabels={"quick-rollback":"快速回滚","edit-rollback-reason":"修改回滚原因","edit-details":"保存草稿修改",complete:"完结发布单",execute:"执行发布",submit:"提交审批",approve:"批准发布单",reject:"拒绝发布单",cancel:"取消发布单",copy:"复制新草稿",reprepare:"重新准备"};
export const releaseActionRole=(action:string)=>action==="edit-rollback-reason"?"VIEWER" as const:action==="execute"||action==="complete"||action==="quick-rollback"?"PUBLISHER" as const:action==="approve"||action==="reject"?"VIEWER" as const:"EDITOR" as const;

export const releaseActionRequiresReason=(action:ReleaseStateAction)=>action!=="submit"&&action!=="execute"&&action!=="complete";

export const releaseRequests={
 quickRollback:(id:string,expectedVersion:string,previewDigest:string,reason:string):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/quick-rollback`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,preview_digest:previewDigest,reason})}),
 rollbackReason:(id:string,reason:string):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/rollback-reason`,method:"POST",body:JSON.stringify({reason})}),
 action:(action:ReleaseStateAction,id:string,expectedVersion:string,reason="",approval?:ApprovalContext):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/${action}`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,...(action==="submit"&&reason?{emergency_reason:reason}:releaseActionRequiresReason(action)?{reason}:{}),...((action==="approve"||action==="reject")?{confirmed_tables:approval?.approvable_tables??[],expected_approval_revision:approval?.revision??""}:{})})}),
 copy:(id:string,expectedVersion:string,items:DraftItem[]):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/copy`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,confirmed:true,items})}),
 reprepare:(id:string,expectedVersion:string,items:DraftItem[]):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/reprepare`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,confirmed:true,items})}),
 create:(input:DraftInput):ReleaseRequestEnvelope=>({path:"/api/v1/release-orders",method:"POST",body:JSON.stringify(input)}),
 edit:(id:string,input:DraftInput|IncrementalDraftInput):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}`,method:"PUT",body:JSON.stringify(input)}),
 cancel:(id:string,expectedVersion:string,reason:string):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/cancel`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,reason})}),
};
export function decodeReleaseRequest(value:ReleaseRequestEnvelope){
 const id=value.path.split("/")[4];
 const body:unknown=JSON.parse(value.body);
 const action=value.path.split("/")[5];
 if(id&&action==="quick-rollback")return {action:"quick-rollback" as const,id,input:quickRollbackInputSchema.parse(body)};
 if(id&&action==="rollback-reason")return {action:"edit-rollback-reason" as const,id,input:rollbackReasonInputSchema.parse(body)};
 if(id&&action==="complete")return {action:"complete" as const,id,input:submitInputSchema.parse(body)};
 if(id&&action==="execute")return {action:"execute" as const,id,input:submitInputSchema.parse(body)};
 if(id&&action==="submit")return {action:"submit" as const,id,input:submitInputSchema.parse(body)};
 if(id&&action==="copy")return {action:"copy" as const,id,input:copyInputSchema.parse(body)};
 if(id&&action==="reprepare")return {action:"reprepare" as const,id,input:copyInputSchema.parse(body)};
 if(id&&action==="cancel")return {action:"cancel" as const,id,input:cancelInputSchema.parse(body)};
 if(id&&action==="approve")return {action:"approve" as const,id,input:approvalInputSchema.parse(body)};
 if(id&&action==="reject")return {action:"reject" as const,id,input:approvalInputSchema.parse(body)};
 if(id&&typeof body==="object"&&body!==null&&"changes" in body)return {action:"edit-details" as const,id,input:incrementalDraftSchema.parse(body)};
 const input=draftInputSchema.parse(body);
 return id?{action:"edit" as const,id,input}:{action:"create" as const,input};
}

// Compare against the acknowledged baseline. Pagination changes only the view;
// the envelope carries changed details, explicit deletions and optional ordering.
export function incrementalDraft(baseline:ReleaseOrder,edited:ReleaseOrder):IncrementalDraftInput {
 const previous=draftFromOrder(baseline).items,next=draftFromOrder(edited).items;
 const old=new Map(previous.map(item=>[item.detail_id,item]));
 const ids=new Set(next.map(item=>item.detail_id));
 const changes:DraftChanges={upserts:next.filter(item=>JSON.stringify(item)!==JSON.stringify(old.get(item.detail_id))),delete_detail_ids:previous.filter(item=>!ids.has(item.detail_id)).map(item=>item.detail_id!)};
 const expectedOrder=previous.filter(item=>ids.has(item.detail_id)).map(item=>item.detail_id);
 if(JSON.stringify(expectedOrder)!==JSON.stringify(next.map(item=>item.detail_id)))changes.detail_order=next.map(item=>item.detail_id!);
 return {title:edited.title,expected_version:baseline.version,...(edited.release_type!==baseline.release_type?{release_type:edited.release_type}:{}),changes};
}
export function rebaseDraftInput(baseline:ReleaseOrder,edited:ReleaseOrder,latest:ReleaseOrder):ReleaseOrder {
 const changes=incrementalDraft(baseline,edited).changes,local=new Map(edited.items.map(item=>[item.detail_id,item]));
 const changed=new Set(changes.upserts?.map(item=>item.detail_id));
 let items=latest.items.filter(item=>!changes.delete_detail_ids?.includes(item.detail_id!)).map(item=>changed.has(item.detail_id)?local.get(item.detail_id)!:item);
 // Keep an edited detail that disappeared visible; its obsolete identity will
 // be rejected on save instead of silently dropping the user's input.
 for(const item of edited.items)if(changed.has(item.detail_id)&&!items.some(current=>current.detail_id===item.detail_id))items.push(item);
 if(changes.detail_order){const rank=new Map(changes.detail_order.map((id,index)=>[id,index]));items=[...items].sort((a,b)=>(rank.get(a.detail_id!)??Infinity)-(rank.get(b.detail_id!)??Infinity))}
 return {...latest,title:edited.title===baseline.title?latest.title:edited.title,release_type:edited.release_type===baseline.release_type?latest.release_type:edited.release_type,items};
}

// Only an explicit editing/copy action assembles all application details. Every
// page is bound to the same whole-order version; any mismatch aborts the load.
export async function loadReleaseForEdit(header:ReleaseHeader):Promise<ReleaseOrder> {
 const items:ReleaseOrder["items"]=[];
 for(let offset=0;offset<header.item_count;){
  const page=await releaseOrders.details(header,offset,100);
  items.push(...page.items);offset+=page.items.length;
 }
 return {...header,items};
}
export function releaseTables(order:{table_names:string[]}) { return order.table_names; }
export function releaseDetailTables(order:Pick<ReleaseOrder,"items">) { return order.items.map(item=>item.table_name); }

export function canReviewRelease(order:Pick<ReleaseHeader,"allowed_actions"|"approval_context">,action:"approve"|"reject") {
 const context=order.approval_context;
 return order.allowed_actions.includes(action)&&Boolean(context.revision)&&context.approvable_tables.length>0&&context.approvable_tables.every(table=>context.tables.some(entry=>entry.table_name===table&&entry.can_approve));
}
