import {useEffect,useState} from "react";
import {useNavigate} from "react-router-dom";
import {decodeReleaseRequest,releaseOrders,releaseRequests,type ReleaseOrder,type ReleaseRequestEnvelope} from "../../api/release-orders";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {useAccountRole} from "../accounts/roles";
import {Button} from "../../components/ui/Button";
import {ErrorState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {pendingReleaseRequests,releaseJournalChanged,type PendingReleaseRequest} from "./release-journal";
import {useReleaseWrite} from "./useReleaseWrite";
import {ReleaseDiff} from "./ReleaseDiff";

export function ReleaseRecovery({scopeFilter}:{scopeFilter?:string}){
 const accountID=useWorkspaceIdentity()!.account.id;
 const [requests,setRequests]=useState(()=>pendingReleaseRequests(accountID));
 useEffect(()=>{const update=()=>setRequests(pendingReleaseRequests(accountID));window.addEventListener(releaseJournalChanged,update);return()=>window.removeEventListener(releaseJournalChanged,update)},[accountID]);
 const selected=requests.filter(item=>!scopeFilter||item.scope===scopeFilter);
 if(!selected.length)return null;
 return <section className="inline-alert mb-6" aria-label="待处理发布请求"><h2>待处理发布请求</h2>{selected.map(item=><div key={item.key} className="mt-3"><p>{item.label}</p><PendingIntent item={item}/>{item.rejection?<RejectedRequest item={item}/>:<RecoveryRequest item={item}/>}</div>)}</section>;
}
function RecoveryRequest({item}:{item:PendingReleaseRequest}){
 const write=useReleaseWrite(item.scope),navigate=useNavigate();const allowed=useAccountRole("EDITOR");
 const protection=useDraftProtection(false,write.pending);
 return <><p>结果待确认。原内容与标识已保留，使用原请求重试可找回业务结果。</p><Button disabled={!allowed||write.pending} onClick={async()=>{
  const result=await write.send(item);if(result)protection.afterSave(()=>navigate(`/configuration/release-orders/${result.id}`));
 }}>{write.pending?"正在确认…":"恢复原发布请求"}</Button>{Boolean(write.error)&&<ErrorState error={write.error}/>}</>;
}

// A definitive rejection resolves uncertainty, but never removes the only copy
// of the user's intent. The original remains durable until explicit rebuilding.
function RejectedRequest({item}:{item:PendingReleaseRequest}){
 const [current,setCurrent]=useState<ReleaseOrder>();
 const [preview,setPreview]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const [rebuilt,setRebuilt]=useState<ReleaseRequestEnvelope>();
 const [cancelAction,setCancelAction]=useState(false);
 const [reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const allowed=useAccountRole("EDITOR"),write=useReleaseWrite(item.scope),navigate=useNavigate();
 const protection=useDraftProtection(false,write.pending);
 const inspect=async()=>{
  setReading(true);setError(undefined);setRebuilt(undefined);setPreview(undefined);
  try{
   const intent=decodeReleaseRequest(item);setCancelAction(intent.action==="cancel");
   const latest=intent.action==="create"?undefined:await releaseOrders.get(intent.id);
   setCurrent(latest);
   if(intent.action==="cancel"){
    if(latest?.allowed_actions.includes("cancel"))setRebuilt(releaseRequests.cancel(intent.id,latest.version,intent.input.reason));
   }else if(intent.action==="create"||latest?.allowed_actions.includes("edit")){
    const snapshot=await releaseOrders.preview(intent.input);setPreview(snapshot);
    const input={...intent.input,items:intent.input.items.map((entry,index)=>({...entry,expected_record_version:snapshot.items[index]!.expected_record_version}))};
    setRebuilt(intent.action==="create"?releaseRequests.create(input):releaseRequests.edit(intent.id,{...input,expected_version:latest!.version}));
   }
  }catch(cause){setError(cause)}finally{setReading(false)}
 };
 return <><p role="alert">服务器已明确拒绝原请求。原申请保留，请查看最新状态与配置后决定是否重建。</p>
  <Button disabled={!allowed||reading||write.pending} onClick={()=>void inspect()}>查看最新状态与配置</Button>
  {current&&<><p>最新发布单版本：{current.version}，状态：{current.state}</p><ReleaseDiff order={current}/></>}
  {preview&&<><p>原申请与最新记录基线的差异：</p><ReleaseDiff order={{items:preview.items}}/></>}
  {(current||preview)&&<Button disabled={!allowed||reading||write.pending||!rebuilt} onClick={async()=>{
   if(!rebuilt)return;write.confirmRebuild();const result=await write.send({...rebuilt,label:item.label});
   if(result)protection.afterSave(()=>navigate(`/configuration/release-orders/${result.id}`));
  }}>{cancelAction?"确认按最新状态取消草稿":"确认重建并保存草稿"}</Button>}
  {Boolean(error)&&<ErrorState error={error}/>} {Boolean(write.error)&&<ErrorState error={write.error}/>}
 </>;
}

function PendingIntent({item}:{item:PendingReleaseRequest}){
 let intent:ReturnType<typeof decodeReleaseRequest>;
 try{intent=decodeReleaseRequest(item)}catch{return <p>原申请内容无法读取；原请求标识仍保留。</p>}
 return <details className="my-2"><summary>查看原申请内容</summary>{intent.action==="cancel"?<p>取消原因：{intent.input.reason}</p>:<><p>{intent.input.table_name}</p>{intent.input.items.map((entry,index)=><div key={index}><strong>{entry.operation} · {entry.id??entry.content.id??"待生成 id"}</strong><dl>{Object.entries(entry.content).map(([field,value])=><div key={field} className="break-all"><dt>{field}</dt><dd className="whitespace-pre-wrap">{value===null?"SQL NULL":value===""?"空字符串（\"\"）":<>值：{value}</>}</dd></div>)}</dl></div>)}</>}</details>;
}
