import {useState} from "react";
import {type ReleaseOrder} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {ReleaseDiff} from "./ReleaseDiff";
import {PublicationResult} from "./PublicationResult";

export function ReleaseReview({order,people}:{order:ReleaseOrder;people:Record<string,string>}){
 const [selected,setSelected]=useState<"request"|"publication"|"restoration">();
 const view=selected??(order.publication?"publication":"request");
 return <section className="release-panel min-w-0" aria-label="发布单审阅">
  <div className="flex flex-wrap items-center justify-between gap-3 mb-4"><h2 className="text-lg font-semibold">变更内容</h2><div className="flex flex-wrap gap-2" aria-label="审阅内容切换">
   <Button variant={view==="request"?"primary":"secondary"} aria-pressed={view==="request"} onClick={()=>setSelected("request")}>申请差异</Button>
   {order.publication&&<Button variant={view==="publication"?"primary":"secondary"} aria-pressed={view==="publication"} onClick={()=>setSelected("publication")}>{order.rollback_of_id?"恢复结果":"原发布结果"}</Button>}
   {order.state==="ROLLED_BACK"&&order.rollback&&<Button variant={view==="restoration"?"primary":"secondary"} aria-pressed={view==="restoration"} onClick={()=>setSelected("restoration")}>恢复结果</Button>}
  </div></div>
  {view==="request"?<><p className="mb-4 text-muted-foreground">以下是申请时的差异；发布时生成的字段以实际数据库结果为准。</p><ReleaseDiff order={order}/></>:view==="publication"&&order.publication?<PublicationResult result={order.publication} people={people}/>:view==="restoration"&&order.rollback?<><p>以下为本单回滚执行的实际数据库恢复结果。</p><PublicationResult result={order.rollback} people={people} restoration/></>:<p role="alert">尚无实际数据库发布结果。</p>}
 </section>;
}
