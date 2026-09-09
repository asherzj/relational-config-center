import {useEffect,useState} from "react";
import {Link,useNavigate} from "react-router-dom";
import {decodeReleaseRequest,draftFromOrder,releaseActionLabels,releaseActionRole,releaseOrders,releaseRequests,type ReleaseOrder,type ReleaseRequestEnvelope} from "../../api/release-orders";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {useAccountRole} from "../accounts/roles";
import {Button} from "../../components/ui/Button";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
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
 return <section className="inline-alert release-recovery mb-6" aria-label="待处理发布请求"><h2>待处理发布请求</h2>{selected.map(item=><div key={item.key} className="mt-3"><p>{item.label}</p><PendingIntent item={item}/>{item.rejection?<RejectedRequest item={item}/>:<RecoveryRequest item={item}/>}</div>)}</section>;
}
function requestAction(item:PendingReleaseRequest){try{return decodeReleaseRequest(item).action}catch{return undefined}}
function RecoveryRequest({item}:{item:PendingReleaseRequest}){
 const action=requestAction(item),write=useReleaseWrite(item.scope),navigate=useNavigate();const allowed=useAccountRole(releaseActionRole(action??""));
 const protection=useDraftProtection(false,write.pending);
 return <><p>结果待确认。原内容与标识已保留，使用原请求重试可找回业务结果。</p><Button disabled={!action||!allowed||write.pending} onClick={async()=>{
  const result=await write.send(item);if(result)protection.afterSave(()=>navigate(`/configuration/release-orders/${result.id}`));
 }}>{write.pending?"正在确认…":"恢复原发布请求"}</Button>{Boolean(write.error)&&<ErrorState error={write.error}/>}</>;
}

