import { DatabaseZap, Plus, RefreshCw, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { EmptyState, ErrorState, LoadingState } from "../../components/ui/Feedback";
import { formatTimestamp, incompatibilityReasonLabels } from "./model";
import { useDatabaseTables, useTablePolicies } from "./queries";
import { TablePolicyDrawer } from "./TablePolicyDrawer";

export function TablePoliciesPage() {
  const navigate = useNavigate();
  const { tableName } = useParams<{ tableName?: string }>();
  const discovery = useDatabaseTables();
  const policies = useTablePolicies();
  const [filter, setFilter] = useState("");
  const normalizedFilter = filter.trim().toLocaleLowerCase();
  const filteredPolicies = useMemo(() => (policies.data ?? []).filter((policy) =>
    !normalizedFilter || [policy.tableName, policy.queryPolicyCode, policy.mutationPolicyCode, policy.modifier]
      .some((value) => value.toLocaleLowerCase().includes(normalizedFilter)),
  ), [normalizedFilter, policies.data]);

  return (
    <main className="workspace table-policies-workspace">
      <div className="page-heading">
        <div><h1>表规则分配</h1><p>从真实 Managed Data Source 发现表，并分配可复用的 Active 查询规则与变更规则。</p></div>
        <Button variant="primary" icon={<Plus size={17} />} onClick={() => navigate("/platform/table-policies?mode=create")}>新建分配</Button>
      </div>

      <section className="discovery-panel" aria-label="Database Table Discovery">
        <header><span><DatabaseZap size={18} />Database Table Discovery</span><small>发现不代表授权</small></header>
        {discovery.isPending ? <LoadingState label="正在读取真实数据库表…" /> : discovery.isError ? <ErrorState error={discovery.error} onRetry={() => void discovery.refetch()} /> : !discovery.data.length ? (
          <div className="feedback-state"><strong>没有可发现的数据库表</strong><span>Managed Data Source 当前没有可用于规则分配的真实表。</span></div>
        ) : (
          <div className="discovery-grid">
            {discovery.data.map((table) => (
              <article key={table.tableName} className={!table.compatible ? "discovery-incompatible" : ""}>
                <strong><code>{table.tableName}</code></strong>
                <span>{table.tableComment || "暂无表注释"}</span>
                <div>
                  <span className={`status-badge ${table.policyEnabled ? "status-active" : "status-draft"}`}>{table.policyExists ? table.policyEnabled ? "已启用" : "已分配 · 未启用" : "未分配"}</span>
                  <span className={`compatibility ${table.compatible ? "compatibility-ok" : "compatibility-bad"}`}>{table.compatible ? "结构兼容" : table.incompatibilityReason ? incompatibilityReasonLabels[table.incompatibilityReason] : "结构不兼容"}</span>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section className="catalog" aria-label="表规则目录">
        <div className="catalog-toolbar">
          <label className="search-field"><Search size={16} /><span className="sr-only">筛选表规则</span><input type="search" aria-label="筛选表规则" placeholder="筛选表名、规则编码或修改人" value={filter} onChange={(event) => setFilter(event.target.value)} /></label>
        </div>
        {policies.isPending ? <LoadingState label="正在读取表规则目录…" /> : policies.isError ? <ErrorState error={policies.error} onRetry={() => void policies.refetch()} /> : !policies.data.length ? <EmptyState entity="表规则" /> : !filteredPolicies.length ? <div className="feedback-state"><strong>没有匹配的表规则</strong><span>尝试调整筛选条件。</span></div> : (
          <div className="table-scroll"><table className="policy-table table-policy-table"><thead><tr><th>表名</th><th>查询规则编码</th><th>变更规则编码</th><th>状态</th><th>创建人</th><th>修改人</th><th>修改时间</th><th>操作</th></tr></thead><tbody>
            {filteredPolicies.map((policy) => <tr key={policy.tableName} className={tableName === policy.tableName ? "selected-row" : ""}>
              <td><code>{policy.tableName}</code></td><td><code>{policy.queryPolicyCode}</code></td><td><code>{policy.mutationPolicyCode}</code></td>
              <td><span className={`status-badge ${policy.enabled ? "status-active" : "status-draft"}`}>{policy.enabled ? "已启用" : "未启用"}</span></td>
              <td>{policy.creator}</td><td>{policy.modifier}</td><td className="timestamp">{formatTimestamp(policy.modifiedAt)}</td>
              <td><div className="row-actions"><button onClick={() => navigate(`/platform/table-policies/${encodeURIComponent(policy.tableName)}`)}>查看</button></div></td>
            </tr>)}
          </tbody></table></div>
        )}
        <footer className="catalog-footer"><span>共 {policies.data?.length ?? 0} 个表规则</span><span>完整目录 · 客户端筛选</span><Button className="catalog-refresh" variant="ghost" icon={<RefreshCw size={15} />} onClick={() => { void policies.refetch(); void discovery.refetch(); }}>刷新</Button></footer>
      </section>
      <TablePolicyDrawer tableName={tableName} />
    </main>
  );
}
