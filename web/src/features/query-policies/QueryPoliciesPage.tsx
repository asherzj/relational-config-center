import { FileCode2, Plus, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { EmptyState, ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { presentError } from "../../api/error-messages";
import {
  allowedActions,
  formatTimestamp,
  policyStatusLabels,
  supportedQueryPolicyTypes,
  type QueryPolicy,
} from "./model";
import { QueryPolicyDrawer } from "./QueryPolicyDrawer";
import {
  useActivateQueryPolicy,
  useDeleteQueryPolicy,
  useDeprecateQueryPolicy,
  useQueryPolicies,
  useQueryPolicyTypes,
} from "./queries";

type Command = "activate" | "deprecate" | "delete";
type PendingCommand = { command: Command; code: string } | null;

const commandContent: Record<Command, { title: string; description: string; label: string; destructive?: boolean }> = {
  activate: {
    title: "激活查询规则？",
    description: "激活后执行约束将不可修改，并可以分配给新的表规则。",
    label: "确认激活",
  },
  deprecate: {
    title: "弃用查询规则？",
    description: "既有表规则仍可继续使用，但新的表规则不能再选择它。",
    label: "确认弃用",
    destructive: true,
  },
  delete: {
    title: "删除查询规则草稿？",
    description: "删除后无法恢复。只有草稿状态的查询规则可以删除。",
    label: "确认删除",
    destructive: true,
  },
};

function PolicyActions({ policy, onCommand }: { policy: QueryPolicy; onCommand: (command: Command, code: string) => void }) {
  const navigate = useNavigate();
  const actions = allowedActions[policy.status];
  const supported = supportedQueryPolicyTypes.has(policy.typeCode);
  const open = (suffix = "") => navigate(`/platform/query-policies/${encodeURIComponent(policy.code)}${suffix}`);
  return (
    <div className="row-actions">
      <button onClick={() => open()}>查看</button>
      {supported && actions.includes("replace") && <button onClick={() => open("?mode=edit")}>编辑</button>}
      {supported && actions.includes("metadata") && <button onClick={() => open("?mode=metadata")}>元数据</button>}
      {supported && actions.includes("activate") && <button onClick={() => onCommand("activate", policy.code)}>激活</button>}
      {supported && actions.includes("deprecate") && <button className="danger-link" onClick={() => onCommand("deprecate", policy.code)}>弃用</button>}
      {supported && actions.includes("delete") && <button className="danger-link" onClick={() => onCommand("delete", policy.code)}>删除</button>}
    </div>
  );
}

export function QueryPoliciesPage() {
  const navigate = useNavigate();
  const { code } = useParams<{ code?: string }>();
  const policies = useQueryPolicies();
  const types = useQueryPolicyTypes();
  const activate = useActivateQueryPolicy();
  const deprecate = useDeprecateQueryPolicy();
  const remove = useDeleteQueryPolicy();
  const { showToast } = useToast();
  const [pendingCommand, setPendingCommand] = useState<PendingCommand>(null);
  const commandMutation = pendingCommand?.command === "activate" ? activate : pendingCommand?.command === "deprecate" ? deprecate : remove;

  const executeCommand = () => {
    if (!pendingCommand) return;
    const { command, code: targetCode } = pendingCommand;
    const onSuccess = () => {
      showToast(command === "activate" ? "查询规则已激活" : command === "deprecate" ? "查询规则已弃用" : "查询规则草稿已删除");
      setPendingCommand(null);
      if (command === "delete" && code === targetCode) navigate("/platform/query-policies");
    };
    const onError = (error: unknown) => {
      const shown = presentError(error);
      showToast(shown.requestId ? `${shown.message}（请求编号：${shown.requestId}）` : shown.message);
      setPendingCommand(null);
    };
    if (command === "activate") activate.mutate(targetCode, { onSuccess, onError });
    else if (command === "deprecate") deprecate.mutate(targetCode, { onSuccess, onError });
    else remove.mutate(targetCode, { onSuccess, onError });
  };

  const supportedTypes = (types.data ?? []).filter((type) => supportedQueryPolicyTypes.has(type));
  const confirm = pendingCommand ? commandContent[pendingCommand.command] : null;

  return (
    <main className="workspace">
      <div className="page-heading">
        <div>
          <h1>查询规则定义</h1>
          <p>创建可复用、版本化的查询规则；草稿验证通过后才能激活并分配。</p>
        </div>
        <Button variant="primary" icon={<Plus size={17} />} onClick={() => navigate("/platform/query-policies/new")} disabled={types.isPending || types.isError || !supportedTypes.length}>
          新建草稿
        </Button>
      </div>

      <section className="type-registry" aria-label="查询规则类型注册表">
        <span><FileCode2 size={18} />已注册规则类型</span>
        {types.isPending && <small>正在读取…</small>}
        {types.isError && <small className="danger-color">类型注册表不可用</small>}
        {types.data?.map((type) => (
          <code className={supportedQueryPolicyTypes.has(type) ? "type-supported" : "type-unsupported"} key={type}>{type}</code>
        ))}
        <small className="registry-note">显式、代码所有 · 无运行时插件</small>
      </section>

      <section className="catalog" aria-label="查询规则目录">
        {policies.isPending ? <LoadingState label="正在读取查询规则目录…" /> : policies.isError ? (
          <ErrorState error={policies.error} onRetry={() => void policies.refetch()} />
        ) : !policies.data.length ? <EmptyState /> : (
          <div className="table-scroll">
            <table className="policy-table">
              <thead><tr><th>规则编码</th><th>名称</th><th>类型</th><th>默认排序</th><th>默认 / 最大页</th><th>状态</th><th>修改人</th><th>修改时间</th><th>操作</th></tr></thead>
              <tbody>
                {policies.data.map((policy) => (
                  <tr key={policy.code} className={code === policy.code ? "selected-row" : ""}>
                    <td><code>{policy.code}</code></td>
                    <td><strong>{policy.name}</strong><small>{policy.description || "暂无描述"}</small></td>
                    <td><code>{policy.typeCode}</code>{!supportedQueryPolicyTypes.has(policy.typeCode) && <small className="unsupported-copy">仅可查看</small>}</td>
                    <td><code>{policy.defaultOrderField} {policy.defaultOrderDirection}</code></td>
                    <td>{policy.defaultPageSize} / {policy.maxPageSize}</td>
                    <td><span className={`status-badge status-${policy.status.toLowerCase()}`}>{policyStatusLabels[policy.status]}</span></td>
                    <td>{policy.modifier}</td>
                    <td className="timestamp">{formatTimestamp(policy.modifiedAt)}</td>
                    <td><PolicyActions policy={policy} onCommand={(command, targetCode) => setPendingCommand({ command, code: targetCode })} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <footer className="catalog-footer">
          <span>共 {policies.data?.length ?? 0} 个查询规则</span>
          <span>规则目录 · 生命周期受保护</span>
          <Button className="catalog-refresh" variant="ghost" icon={<RefreshCw size={15} />} onClick={() => void policies.refetch()} disabled={policies.isFetching}>刷新</Button>
        </footer>
      </section>

      <QueryPolicyDrawer code={code} onRequestCommand={(command, targetCode) => setPendingCommand({ command, code: targetCode })} />
      {confirm && (
        <ConfirmDialog
          open
          title={confirm.title}
          description={confirm.description}
          confirmLabel={confirm.label}
          destructive={confirm.destructive}
          pending={commandMutation.isPending}
          onCancel={() => setPendingCommand(null)}
          onConfirm={executeCommand}
        />
      )}
    </main>
  );
}
