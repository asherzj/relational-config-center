import {useState} from "react";
import {ApiError} from "../../api/client";
import {draftFromOrder,releaseOrders,releaseRequests,type ReleaseOrder} from "../../api/release-orders";
import {Drawer} from "../../components/ui/Drawer";
import {Button} from "../../components/ui/Button";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {ErrorState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {ReleaseDiff} from "./ReleaseDiff";
import {ReleaseItemPager,releasePageSize} from "./ReleaseItemPager";

type FieldInput={state:"omitted"|"value"|"sql_null";value:string};
export function ReleaseDraftEditor({order,onClose}:{order:ReleaseOrder;onClose:()=>void}){
 const [baseline,setBaseline]=useState(order);
 const [items,setItems]=useState(order.items);
 const [selected,setSelected]=useState(0);
 const [latest,setLatest]=useState<ReleaseOrder>();const [readError,setReadError]=useState<unknown>();const [reading,setReading]=useState(false);
 const [recordLatest,setRecordLatest]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const write=useReleaseWrite(`edit:${order.id}`);
 const allowed=useAccountRole("EDITOR")&&baseline.allowed_actions.includes("edit")&&baseline.state==="DRAFT";
 const conflict=write.error instanceof ApiError&&["release_version_conflict","release_state_invalid"].includes(write.error.code);
 const recordConflict=write.error instanceof ApiError&&write.error.code==="record_version_conflict";
 const protection=useDraftProtection(true,write.pending);
 const item=items[selected]!;
 const page=Math.floor(selected/releasePageSize);
 const fields:Record<string,FieldInput>=Object.fromEntries(item.fields.map(field=>{
  const supplied=Object.hasOwn(item.content,field.name),value=item.content[field.name];
  return [field.name,{state:supplied?(value===null?"sql_null":"value"):"omitted",value:value??""}];
 }));
 const currentInput=()=>draftFromOrder({...baseline,items});
 const updateField=(name:string,value:FieldInput)=>{
  setItems(current=>current.map((entry,index)=>{
   if(index!==selected)return entry;
   const content={...entry.content};if(value.state==="omitted")delete content[name];else content[name]=value.state==="sql_null"?null:value.value;
   return {...entry,content};
  }));setRecordLatest(undefined);
 };
 const save=async()=>{
  const saved=await write.send({...releaseRequests.edit(order.id,currentInput()),label:`修改 ${order.id}`});
  if(saved){protection.afterSave(onClose)}
 };
 const inspect=async()=>{
  if(reading)return;setReading(true);setReadError(undefined);
  try{setLatest(await releaseOrders.get(order.id))}catch(cause){setReadError(cause)}finally{setReading(false)}
 };
 const inspectRecord=async()=>{
  if(reading)return;setReading(true);setReadError(undefined);
  try{setRecordLatest(await releaseOrders.preview(currentInput()))}catch(cause){setReadError(cause)}finally{setReading(false)}
 };
 return <Drawer open eyebrow="发布草稿" title={`编辑 ${order.table_name} 草稿`} onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant="primary" disabled={!allowed||write.pending||reading||conflict||recordConflict} onClick={()=>void save()}>{write.pending?"正在保存…":write.unresolved?"使用原请求重试":"保存草稿修改"}</Button></>}>
 {!allowed&&<p role="alert">当前身份或发布单状态不允许编辑，已输入内容保留。</p>}
 <p>保存只修改草稿。未提交字段保持原意，自动字段在正式发布时生成。</p>
 <fieldset disabled={write.pending||write.unresolved}><ReleaseItemPager count={items.length} page={page} onPage={page=>setSelected(page*releasePageSize)} onLocate={setSelected} label="编辑明细"/></fieldset>
 <div className="flex flex-wrap items-end gap-3 my-5"><label>编辑明细<NativeSelect aria-label="编辑明细" value={selected} disabled={write.pending||write.unresolved} onChange={event=>setSelected(Number(event.target.value))}>{items.slice(page*releasePageSize,(page+1)*releasePageSize).map((entry,offset)=>{const index=page*releasePageSize+offset;return <option key={index} value={index}>明细 {index+1} · {entry.operation} · {entry.id??"待生成 id"}</option>})}</NativeSelect></label><Button disabled={!allowed||items.length<=1||write.pending||write.unresolved} onClick={()=>{setItems(current=>current.filter((_,index)=>index!==selected));setSelected(Math.max(0,selected-1));setRecordLatest(undefined)}}>移除此明细</Button><p>共 {items.length} 项。保存将更新整张草稿。</p></div>
 <fieldset disabled={!allowed||write.pending||write.unresolved||reading} className="grid gap-5 mt-5">{item.fields.filter(field=>field.editable).map(field=>{
  const value=fields[field.name]??{state:"omitted",value:""};
  return <div key={field.name} className="grid gap-2"><label>{field.name} 提交方式<NativeSelect aria-label={`${field.name} 提交方式`} value={value.state} onChange={event=>updateField(field.name,{...value,state:event.target.value as FieldInput["state"]})}><option value="omitted">未提交</option><option value="value">提交值（可为空字符串）</option>{field.nullable&&<option value="sql_null">SQL NULL</option>}</NativeSelect></label><label>{field.name} 申请值<Input value={value.value} disabled={value.state!=="value"} onChange={event=>updateField(field.name,{...value,value:event.target.value})}/></label></div>
 })}</fieldset>
 {Boolean(write.error)&&<ErrorState error={write.error}/>}
 {write.error instanceof ApiError&&write.error.itemIndex!==undefined&&write.error.itemIndex<items.length&&<Button onClick={()=>setSelected((write.error as ApiError).itemIndex!)}>定位错误明细</Button>}
 {write.unresolved&&<p role="alert">结果待确认。原请求与全部输入已保留，刷新后也可从发布单页使用原请求重试。</p>}
 {conflict&&<section className="inline-alert"><p>发布单已被其他窗口修改。你的输入已保留，请先查看最新发布单。</p><Button disabled={reading} onClick={()=>void inspect()}>查看最新发布单</Button>{latest&&<><p>最新发布单版本：{latest.version}，状态：{latest.state}</p><ReleaseDiff order={latest}/><Button disabled={!latest.allowed_actions.includes("edit")||latest.state!=="DRAFT"} onClick={()=>{setBaseline(latest);setLatest(undefined);write.confirmRebuild();write.clearError()}}>基于最新发布单重建</Button></>}</section>}

 {(recordConflict||item.operation==="ADD")&&<section className="inline-alert"><p>{recordConflict?"配置记录基线已变化，输入保留。请核对最新配置后明确重建。":"更换新增 id 或记录基线变化时，先查看该目标的最新基线。"}</p><Button disabled={reading||write.pending||write.unresolved} onClick={()=>void inspectRecord()}>查看最新配置</Button>{recordLatest&&<><p>已读取服务器最新记录基线；此预览没有保存或执行任何变更。</p><ReleaseDiff order={{...baseline,items:recordLatest.items}}/><Button disabled={write.pending||write.unresolved} onClick={()=>{
  setItems(recordLatest.items);setRecordLatest(undefined);write.confirmRebuild();write.clearError();
 }}>基于最新配置重建</Button></>}</section>}

 {Boolean(readError)&&<ErrorState error={readError}/>}
 </Drawer>;
}
