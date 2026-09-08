import {useState} from "react";
import {ReleaseItemPager,releasePageSize} from "./ReleaseItemPager";
import type {ReleaseField,ReleaseOrder} from "../../api/release-orders";
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from "../../components/shadcn/table";
export function ReleaseValue({state,value}:{state:ReleaseField["proposed_state"]|ReleaseField["before_state"];value:string|null}){
 const labels={sql_null:"SQL NULL",omitted:"未提交",absent:"不存在",automatic:"发布时生成",generated:"数据库生成（发布后确认）"};
 return <span className="whitespace-pre-wrap break-all">{state==="value"?(value===""?"空字符串（\"\"）":<><small className="mr-2 rounded border px-1 text-muted-foreground">值</small><span>{value}</span></>):labels[state]}</span>;
}
export function ReleaseDiff({order,beforeLabel="服务器原值",proposedLabel="申请值"}:{order:Pick<ReleaseOrder,"items">;beforeLabel?:string;proposedLabel?:string}){
 const [requestedPage,setPage]=useState(0);
 const page=Math.min(requestedPage,Math.max(0,Math.ceil(order.items.length/releasePageSize)-1));
 return <><ReleaseItemPager count={order.items.length} page={page} onPage={setPage}/>{order.items.slice(page*releasePageSize,(page+1)*releasePageSize).map((item,offset)=>{const index=page*releasePageSize+offset;return <section key={index} aria-label={`明细 ${index+1}`} className="mb-6">
  <p className="mb-3">明细 {index+1} · {item.operation} · 记录 {item.id??"发布时生成 id"} · 记录基线 {item.expected_record_version||"尚无已知 id"}</p>
  <div className="table-scroll"><Table><TableHeader><TableRow><TableHead>字段</TableHead><TableHead>{beforeLabel}</TableHead><TableHead>{proposedLabel}</TableHead></TableRow></TableHeader><TableBody>{item.fields.map(field=><TableRow key={field.name}><TableHead>{field.name}<small className="block">{field.type}</small></TableHead><TableCell><ReleaseValue state={field.before_state} value={field.before}/></TableCell><TableCell><ReleaseValue state={field.proposed_state} value={field.proposed}/></TableCell></TableRow>)}</TableBody></Table></div>
 </section>})}</>;
}
