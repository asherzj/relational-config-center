import { useEffect, useRef, useState } from "react";
import { ApiError, isUncertainWriteError } from "../../api/client";
import { Button } from "./Button";
import { ErrorState } from "./Feedback";

export function useWriteRecovery() {
  const blocked = useRef(false);
  const [error, setError] = useState<unknown>(null);
  const fail = (error: unknown) => {
    if (isUncertainWriteError(error)) { blocked.current = true; setError(error); }
  };
  const reset = () => { blocked.current = false; setError(null); };
  return { blocked, error, fail, reset };
}

const labels: Record<string, string> = {
  code: "规则编码", name: "显示名称", description: "描述", status: "状态", typeCode: "规则类型",
  tableName: "真实数据库表", queryPolicyCode: "查询规则编码", mutationPolicyCode: "变更规则编码", enabled: "已启用",
  allowAdd: "允许 ADD", allowModify: "允许 MODIFY", allowDelete: "允许 DELETE",
  defaultOrderField: "默认排序字段", defaultOrderDirection: "默认排序方向", defaultPageSize: "默认每页条数", maxPageSize: "最大每页条数",
  createOperatorField: "新增操作人字段", createTimeField: "新增时间字段", modifyOperatorField: "修改操作人字段", modifyTimeField: "修改时间字段",
  creator: "创建人", modifier: "修改人", createdAt: "创建时间", modifiedAt: "修改时间",
};
function Value({ value }: { value: unknown }) {
  if (value === null) return <span className="cell-state cell-null">NULL</span>;
  if (value === "") return <span className="cell-state cell-empty">空字符串</span>;
  if (typeof value === "boolean") return <span>{value ? "是" : "否"}</span>;
  return <span style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{String(value)}</span>;
}
function Snapshot({ value }: { value: unknown }) {
  if (!value || typeof value !== "object") return <Value value={value} />;
  const snapshot = value as Record<string, unknown>;
  if (Array.isArray(snapshot.rows)) {
    const rows = snapshot.rows as Record<string, unknown>[];
    const columns = (snapshot.columns as { name: string }[]).map(column => column.name);
    return <>{rows.length === 0 ? <p>本次查询没有返回记录。</p> : <table className="change-set-table"><thead><tr>{columns.map(column => <th key={column}>{column}</th>)}</tr></thead><tbody>{rows.map((row, index) => <tr key={index}>{columns.map(column => <td key={column}><Value value={row[column]} /></td>)}</tr>)}</tbody></table>}</>;
  }
  return <dl>{Object.entries(snapshot).map(([field, value]) => <div key={field}><dt>{labels[field] ?? field}</dt><dd><Value value={value} /></dd></div>)}</dl>;
}

export function WriteRecovery({ error, onCheck, onResume, resumeLabel = "我已核对，返回修改", children }: {
  error: unknown;
  onCheck: () => Promise<unknown>;
  onResume: () => void;
  resumeLabel?: string;
  children?: React.ReactNode;
}) {
  const checking = useRef(false);
  const currentError = useRef(error);
  currentError.current = error;
  const [pending, setPending] = useState(false);
  const [checkedError, setCheckedError] = useState<unknown>(null);
  const checked = checkedError === error && isUncertainWriteError(error);
  const [snapshot, setSnapshot] = useState<unknown>(undefined);
  const [checkError, setCheckError] = useState<unknown>(null);
  useEffect(() => { setSnapshot(undefined); setCheckError(null); setCheckedError(null); }, [error]);
  if (!isUncertainWriteError(error)) return null;
  const check = async () => {
    if (checking.current) return;
    checking.current = true; setPending(true); setCheckError(null); setSnapshot(undefined); setCheckedError(null);
    try { const value = await onCheck(); if (currentError.current !== error) return; setSnapshot(value); setCheckedError(error); }
    catch (cause) {
      if (currentError.current !== error) return;
      setCheckError(cause);
      // A recognized missing resource is a usable current observation, never proof
      // that the earlier command failed. Other failed reads do not complete a check.
      setCheckedError(cause instanceof ApiError && cause.status === 404 && ["query_policy_not_found", "mutation_policy_not_found", "table_policy_not_found"].includes(cause.code) ? error : null);
    }
    finally { checking.current = false; setPending(false); }
  };
  return <section className="write-recovery" role="alert" aria-label="提交结果尚未确认">
    <strong>提交结果尚未确认</strong>
    <p>请求可能已经写入。为避免重复提交，本次提交已锁定；输入和预览仍保留。核对前请勿重新发起相同操作。</p>
    {error instanceof ApiError && error.requestId && <p>请求编号：{error.requestId}</p>}
    {children}
    <Button disabled={pending} onClick={() => void check()}>{pending ? "正在只读核对…" : "只读核对当前状态"}</Button>
    <p>查询只展示当前状态；值相同、记录不存在或分页未找到，都不能证明这次提交成功或失败。核对不会重新写入或自动解锁本次提交。</p>
    {checkError !== null && <ErrorState error={checkError} />}
    {checked && snapshot !== undefined && <div className="write-recovery-snapshot"><strong>当前查询结果（仅供核对）</strong><Snapshot value={snapshot} /></div>}
    {checked && <><p>请根据核对结果自行判断后续操作；重新提交仍可能造成重复写入。</p><Button disabled={pending} onClick={onResume}>{resumeLabel}</Button></>}
  </section>;
}
