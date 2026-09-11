import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {ApiError} from "../../api/client";
import {loadReleaseForEdit,type ReleaseHeader,draftFromOrder,releaseDetailTables,incrementalDraft,rebaseDraftInput,releaseOrders,releaseRequests,releaseTitleError,type ReleaseOrder} from "../../api/release-orders";
import {useToast} from "../../components/ui/Toast";
import {Drawer} from "../../components/ui/Drawer";
import {Button} from "../../components/ui/Button";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {CurrentFieldDisplayProvider} from "../field-display/CurrentFieldDisplay";
import {ReleaseDiff} from "./ReleaseDiff";
import {PendingIntent} from "./ReleaseRequestReview";
import {ReleaseItemPager,releasePageSize} from "./ReleaseItemPager";
import {ManagedTextInput} from "../managed-data/ManagedTextInput";

type FieldInput={state:"omitted"|"value"|"sql_null";value:string};
export function ReleaseDraftEditor({order,onClose}:{order:ReleaseHeader;onClose:()=>void}){
 const [source,setSource]=useState(order);
 const input=useQuery({queryKey:["release-edit-input",source.id,source.version],queryFn:()=>loadReleaseForEdit(source),retry:false,refetchOnWindowFocus:false});
 if(!input.data)return <Drawer open eyebrow="发布草稿" title="编辑多表草稿" onClose={onClose}>{input.isPending?<LoadingState/>:<ErrorState error={input.error} onRetry={()=>{if(order.version!==source.version)setSource(order);else void input.refetch()}}/>}</Drawer>;
 return <LoadedReleaseDraftEditor order={input.data} current={order} onClose={onClose}/>;
}
function LoadedReleaseDraftEditor({order,current,onClose}:{order:ReleaseOrder;current:ReleaseHeader;onClose:()=>void}){
 const {showToast}=useToast();
 const [baseline,setBaseline]=useState(order);
 const [title,setTitle]=useState(order.title);
 const [items,setItems]=useState(order.items);
 const [selected,setSelected]=useState(0);
 const [latest,setLatest]=useState<ReleaseOrder>();const [readError,setReadError]=useState<unknown>();const [reading,setReading]=useState(false);
 const [recordLatest,setRecordLatest]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const write=useReleaseWrite(`edit:${order.id}`);
 const [originalKey]=useState(write.storedRequest?.key);
 const hasRole=useAccountRole("EDITOR");
 const allowed=hasRole&&current.allowed_actions.includes("edit")&&current.state==="DRAFT";
 const canRepeat=hasRole&&Boolean(write.storedRequest);
 const conflict=write.error instanceof ApiError&&["release_version_conflict","release_state_invalid","release_target_conflict"].includes(write.error.code);
 const recordConflict=write.error instanceof ApiError&&write.error.code==="record_version_conflict";
 const titleError=releaseTitleError(title);
 const protection=useDraftProtection(true,write.pending);
 const item=items[selected];
 const page=Math.floor(selected/releasePageSize);
 const fields:Record<string,FieldInput>=Object.fromEntries((item?.fields??[]).map(field=>{
  const supplied=Object.hasOwn(item.content,field.name),value=item.content[field.name];
  return [field.name,{state:supplied?(value===null?"sql_null":"value"):"omitted",value:value??""}];
 }));
 const currentInput=()=>({...draftFromOrder({...baseline,items}),title});
 const updateField=(name:string,value:FieldInput)=>{
  setItems(current=>current.map((entry,index)=>{
   if(index!==selected)return entry;
   const content={...entry.content};if(value.state==="omitted")delete content[name];else content[name]=value.state==="sql_null"?null:value.value;
   return {...entry,content};
  }));setRecordLatest(undefined);
 };
 const save=async()=>{
  const saved=originalKey&&write.unresolved&&originalKey===write.storedRequest?.key?await write.retry():await write.send({...releaseRequests.edit(order.id,incrementalDraft(baseline,{...baseline,title,items})),label:`修改 ${order.id}`});
  if(saved){showToast("草稿已保存");protection.afterSave(onClose)}
 };
 const inspect=async()=>{
  if(reading)return;setReading(true);setReadError(undefined);
  try{setLatest(await loadReleaseForEdit(await releaseOrders.get(order.id)))}catch(cause){setReadError(cause)}finally{setReading(false)}
 };
 const inspectRecord=async()=>{
  if(reading)return;setReading(true);setReadError(undefined);
  try{setRecordLatest(await releaseOrders.preview(currentInput()))}catch(cause){setReadError(cause)}finally{setReading(false)}
 };
 return <Drawer open eyebrow="发布草稿" title="编辑多表草稿" onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant="primary" disabled={write.blocked||(!allowed&&!canRepeat)||Boolean(titleError)||write.pending||reading||conflict||recordConflict} onClick={()=>void save()}>{write.pending?"正在保存…":"保存草稿修改"}</Button></>}>
 {!allowed&&<p role="alert">当前身份或发布单状态不允许编辑，已输入内容保留。</p>}
 <p>保存草稿即占用目标，直到移除最后一条引用或发布单结束。只提交本次明细变更，全部分页共用整单版本。</p>
 {baseline.missing_flow_tables.length>0&&<p className="mt-3 text-warning">缺失常规流程：{baseline.missing_flow_tables.join("、")}。管理员修复配置后，可直接保存，无需修改内容；保存仅补齐缺失流程，已有实例保持不变。</p>}
 <div className="grid gap-2 my-5"><label htmlFor="release-order-title">发布单标题</label><Input id="release-order-title" value={title} required aria-invalid={Boolean(titleError)} aria-describedby={`release-order-title-count${titleError?" release-order-title-error":""}`} disabled={write.blocked||!allowed||write.pending||write.unresolved} onChange={event=>setTitle(event.target.value)}/><span id="release-order-title-count" className="text-xs text-muted-foreground">{Array.from(title).length} / 100 字符</span>{titleError&&<small id="release-order-title-error" className="field-error">{titleError}</small>}</div>
 <fieldset disabled={write.pending||write.unresolved}><ReleaseItemPager count={items.length} page={page} onPage={page=>setSelected(page*releasePageSize)} onLocate={setSelected} label="编辑明细"/></fieldset>
 <div className="flex flex-wrap items-end gap-3 my-5"><label>编辑明细<NativeSelect aria-label="编辑明细" value={selected} disabled={write.pending||write.unresolved} onChange={event=>setSelected(Number(event.target.value))}>{items.slice(page*releasePageSize,(page+1)*releasePageSize).map((entry,offset)=>{const index=page*releasePageSize+offset;return <option key={index} value={index}>明细 {index+1} · {entry.table_name} · {entry.operation} · {entry.id??"待生成 id"}</option>})}</NativeSelect></label><Button disabled={write.blocked||!allowed||items.length===0||write.pending||write.unresolved} onClick={()=>{setItems(current=>current.filter((_,index)=>index!==selected));setSelected(Math.max(0,selected-1));setRecordLatest(undefined)}}>移除此明细</Button><p>共 {items.length} 项。保存只提交本次修改、删除和排序。</p></div>
 <div className="flex gap-2"><Button disabled={!allowed||write.pending||write.unresolved||selected===0} onClick={()=>{setItems(current=>{const next=[...current];[next[selected-1],next[selected]]=[next[selected]!,next[selected-1]!];return next});setSelected(selected-1)}}>上移明细</Button><Button disabled={!allowed||write.pending||write.unresolved||selected>=items.length-1} onClick={()=>{setItems(current=>{const next=[...current];[next[selected],next[selected+1]]=[next[selected+1]!,next[selected]!];return next});setSelected(selected+1)}}>下移明细</Button></div>
 {!item&&<p>草稿暂无明细，可保存空草稿后从统一变更入口添加。</p>}
 <fieldset disabled={write.blocked||!allowed||write.pending||write.unresolved||reading} className="grid min-w-0 gap-5 mt-5">{(item?.fields??[]).filter(field=>field.editable).map(field=>{
  const value=fields[field.name]??{state:"omitted",value:""};
  return <div key={field.name} className="grid min-w-0 gap-2"><label className="min-w-0 break-all">{field.name} 提交方式<NativeSelect aria-label={`${field.name} 提交方式`} value={value.state} onChange={event=>updateField(field.name,{...value,state:event.target.value as FieldInput["state"]})}><option value="omitted">未提交</option><option value="value">提交值（可为空字符串）</option>{field.nullable&&<option value="sql_null">SQL NULL</option>}</NativeSelect></label>{field.type==="string"||field.type==="json"?<ManagedTextInput label={`${field.name} 申请值`} value={value.value} disabled={value.state!=="value"} onChange={next=>updateField(field.name,{...value,value:next})}/>:<label className="min-w-0 break-all">{field.name} 申请值<Input value={value.value} disabled={value.state!=="value"} onChange={event=>updateField(field.name,{...value,value:event.target.value})}/></label>}</div>
 })}</fieldset>
 {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}
 {Boolean(write.error)&&<ErrorState error={write.error}/>}
 {write.error instanceof ApiError&&write.error.itemIndex!==undefined&&write.error.itemIndex<items.length&&<Button onClick={()=>setSelected((write.error as ApiError).itemIndex!)}>定位错误明细</Button>}
 {write.unresolved&&<p role="alert">原请求与全部输入已保留；再次保存将提交原请求。</p>}
 {conflict&&<section className="inline-alert"><p>{write.error instanceof ApiError&&write.error.code==="release_target_conflict"?"目标被另一张发布单占用。修改后的输入已保留，请核对当前草稿再重建保存。":"发布单已被其他窗口修改。你的输入已保留，请先查看最新发布单。"}</p><Button disabled={reading} onClick={()=>void inspect()}>查看最新发布单</Button>{latest&&<><p>最新发布单版本：{latest.version}，状态：{latest.state}</p><CurrentFieldDisplayProvider tableNames={releaseDetailTables(latest)}><ReleaseDiff order={latest}/></CurrentFieldDisplayProvider><Button disabled={!latest.allowed_actions.includes("edit")||latest.state!=="DRAFT"} onClick={()=>{const rebuilt=rebaseDraftInput(baseline,{...baseline,title,items},latest);setItems(rebuilt.items);setSelected(current=>Math.min(current,Math.max(0,rebuilt.items.length-1)));setTitle(rebuilt.title);setBaseline(latest);setLatest(undefined);write.confirmRebuild();write.clearError()}}>基于最新发布单重建</Button></>}</section>}

 {(recordConflict||item?.operation==="ADD")&&<section className="inline-alert"><p>{recordConflict?"配置记录基线已变化，输入保留。请核对最新配置后明确重建。":"更换新增 id 或记录基线变化时，先查看该目标的最新基线。"}</p><Button disabled={reading||write.pending||write.unresolved} onClick={()=>void inspectRecord()}>查看最新配置</Button>{recordLatest&&<><p>已读取服务器最新记录基线；此预览没有保存或执行任何变更。</p><CurrentFieldDisplayProvider tableNames={releaseDetailTables(recordLatest)}><ReleaseDiff order={{...baseline,items:recordLatest.items}}/></CurrentFieldDisplayProvider><Button disabled={write.pending||write.unresolved} onClick={()=>{
  setItems(recordLatest.items);setRecordLatest(undefined);write.confirmRebuild();write.clearError();
 }}>基于最新配置重建</Button></>}</section>}

 {Boolean(readError)&&<ErrorState error={readError}/>}
 </Drawer>;
}
