import {useState,type ReactNode} from "react";
import {type ReleaseHeader} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {PagedReleaseDetails} from "./PagedReleaseDetails";

export function ReleaseReview({order,people,toolbarAction}:{order:ReleaseHeader;people:Record<string,string>;toolbarAction?:ReactNode}){
 const [selected,setSelected]=useState<"request"|"PUBLICATION"|"ROLLBACK">();
 const publication=order.executions.some(execution=>execution.kind==="PUBLICATION");
 const rollback=order.executions.some(execution=>execution.kind==="ROLLBACK");
 const view=selected??(publication?"PUBLICATION":"request");
 return <section className="release-panel min-w-0" aria-label="发布单审阅">
  <div className="flex flex-wrap items-center justify-between gap-3 mb-3"><h2 className="text-lg font-semibold">变更内容</h2><div className="flex flex-wrap items-center gap-3">
   {(publication||rollback)&&<div className="flex flex-wrap gap-1" role="group" aria-label="审阅内容切换">
    <Button variant="ghost" className={view==="request"?"bg-muted":""} aria-pressed={view==="request"} onClick={()=>setSelected("request")}>申请内容</Button>
    {publication&&<Button variant="ghost" className={view==="PUBLICATION"?"bg-muted":""} aria-pressed={view==="PUBLICATION"} onClick={()=>setSelected("PUBLICATION")}>实际发布结果</Button>}
    {rollback&&<Button variant="ghost" className={view==="ROLLBACK"?"bg-muted":""} aria-pressed={view==="ROLLBACK"} onClick={()=>setSelected("ROLLBACK")}>恢复结果</Button>}
   </div>}
   {toolbarAction}
  </div></div>
  <p className="mb-3 text-xs text-muted-foreground">分页与字段筛选仅用于审阅，发布操作始终作用于整单变更。</p>
  {view==="request"&&<p className="mb-4 text-muted-foreground">以下是申请时的差异；发布时生成的字段以实际数据库结果为准。</p>}
  {view==="ROLLBACK"&&<p>以下为本单回滚执行的实际数据库恢复结果，按原申请明细位置展示。</p>}
  <PagedReleaseDetails order={order} kind={view} people={people}/>
 </section>;
}
