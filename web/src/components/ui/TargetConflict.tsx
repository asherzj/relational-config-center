import {useQuery} from "@tanstack/react-query";
import {releaseOrders} from "../../api/release-orders";

export function TargetConflict({tableName,orderID,applicantID}:{tableName:string;orderID:string;applicantID:string}) {
 const people=useQuery({queryKey:["release-order-people",orderID],queryFn:()=>releaseOrders.people(orderID),retry:false});
 return <div className="grid gap-2 break-all"><span>冲突表：{tableName}</span><a className="underline" href={`/configuration/release-orders/${encodeURIComponent(orderID)}`} target="_blank" rel="noreferrer">占用发布单：{orderID}（新窗口查看）</a><span>申请人：{people.data?.people[applicantID]||applicantID}</span>{people.isError&&<span>姓名读取失败，已显示永久账号 ID。</span>}</div>;
}
