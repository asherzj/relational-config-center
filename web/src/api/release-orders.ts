import {z} from "zod";
import {request} from "./client";

const version=z.string().regex(/^(0|[1-9][0-9]*)$/);
const content=z.record(z.string(),z.string().nullable());
export const draftItemSchema=z.object({operation:z.enum(["ADD","MODIFY","DELETE"]),id:z.string().nullable().optional(),expected_record_version:z.string().optional(),content});
export type DraftItem=z.infer<typeof draftItemSchema>;
export type DraftInput={table_name:string;items:DraftItem[];expected_version?:string};
export const releaseFieldSchema=z.object({name:z.string(),type:z.string(),nullable:z.boolean(),editable:z.boolean(),before_state:z.enum(["value","sql_null","absent"]),before:z.string().nullable(),proposed_state:z.enum(["value","sql_null","omitted","absent","automatic","generated"]),proposed:z.string().nullable()});
const canonicalRowSchema=z.object({format:z.literal("rcc-admin-mysql-row-v1"),schema_digest:z.string().regex(/^[a-f0-9]{64}$/),deleted:z.boolean(),fields:z.array(z.object({name:z.string(),type:z.string(),encoding:z.enum(["text","json","base64","sql_null"]),value:z.string().nullable()})),checksum:z.string().regex(/^[a-f0-9]{64}$/)});
const publicationSchema=z.object({table_version:version,publisher_id:z.string(),executed_at:z.string(),notification:z.object({id:z.string(),table_version:version,status:z.literal("NOT_CONNECTED")}),commands:z.array(z.object({order_id:z.string(),sequence:version,table_name:z.string(),table_version:version,operation:z.enum(["ADD","MODIFY","DELETE"]),id:z.string(),record_version:version,before:canonicalRowSchema,final:canonicalRowSchema})).min(1)});
export const releaseOrderSchema=z.object({
 publication:publicationSchema.optional(),
 copied_from_id:z.string().optional(),frozen_digest:z.string().optional(),id:z.string(),table_name:z.string(),applicant_id:z.string(),state:z.enum(["DRAFT","PENDING_APPROVAL","APPROVED","SUCCEEDED","REJECTED","CANCELLED","ROLLED_BACK"]),version,
 items:z.array(draftItemSchema.extend({id:z.string().nullable(),expected_record_version:z.string(),before:content.nullable(),fields:z.array(releaseFieldSchema)})),
 history:z.array(z.object({action:z.string(),actor_id:z.string(),at:z.string(),version,reason:z.string()})),created_at:z.string(),updated_at:z.string(),allowed_actions:z.array(z.string()),
});
export type ReleaseOrder=z.infer<typeof releaseOrderSchema>;
export type ReleaseField=z.infer<typeof releaseFieldSchema>;
export const releaseOrders={
 list:(filters:Record<string,string>)=>request(`/api/v1/release-orders?${new URLSearchParams(filters)}`,{schema:z.object({orders:z.array(releaseOrderSchema),next_cursor:z.string()})}),
 preview:(input:DraftInput)=>request("/api/v1/release-orders/preview",{method:"POST",body:JSON.stringify(input),schema:z.object({table_name:z.string(),items:releaseOrderSchema.shape.items})}),
 get:(id:string)=>request(`/api/v1/release-orders/${encodeURIComponent(id)}`,{schema:releaseOrderSchema}),
 write:(path:string,method:string,body:string,key:string)=>request(path,{method,body,headers:{"Idempotency-Key":key},schema:releaseOrderSchema}),
};
export function draftFromOrder(order:ReleaseOrder):DraftInput{
 return {table_name:order.table_name,expected_version:order.version,items:order.items.map(item=>({operation:item.operation,...(item.operation!=="ADD"?{id:item.id}:{}),expected_record_version:item.expected_record_version,content:{...item.content}}))};
}

// Transport envelopes are serialized here once and retained unchanged for retries.
export type ReleaseRequestEnvelope={path:string;method:"POST"|"PUT";body:string};
const draftInputSchema=z.object({table_name:z.string(),items:z.array(draftItemSchema),expected_version:z.string().optional()});
const cancelInputSchema=z.object({expected_version:z.string(),reason:z.string()});
const submitInputSchema=z.object({expected_version:z.string()});
const copyInputSchema=z.object({expected_version:z.string(),confirmed:z.literal(true),items:z.array(draftItemSchema)});
export type ReleaseStateAction="submit"|"approve"|"reject"|"cancel"|"execute";
export const releaseActionLabels={execute:"执行发布",submit:"提交审批",approve:"批准发布单",reject:"拒绝发布单",cancel:"取消发布单",copy:"复制新草稿"};
export const releaseActionRole=(action:string)=>action==="execute"?"PUBLISHER" as const:action==="approve"||action==="reject"?"APPROVER" as const:"EDITOR" as const;

export const releaseRequests={
 action:(action:ReleaseStateAction,id:string,expectedVersion:string,reason=""):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/${action}`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,...(action!=="submit"&&action!=="execute"?{reason}:{})})}),
 copy:(id:string,expectedVersion:string,items:DraftItem[]):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/copy`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,confirmed:true,items})}),
 create:(input:DraftInput):ReleaseRequestEnvelope=>({path:"/api/v1/release-orders",method:"POST",body:JSON.stringify(input)}),
 edit:(id:string,input:DraftInput):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}`,method:"PUT",body:JSON.stringify(input)}),
 cancel:(id:string,expectedVersion:string,reason:string):ReleaseRequestEnvelope=>({path:`/api/v1/release-orders/${encodeURIComponent(id)}/cancel`,method:"POST",body:JSON.stringify({expected_version:expectedVersion,reason})}),
};
export function decodeReleaseRequest(value:ReleaseRequestEnvelope){
 const id=value.path.split("/")[4];
 const body:unknown=JSON.parse(value.body);
 const action=value.path.split("/")[5];
 if(id&&action==="execute")return {action:"execute" as const,id,input:submitInputSchema.parse(body)};
 if(id&&action==="submit")return {action:"submit" as const,id,input:submitInputSchema.parse(body)};
 if(id&&action==="copy")return {action:"copy" as const,id,input:copyInputSchema.parse(body)};
 if(id&&action==="cancel")return {action:"cancel" as const,id,input:cancelInputSchema.parse(body)};
 if(id&&action==="approve")return {action:"approve" as const,id,input:cancelInputSchema.parse(body)};
 if(id&&action==="reject")return {action:"reject" as const,id,input:cancelInputSchema.parse(body)};
 const input=draftInputSchema.parse(body);
 return id?{action:"edit" as const,id,input}:{action:"create" as const,input};
}
