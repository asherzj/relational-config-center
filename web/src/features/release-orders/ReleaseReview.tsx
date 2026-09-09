import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {releaseOrders,type ReleaseOrder} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {ReleaseDiff} from "./ReleaseDiff";
import {PublicationResult} from "./PublicationResult";

export function ReleaseReview({order,people}:{order:ReleaseOrder;people:Record<string,string>}){
 const [selected,setSelected]=useState<"request"|"publication"|"restoration">();
 const view=selected??(order.publication?"publication":"request");
 const restoration=useQuery({queryKey:["release-order",order.rollback_order_id],queryFn:()=>releaseOrders.get(order.rollback_order_id!),enabled:view==="restoration"&&order.state==="ROLLED_BACK"&&Boolean(order.rollback_order_id),retry:false});
 return <section className="release-panel min-w-0" aria-label="发布单审阅">
  <div className="flex flex-wrap items-center justify-between gap-3 mb-4"><h2 className="text-lg font-semibold">变更内容</h2><div className="flex flex-wrap gap-2" aria-label="审阅内容切换">
   <Button variant={view==="request"?"primary":"secondary"} aria-pressed={view==="request"} onClick={()=>setSelected("request")}>申请差异</Button>
   {order.publication&&<Button variant={view==="publication"?"primary":"secondary"} aria-pressed={view==="publication"} onClick={()=>setSelected("publication")}>{order.rollback_of_id?"恢复结果":"原发布结果"}</Button>}
   {order.state==="ROLLED_BACK"&&order.rollback_order_id&&<Button variant={view==="restoration"?"primary":"secondary"} aria-pressed={view==="restoration"} onClick={()=>setSelected("restoration")}>恢复结果</Button>}
  </div></div>
  {view==="request"?<><p className="mb-4 text-muted-foreground">以下是申请时的差异；发布时生成的字段以实际数据库结果为准。</p><ReleaseDiff order={order}/></>:view==="publication"&&order.publication?<PublicationResult result={order.publication} people={people}/>:view==="restoration"?(restoration.isPending?<LoadingState label="正在读取实际恢复结果…"/>:restoration.isError?<ErrorState error={restoration.error} onRetry={()=>void restoration.refetch()}/>:restoration.data.publication&&restoration.data.rollback_of_id===order.id?<><p>以下为关联回滚发布单的实际数据库恢复结果。</p><PublicationResult result={restoration.data.publication} people={people}/></>:<p role="alert">关联单尚无可核对的实际恢复结果，请重新读取发布单。</p>):<p role="alert">尚无实际数据库发布结果。</p>}
 </section>;
}
