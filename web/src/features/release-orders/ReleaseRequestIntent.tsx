import {decodeReleaseRequest,type DraftItem} from "../../api/release-orders";
import type {PendingReleaseRequest} from "./release-journal";

export function PendingIntent({item}:{item:PendingReleaseRequest}){
 let intent:ReturnType<typeof decodeReleaseRequest>;
 try{intent=decodeReleaseRequest(item)}catch{return <p>原申请内容无法读取；原请求标识仍保留。</p>}
 if(intent.action==="quick-rollback-preview")return <details className="my-2"><summary>查看原恢复预览请求</summary><p>原发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p><p>应急恢复流程保存请求已保留，再次保存将提交原正文与请求标识。</p></details>;
 if(intent.action==="quick-rollback")return <details className="my-2"><summary>查看原申请内容</summary><p>原发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p><p>快速回滚原因：{intent.input.reason}</p><p>原恢复预览摘要已保留，再次操作将提交原请求。</p></details>;
 if(intent.action==="edit-rollback-reason")return <details className="my-2"><summary>查看原申请内容</summary><p>原发布单号：{intent.id}</p><p>回滚原因：{intent.input.reason}</p></details>;
 if(intent.action==="complete")return <details className="my-2"><summary>查看原申请内容</summary><p>完结发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p><p>释放全部目标记录的占用，并关闭快速回滚；配置内容保持不变。</p></details>;
 if(intent.action==="execute")return <details className="my-2"><summary>查看原申请内容</summary><p>发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p></details>;
 if(intent.action==="submit")return <details className="my-2"><summary>查看原申请内容</summary><p>提交单号：{intent.id}，发布单版本：{intent.input.expected_version}</p>{intent.input.emergency_reason&&<p className="whitespace-pre-wrap">应急原因：{intent.input.emergency_reason}</p>}</details>;
 if(intent.action==="approve"||intent.action==="reject")return <details className="my-2"><summary>查看原申请内容</summary><p>审批意见：{intent.input.reason}</p><p className="break-all">原确认表范围：{intent.input.confirmed_tables.join("、")}</p><p>原发布单版本：{intent.input.expected_version}</p></details>;
 if(intent.action==="cancel")return <details className="my-2"><summary>查看原申请内容</summary><p>{intent.action==="cancel"?"取消原因":"审批意见"}：{intent.input.reason}</p></details>;
 if(intent.action==="edit-details")return <details className="my-2"><summary>查看原申请内容</summary><p>整单版本 {intent.input.expected_version}</p>{intent.input.release_type&&<p>切换发布方式：{intent.input.release_type==="EMERGENCY"?"应急发布":"常规发布"}</p>}<p>删除明细：{intent.input.changes.delete_detail_ids?.join("、")||"无"}</p><p>明细顺序：{intent.input.changes.detail_order?.join("、")||"保持"}</p>{intent.input.changes.upserts?.map((entry,index)=>{
  const position=entry.detail_id?intent.input.changes.detail_order?.indexOf(entry.detail_id):undefined;
  return <PendingDetail key={entry.detail_id??index} entry={entry} position={position!==undefined&&position>=0?position+1:undefined}/>;
 })}</details>;
 return <details className="my-2"><summary>查看原申请内容</summary><p>{intent.action==="copy"?`复制原单 ${intent.id}`:intent.action==="reprepare"?`重新准备原单 ${intent.id}`:intent.input.title}</p>{"release_type" in intent.input&&intent.input.release_type&&<p>发布方式：{intent.input.release_type==="EMERGENCY"?"应急发布":"常规发布"}</p>}{intent.input.items.map((entry,index)=><PendingDetail key={index} entry={entry} position={index+1}/>)}</details>;
}
function PendingDetail({entry,position}:{entry:DraftItem;position?:number}){
 return <div><strong>{position?`明细 ${position}`:"变更明细"} · {entry.table_name} · {entry.operation} · 记录 {entry.id??entry.content.id??"待生成 id"}</strong>{entry.detail_id&&<p className="break-all text-xs">明细标识：{entry.detail_id}</p>}<dl>{Object.entries(entry.content).map(([field,value])=><div key={field} className="break-all"><dt>{field}</dt><dd className="whitespace-pre-wrap">{value===null?"SQL NULL":value===""?"空字符串（\"\"）":<>值：{value}</>}</dd></div>)}</dl></div>;

}

export function pendingRequestReason(item:PendingReleaseRequest|undefined):string {
 if(!item)return "";
 try{const {input}=decodeReleaseRequest(item);return "reason" in input?input.reason:"emergency_reason" in input?input.emergency_reason??"":""}catch{return ""}
}
