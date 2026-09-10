import {useState} from "react";
import {type ReleaseHeader} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {PagedReleaseDetails} from "./PagedReleaseDetails";

export function ReleaseReview({order,people}:{order:ReleaseHeader;people:Record<string,string>}){
 const [selected,setSelected]=useState<"request"|"PUBLICATION"|"ROLLBACK">();
 const publication=order.executions.some(execution=>execution.kind==="PUBLICATION");
 const rollback=order.executions.some(execution=>execution.kind==="ROLLBACK");
 const view=selected??(publication?"PUBLICATION":"request");
 return <section className="release-panel min-w-0" aria-label="发布单审阅">
  <div className="flex flex-wrap items-center justify-between gap-3 mb-4"><h2 className="text-lg font-semibold">变更内容</h2><div className="flex flex-wrap gap-2" aria-label="审阅内容切换">
   <Button variant={view==="request"?"primary":"secondary"} aria-pressed={view==="request"} onClick={()=>setSelected("request")}>申请差异</Button>
   {publication&&<Button variant={view==="PUBLICATION"?"primary":"secondary"} aria-pressed={view==="PUBLICATION"} onClick={()=>setSelected("PUBLICATION")}>原发布结果</Button>}
   {rollback&&<Button variant={view==="ROLLBACK"?"primary":"secondary"} aria-pressed={view==="ROLLBACK"} onClick={()=>setSelected("ROLLBACK")}>恢复结果</Button>}
  </div></div>
  {view==="request"&&<p className="mb-4 text-muted-foreground">以下是申请时的差异；发布时生成的字段以实际数据库结果为准。</p>}
  {view==="ROLLBACK"&&<p>以下为本单回滚执行的实际数据库恢复结果，按原申请明细位置展示。</p>}
  <PagedReleaseDetails order={order} kind={view} people={people}/>
 </section>;
}