// A definitive rejection resolves uncertainty, but never removes the only copy
// of the user's intent. Every state action is rebuilt only after current review.
function RejectedRequest({item}:{item:PendingReleaseRequest}){
 const [current,setCurrent]=useState<ReleaseOrder>();
 const [preview,setPreview]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const [rebuilt,setRebuilt]=useState<ReleaseRequestEnvelope>();
 const [needsDraftUpdate,setNeedsDraftUpdate]=useState(false);
 const [reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const action=requestAction(item),allowed=useAccountRole(releaseActionRole(action??"")),write=useReleaseWrite(item.scope),navigate=useNavigate();
 const protection=useDraftProtection(false,write.pending);
 const inspect=async()=>{
  setReading(true);setError(undefined);setRebuilt(undefined);setPreview(undefined);setNeedsDraftUpdate(false);
  try{
   const intent=decodeReleaseRequest(item);
   const latest=intent.action==="create"?undefined:await releaseOrders.get(intent.id);setCurrent(latest);
   if(intent.action==="execute"||intent.action==="complete"){
    if(latest?.allowed_actions.includes(intent.action))setRebuilt(releaseRequests.action(intent.action,intent.id,latest.version));
   }else if(intent.action==="quick-rollback"){
    if(latest?.allowed_actions.includes("quick-rollback")){
     const restoration=await releaseOrders.quickRollbackPreview(intent.id,latest.version);setPreview(restoration);
     setRebuilt(releaseRequests.quickRollback(intent.id,latest.version,restoration.preview_digest,intent.input.reason));
    }
   }else if(intent.action==="rollback"){
    if(latest?.allowed_actions.includes("rollback"))setRebuilt(releaseRequests.action("rollback",intent.id,latest.version,intent.input.reason));
   }else if(intent.action==="cancel"||intent.action==="approve"||intent.action==="reject"){
    if(latest?.allowed_actions.includes(intent.action))setRebuilt(releaseRequests.action(intent.action,intent.id,latest.version,intent.input.reason));
   }else if(intent.action==="submit"){
    if(latest?.allowed_actions.includes("submit")){
     if(latest.rollback_of_id)setRebuilt(releaseRequests.action("submit",intent.id,latest.version));
     else{
      const snapshot=await releaseOrders.preview(draftFromOrder(latest));setPreview(snapshot);
      const changed=snapshot.items.some((entry,index)=>entry.expected_record_version!==latest.items[index]!.expected_record_version);
      setNeedsDraftUpdate(changed);if(!changed)setRebuilt(releaseRequests.action("submit",intent.id,latest.version));
     }
    }
   }else if(intent.action==="copy"||intent.action==="reprepare"){
    if(latest?.allowed_actions.includes(intent.action)){
     const snapshot=await releaseOrders.preview({table_name:latest.table_name,items:intent.input.items});setPreview(snapshot);
     const items=intent.input.items.map((entry,index)=>({...entry,expected_record_version:snapshot.items[index]!.expected_record_version}));
     setRebuilt(intent.action==="copy"?releaseRequests.copy(intent.id,latest.version,items):releaseRequests.reprepare(intent.id,latest.version,items));
    }
   }else if(intent.action==="create"||latest?.allowed_actions.includes("edit")){
    const snapshot=await releaseOrders.preview(intent.input);setPreview(snapshot);
    const input={...intent.input,items:intent.input.items.map((entry,index)=>({...entry,expected_record_version:snapshot.items[index]!.expected_record_version}))};
    setRebuilt(intent.action==="create"?releaseRequests.create(input):releaseRequests.edit(intent.id,{...input,expected_version:latest!.version}));
   }
  }catch(cause){setError(cause)}finally{setReading(false)}
 };
 const confirmLabel=action==="cancel"?`确认按最新状态取消${current?.state==="DRAFT"?"草稿":"发布单"}`:action==="create"||action==="edit"?"确认重建并保存草稿":`确认按最新状态${action?releaseActionLabels[action]:"重建"}`;
 return <><p role="alert">服务器已明确拒绝原请求。原申请保留，请查看最新状态与配置后决定是否重建。</p>
  <Button disabled={!action||!allowed||reading||write.pending} onClick={()=>void inspect()}>{reading?"正在检查…":"查看最新状态与配置"}</Button>{reading&&<LoadingState label="正在检查最新状态与配置…"/>}
  {current&&<><p>最新发布单版本：{current.version}，状态：{current.state}</p>{current.rollback_order_id&&<p>当前关联回滚发布单：<Link to={`/configuration/release-orders/${current.rollback_order_id}`}>{current.rollback_order_id}</Link></p>}<ReleaseDiff order={current}/></>}
  {preview&&<><p>{action==="quick-rollback"?"重新审阅整单恢复预览，确认后将使用新的请求标识执行：":"原申请与最新记录基线的差异："}</p><ReleaseDiff order={{items:preview.items}} beforeLabel={action==="quick-rollback"?"当前值":undefined} proposedLabel={action==="quick-rollback"?"恢复值":undefined}/></>}
  {needsDraftUpdate&&current&&<p>草稿记录基线已变化，请先<Link to={`/configuration/release-orders/${current.id}`}>编辑草稿并核对最新配置</Link>，再重新检查提交。</p>}
  {action==="quick-rollback"&&current&&!current.allowed_actions.includes("quick-rollback")&&<p>当前发布单已不能快速回滚，原原因保留供核对。</p>}
  {(current||preview)&&(action!=="quick-rollback"||Boolean(rebuilt))&&<Button disabled={write.blocked||!allowed||reading||write.pending||!rebuilt} onClick={async()=>{
   if(!rebuilt)return;write.confirmRebuild();const result=await write.send({...rebuilt,label:item.label});
   if(result)protection.afterSave(()=>navigate(`/configuration/release-orders/${result.id}`));
  }}>{confirmLabel}</Button>}
  {Boolean(error)&&<ErrorState error={error}/>} {Boolean(write.error)&&<ErrorState error={write.error}/>}
 </>;
}
function PendingIntent({item}:{item:PendingReleaseRequest}){
 let intent:ReturnType<typeof decodeReleaseRequest>;
 try{intent=decodeReleaseRequest(item)}catch{return <p>原申请内容无法读取；原请求标识仍保留。</p>}
 if(intent.action==="quick-rollback")return <details className="my-2"><summary>查看原申请内容</summary><p>原发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p><p>快速回滚原因：{intent.input.reason}</p><p>原恢复预览摘要已保留，将使用原请求确认结果。</p></details>;
 if(intent.action==="complete")return <details className="my-2"><summary>查看原申请内容</summary><p>完结发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p><p>释放全部目标记录的占用，并关闭快速回滚；配置内容保持不变。</p></details>;
 if(intent.action==="execute")return <details className="my-2"><summary>查看原申请内容</summary><p>发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p></details>;
 if(intent.action==="submit")return <details className="my-2"><summary>查看原申请内容</summary><p>提交单号：{intent.id}，发布单版本：{intent.input.expected_version}</p></details>;
 if(intent.action==="rollback")return <details className="my-2"><summary>查看原申请内容</summary><p>原发布单号：{intent.id}，发布单版本：{intent.input.expected_version}</p><p>回滚原因：{intent.input.reason}</p></details>;
 if(intent.action==="cancel"||intent.action==="approve"||intent.action==="reject")return <details className="my-2"><summary>查看原申请内容</summary><p>{intent.action==="cancel"?"取消原因":"审批意见"}：{intent.input.reason}</p></details>;
 return <details className="my-2"><summary>查看原申请内容</summary><p>{intent.action==="copy"?`复制原单 ${intent.id}`:intent.action==="reprepare"?`重新准备原单 ${intent.id}`:intent.input.table_name}</p>{intent.input.items.map((entry,index)=><div key={index}><strong>{entry.operation} · {entry.id??entry.content.id??"待生成 id"}</strong><dl>{Object.entries(entry.content).map(([field,value])=><div key={field} className="break-all"><dt>{field}</dt><dd className="whitespace-pre-wrap">{value===null?"SQL NULL":value===""?"空字符串（\"\"）":<>值：{value}</>}</dd></div>)}</dl></div>)}</details>;
}
