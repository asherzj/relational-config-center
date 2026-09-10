import {useState} from "react";
import {ReleaseItemPager,releasePageSize} from "./ReleaseItemPager";
import type {ReleaseField,ReleaseOrder} from "../../api/release-orders";
import {Checkbox} from "../../components/shadcn/checkbox";
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from "../../components/shadcn/table";
import {CurrentFieldDisplayStatus,CurrentFieldName,CurrentFieldValue,orderDisplayedFields,useCurrentFieldDisplayContext} from "../field-display/CurrentFieldDisplay";

export function ReleaseValue({state,value,type}:{state:ReleaseField["proposed_state"]|ReleaseField["before_state"];value:string|null;type?:string}){
 const labels={sql_null:"SQL NULL",omitted:"未提交",absent:"不存在",automatic:"发布时生成",generated:"数据库生成（发布后确认）"};
 return <span className="whitespace-pre-wrap break-all">{state==="value"?(type?.toLowerCase()==="json"?`JSON：${value}`:value===""?'空字符串（""）':<><small className="mr-2 rounded border px-1 text-muted-foreground">值</small><span>{value}</span></>):labels[state]}</span>;
}
export function ReleaseDiff({order,beforeLabel="服务器原值",proposedLabel="申请值"}:{order:Pick<ReleaseOrder,"items">;beforeLabel?:string;proposedLabel?:string}){
 const currentDisplay=useCurrentFieldDisplayContext();
 const [requestedPage,setPage]=useState(0),[onlyChanges,setOnlyChanges]=useState(true);
 const [expanded,setExpanded]=useState<Set<number>>(()=>new Set([0]));
 const page=Math.min(requestedPage,Math.max(0,Math.ceil(order.items.length/releasePageSize)-1));
 return <section aria-label="变更内容" className="min-w-0">
  <CurrentFieldDisplayStatus pending={currentDisplay.pending} error={currentDisplay.error} onRetry={()=>void currentDisplay.retry()}/>
  <div className="flex flex-wrap items-center justify-between gap-3 mb-3"><p className="text-muted-foreground">{["ADD","MODIFY","DELETE"].map(operation=>`${operation} ${order.items.filter(item=>item.operation===operation).length}`).join(" · ")}</p><label className="flex items-center gap-2"><Checkbox checked={onlyChanges} onCheckedChange={value=>setOnlyChanges(value===true)}/>仅看变更</label></div>
  <ReleaseItemPager count={order.items.length} page={page} onPage={next=>{setPage(next);setExpanded(new Set([next*releasePageSize]))}} onLocate={index=>{setPage(Math.floor(index/releasePageSize));setExpanded(new Set([index]))}}/>
  {order.items.slice(page*releasePageSize,(page+1)*releasePageSize).map((item,offset)=>{
   const index=page*releasePageSize+offset;
   const fields=orderDisplayedFields(currentDisplay.display,item.fields.filter(field=>!onlyChanges||item.operation!=="MODIFY"||!(field.proposed_state==="omitted"||field.before_state===field.proposed_state&&(field.before_state==="sql_null"||field.before_state==="value"&&field.before===field.proposed))),field=>field.name);
   return <section key={index} aria-label={`明细 ${index+1}`}><details open={expanded.has(index)} className="release-diff-item mb-3 min-w-0 rounded-lg border">
    <summary onClick={event=>{event.preventDefault();setExpanded(previous=>{const next=new Set(previous);if(next.has(index))next.delete(index);else next.add(index);return next})}} className="cursor-pointer p-4 break-all font-medium">明细 {index+1} · {item.operation} · 记录 {item.id??"发布时生成 id"}</summary>
    <div className="min-w-0 px-4 pb-4"><p className="mb-3 text-xs text-muted-foreground break-all">记录基线 {item.expected_record_version||"尚无已知 id"}</p>
     {fields.length===0?<p>没有确定发生变化的字段；取消“仅看变更”可查看完整申请。</p>:<div className="table-scroll"><Table className="min-w-[560px] table-fixed" containerProps={{tabIndex:0,role:"region","aria-label":`明细 ${index+1} 字段对比，可横向滚动`,className:"release-diff-scroll"}}><TableHeader><TableRow><TableHead className="w-1/4">字段</TableHead><TableHead>{beforeLabel}</TableHead><TableHead>{proposedLabel}</TableHead></TableRow></TableHeader><TableBody>{fields.map(field=><TableRow key={field.name}><TableHead scope="row" className="whitespace-normal break-all"><CurrentFieldName display={currentDisplay.display} name={field.name} detail={field.type}/></TableHead><TableCell className="release-diff-before align-top font-mono"><CurrentFieldValue display={currentDisplay.display} name={field.name} value={field.before_state==="value"?field.before:null}><ReleaseValue state={field.before_state} value={field.before} type={field.type}/></CurrentFieldValue></TableCell><TableCell className="release-diff-after align-top font-mono"><CurrentFieldValue display={currentDisplay.display} name={field.name} value={field.proposed_state==="value"?field.proposed:null}><ReleaseValue state={field.proposed_state} value={field.proposed} type={field.type}/></CurrentFieldValue></TableCell></TableRow>)}</TableBody></Table></div>}
    </div>
   </details></section>;
  })}
 </section>;
}
