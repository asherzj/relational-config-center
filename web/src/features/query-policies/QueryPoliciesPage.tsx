import { getQueryPolicy } from "../../api/query-policies";
import { WriteRecovery } from "../../components/ui/WriteRecovery";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";
import { Badge } from "../../components/shadcn/badge";
import { FileCode2, Plus, RefreshCw } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";
import { isUncertainWriteError } from "../../api/client";
import { Button } from "../../components/ui/Button";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { EmptyState, ErrorState, LoadingState } from "../../components/ui/Feedback";
import {
  policyActionAvailability,
  policyTypeAvailabilityHint,
  usePolicyLifecycleCommands,
  type PolicyCommandCopy,
  type PolicyLifecycleCommand,
} from "../policies/lifecycle";
import {
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

const commandContent: PolicyCommandCopy = {
  activate: {
    title: "激活查询规则？",
    description: "激活后执行约束将不可修改，并可以分配给新的表规则。",
    label: "确认激活",
    success: "查询规则已激活",
  },
  deprecate: {
    title: "弃用查询规则？",
    description: "既有表规则仍可继续使用，但新的表规则不能再选择它。",
    label: "确认弃用",
    success: "查询规则已弃用",
    destructive: true,
  },
  delete: {
    title: "删除查询规则草稿？",
    description: "删除后无法恢复。只有草稿状态的查询规则可以删除。",
    label: "确认删除",
    success: "查询规则草稿已删除",
    destructive: true,
  },
};

function PolicyActions({ policy, supported, commandsBlocked, onCommand }: { policy: QueryPolicy; supported: boolean; commandsBlocked: boolean; onCommand: (command: PolicyLifecycleCommand, code: string) => void }) {
  const navigate = useNavigate();
  const actions = policyActionAvailability(policy.status, supported);
  const open = (suffix = "") => navigate(`/platform/query-policies/${encodeURIComponent(policy.code)}${suffix}`);
  return (
    <div className="row-actions">
      <Button variant="ghost" onClick={() => open()}>查看</Button>
      {actions.replace && <Button variant="ghost" onClick={() => open("?mode=edit")}>修改执行规则</Button>}
      {actions.metadata && <Button variant="ghost" onClick={() => open("?mode=metadata")}>名称和描述</Button>}
      {!commandsBlocked && actions.activate && <Button variant="ghost" onClick={() => onCommand("activate", policy.code)}>激活</Button>}
      {!commandsBlocked && actions.deprecate && <Button variant="ghost" className="danger-link" onClick={() => onCommand("deprecate", policy.code)}>弃用</Button>}
      {!commandsBlocked && actions.delete && <Button variant="ghost" className="danger-link" onClick={() => onCommand("delete", policy.code)}>删除</Button>}
    </div>
  );
}

function PolicyTypeAvailability({ policy, supported }: { policy: QueryPolicy; supported: boolean }) {
  const hint = policyTypeAvailabilityHint(policy.status, supported);
  return hint ? <small className="unsupported-copy">{hint}</small> : null;
}

export function QueryPoliciesPage() {
  const navigate = useNavigate();
  const { code } = useParams<{ code?: string }>();
  const policies = useQueryPolicies();
  const types = useQueryPolicyTypes();
  const activate = useActivateQueryPolicy();
  const deprecate = useDeprecateQueryPolicy();
  const remove = useDeleteQueryPolicy();
  const uncertainCommand = [activate.error, deprecate.error, remove.error].some(isUncertainWriteError);
  const lifecycle = usePolicyLifecycleCommands({
    blocked: uncertainCommand,
    onUncertainWrite: (_error, targetCode) => {
      if (code === targetCode) navigate("/platform/query-policies");
    },
    selectedCode: code,
    collectionPath: "/platform/query-policies",
    copy: commandContent,
    runners: {
      activate: { run: (target, onSuccess, onError) => activate.mutate(target, { onSuccess, onError }), pending: activate.isPending },
      deprecate: { run: (target, onSuccess, onError) => deprecate.mutate(target, { onSuccess, onError }), pending: deprecate.isPending },
      delete: { run: (target, onSuccess, onError) => remove.mutate(target, { onSuccess, onError }), pending: remove.isPending },
    },
  });

  const supportedTypes = (types.data ?? []).filter((type) => supportedQueryPolicyTypes.has(type));
  const confirm = lifecycle.confirm;

  return (
    <main className="workspace">
      <div className="page-heading">
        <div>
          <h1>查询规则定义</h1>
          <p>统一配置表的排序与分页方式。规则从草稿开始，激活后即可分配使用。</p>
        </div>
        <Button variant="primary" icon={<Plus size={17} />} onClick={() => navigate("/platform/query-policies/new")} disabled={uncertainCommand || types.isPending || types.isError || !supportedTypes.length}>
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
      </section>

      <section className="catalog" aria-label="查询规则目录">
        {policies.isPending ? <LoadingState label="正在读取查询规则目录…" /> : policies.isError && !policies.data ? (
          <ErrorState error={policies.error} onRetry={() => void policies.refetch()} />
        ) : !policies.data.length ? <EmptyState /> : (
          <div className="table-scroll">
            <Table className="policy-table">
              <TableHeader><TableRow><TableHead>查询规则</TableHead><TableHead>默认排序</TableHead><TableHead>每页条数</TableHead><TableHead>状态</TableHead><TableHead>最近更新</TableHead><TableHead>操作</TableHead></TableRow></TableHeader>
              <TableBody>
                {policies.data.map((policy) => {
                  const supported = supportedQueryPolicyTypes.has(policy.typeCode) && Boolean(types.data?.includes(policy.typeCode));
                  return <TableRow key={policy.code} className={code === policy.code ? "selected-row" : ""}>
                    <TableCell className="rule-identity"><strong title={policy.description}>{policy.name}</strong><code>{policy.code}</code><small>{policy.typeCode}</small><PolicyTypeAvailability policy={policy} supported={supported} /></TableCell>
                    <TableCell><code>{policy.defaultOrderField} {policy.defaultOrderDirection}</code></TableCell>
                    <TableCell>{policy.defaultPageSize}<small>最多 {policy.maxPageSize} 条</small></TableCell>
                    <TableCell><Badge variant="outline" className={`status-badge status-${policy.status.toLowerCase()}`}>{policyStatusLabels[policy.status]}</Badge></TableCell>
                    <TableCell className="timestamp">{formatTimestamp(policy.modifiedAt)}<small>{policy.modifier}</small></TableCell>
                    <TableCell><PolicyActions policy={policy} supported={supported} commandsBlocked={uncertainCommand} onCommand={lifecycle.request} /></TableCell>
                  </TableRow>;
                })}
              </TableBody>
            </Table>
          </div>
        )}
        <footer className="catalog-footer">
          <span>共 {policies.data?.length ?? 0} 个查询规则</span>
          <span>规则目录 · 生命周期受保护</span>
          <Button className="catalog-refresh" variant="ghost" icon={<RefreshCw size={15} />} onClick={() => void policies.refetch()} disabled={policies.isFetching}>刷新</Button>
        </footer>
      </section>

      <QueryPolicyDrawer code={code} commandsBlocked={uncertainCommand} onRequestCommand={lifecycle.request} />
      {!confirm && <WriteRecovery onResume={() => {
        activate.reset(); deprecate.reset(); remove.reset();
        lifecycle.finishCheck();
        void policies.refetch();
      }} resumeLabel="我已核对，结束本次核对" error={lifecycle.recovery.error} onCheck={() => getQueryPolicy(lifecycle.targetCode!)} />}
      {confirm && (
        <ConfirmDialog
          open
          title={confirm.title}
          description={confirm.description}
          confirmLabel={confirm.label}
          destructive={confirm.destructive}
          pending={lifecycle.pending}
          confirmDisabled={uncertainCommand || lifecycle.recovery.blocked.current}
          children={<WriteRecovery onResume={() => { lifecycle.finishCheck(); void policies.refetch(); }} resumeLabel="我已核对，结束本次核对" error={lifecycle.recovery.error} onCheck={() => getQueryPolicy(lifecycle.targetCode!)} />}
          onCancel={lifecycle.cancel}
          onConfirm={lifecycle.execute}
        />
      )}
    </main>
  );
}
