import {useState} from "react";
import {Link} from "react-router-dom";
import type {ReleaseHeader} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {ReleaseTime} from "./ReleaseTime";
import {ReleasePerson} from "./ReleasePerson";

const actionLabels:Record<string,string>={CREATE:"创建了草稿",EDIT:"修改了草稿",UPDATE:"修改了草稿",SUBMIT:"提交了审批",APPROVE:"通过了审批表",REJECT:"拒绝了发布单",CANCEL:"取消了发布单",COPY:"复制了新草稿",REPREPARE:"重新准备了发布单",EXECUTE:"发布到数据库",EXECUTE_FAILED:"发布未提交",QUICK_ROLLBACK_FAILED:"快速回滚未提交",COMPLETE:"完结了发布单",ROLLBACK_REQUEST:"申请了回滚",QUICK_ROLLBACK:"执行了快速回滚",ROLLBACK_REASON:"修改了回滚原因",ROLLED_BACK:"完成了回滚",ROLLBACK_CANCELLED:"取消了回滚申请",ROLLBACK_REJECTED:"回滚申请被拒绝"};
export function ReleaseHistory({order,people}:{order:ReleaseHeader;people:Record<string,string>}){
 const [all,setAll]=useState(false);
 const history=[...order.history].reverse();
 return <section className="release-panel" aria-label="操作历史"><div className="flex flex-wrap justify-between items-center gap-3 mb-4"><h2 className="text-lg font-semibold">操作历史</h2>{history.length>5&&<Button variant="ghost" onClick={()=>setAll(!all)}>{all?"收起为最近 5 条":`查看全部 ${history.length} 条记录`}</Button>}</div><ol className="grid gap-4">{history.slice(0,all?undefined:5).map((event,index)=><li key={`${event.at}:${event.action}:${index}`} className="border-b pb-4 last:border-0 last:pb-0 break-all"><div className="flex flex-wrap items-center gap-x-6 gap-y-2"><div><p className="font-medium">{event.action==="SUBMIT"&&order.release_type==="EMERGENCY"?"提交了应急发布":actionLabels[event.action]??event.action}</p><p className="text-xs text-muted-foreground">{event.action} · 版本 {event.version}</p></div><span className="text-xs text-muted-foreground"><ReleaseTime value={event.at}/></span></div><div className="my-2"><ReleasePerson id={event.actor_id} name={people[event.actor_id]}/></div>{event.table_names&&event.table_names.length>0&&<p className="mt-2">处理表范围：{event.table_names.join("、")}</p>}{event.approval_sources&&<ul className="my-2 text-xs text-muted-foreground">{event.approval_sources.map(source=><li key={source.table_name}>{source.table_name} · {source.source==="ADMIN"?"默认 ADMIN":source.roles.map(role=>`${role.name}（${role.id}）`).join("、")}</li>)}</ul>}{event.reason&&<p className="whitespace-pre-wrap">{event.reason}</p>}{event.related_order_id&&<p className="mt-2">关联发布单：<Link className="underline underline-offset-4 font-mono" to={`/configuration/release-orders/${event.related_order_id}`}>{event.related_order_id}</Link></p>}</li>)}</ol></section>;
}
