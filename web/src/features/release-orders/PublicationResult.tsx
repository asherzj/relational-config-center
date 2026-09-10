import {useState} from "react";
import {ReleaseItemPager,releasePageSize} from "./ReleaseItemPager";
import type { ReleaseOrder } from "../../api/release-orders";
import {ReleasePerson} from "./ReleasePerson";

type Result = NonNullable<ReleaseOrder["publication"]>;
type Field = Result["commands"][number]["final"]["fields"][number];

function fieldValue(field: Field) {
  switch (field.encoding) {
    case "sql_null": return "SQL NULL";
    case "json": return `JSON：${field.value}`;
    case "base64": return `二进制（Base64）：${field.value}`;
    default: return field.value === "" ? `空字符串（""）` : `值：${field.value}`;
  }
}

export function PublicationResult({result,people={},restoration=false}:{result:Result;people?:Record<string,string>;restoration?:boolean}) {
  const [requestedPage,setPage]=useState(0);
  const page=Math.min(requestedPage,Math.max(0,Math.ceil(result.commands.length/releasePageSize)-1));
  return (
    <section className="my-6 break-all" aria-label="发布结果">
      <h2 className="text-lg font-semibold">{restoration?"数据库恢复结果":"数据库发布结果"}</h2>
      <div className="my-2 flex items-center gap-2"><span>执行人：</span><ReleasePerson id={result.publisher_id} name={people[result.publisher_id]}/></div>
      <p>数据库执行时间：{result.executed_at}</p>
      {result.table_versions?Object.entries(result.table_versions).map(([table,version])=><p key={table}>{table} · 表发布版本 {version} · 刷新通知 {result.notifications?.[table]?.id} · 分发尚未接入</p>):<><p>表发布版本：{result.table_version}</p><p>刷新通知：{result.notification.id} · 分发尚未接入</p></>}
      <ReleaseItemPager count={result.commands.length} page={page} onPage={setPage} label="结果"/>
      {result.commands.slice(page*releasePageSize,(page+1)*releasePageSize).map((command,offset) => (
        <article key={`${command.table_name}:${command.sequence}`} className="my-4">
          <h3>明细 {page*releasePageSize+offset+1} · {command.table_name} · {command.operation} · 记录 {command.id === "" ? '""（空字符串）' : command.id}</h3>
          <p>表内变更序号：{command.sequence} · 记录并发版本：{command.record_version}</p>
          <p>Schema 摘要：{command.final.schema_digest}</p>
          <p>最终行校验和：{command.final.checksum}</p>
          {command.final.deleted ? <p>该记录已删除。</p> : (
            <dl>{command.final.fields.map(field => (
              <div key={field.name} className="border-b py-2">
                <dt>{field.name} · {field.type}</dt>
                <dd className="whitespace-pre-wrap">{fieldValue(field)}</dd>
              </div>
            ))}</dl>
          )}
        </article>
      ))}
    </section>
  );
}
