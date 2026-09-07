import { getMutationPolicy } from "../../api/mutation-policies";
import { WriteRecovery } from "../../components/ui/WriteRecovery";
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
  supportsMutationPolicyType,
  supportedMutationPolicyTypes,
  type MutationPolicy,
} from "./model";
import {
  useActivateMutationPolicy,
  useDeleteMutationPolicy,
  useDeprecateMutationPolicy,
  useMutationPolicies,
  useMutationPolicyTypes,
} from "./queries";
import { MutationPolicyDrawer } from "./MutationPolicyDrawer";

function Capability({ allowed }: { allowed: boolean }) {
  return <span className={`capability ${allowed ? "capability-allowed" : "capability-denied"}`}>{allowed ? "允许" : "禁止"}</span>;
}

function autoFillTargets(policy: MutationPolicy) {
  return [policy.createOperatorField, policy.createTimeField, policy.modifyOperatorField, policy.modifyTimeField].filter(Boolean) as string[];
}

const commandContent: PolicyCommandCopy = {
  activate: { title: "激活变更规则？", description: "激活后授权和 Auto Fill 规则将不可修改，并可以分配给新的表规则。", label: "确认激活", success: "变更规则已激活" },
  deprecate: { title: "弃用变更规则？", description: "既有表规则仍可继续使用，但新的表规则不能再选择它。", label: "确认弃用", success: "变更规则已弃用", destructive: true },
  delete: { title: "删除变更规则草稿？", description: "删除后无法恢复。只有草稿状态的变更规则可以删除。", label: "确认删除", success: "变更规则草稿已删除", destructive: true },
};

function PolicyActions({ policy, supported, commandsBlocked, onCommand }: { policy: MutationPolicy; supported: boolean; commandsBlocked: boolean; onCommand: (command: PolicyLifecycleCommand, code: string) => void }) {
  const navigate = useNavigate();
  const actions = policyActionAvailability(policy.status, supported);
  const open = (suffix = "") => navigate(`/platform/mutation-policies/${encodeURIComponent(policy.code)}${suffix}`);
  return <div className="row-actions">
    <button onClick={() => open()}>查看</button>
    {actions.replace && <button onClick={() => open("?mode=edit")}>修改执行规则</button>}
    {actions.metadata && <button onClick={() => open("?mode=metadata")}>名称和描述</button>}
    {!commandsBlocked && actions.activate && <button onClick={() => onCommand("activate", policy.code)}>激活</button>}
    {!commandsBlocked && actions.deprecate && <button className="danger-link" onClick={() => onCommand("deprecate", policy.code)}>弃用</button>}
    {!commandsBlocked && actions.delete && <button className="danger-link" onClick={() => onCommand("delete", policy.code)}>删除</button>}
  </div>;
}

function PolicyTypeAvailability({ policy, supported }: { policy: MutationPolicy; supported: boolean }) {
  const hint = policyTypeAvailabilityHint(policy.status, supported);
  return hint ? <small className="unsupported-copy">{hint}</small> : null;
}

export function MutationPoliciesPage() {
  const navigate = useNavigate();
  const { code } = useParams<{ code?: string }>();
  const policies = useMutationPolicies();
  const types = useMutationPolicyTypes();
  const activate = useActivateMutationPolicy();
  const deprecate = useDeprecateMutationPolicy();
  const remove = useDeleteMutationPolicy();
  const uncertainCommand = [activate.error, deprecate.error, remove.error].some(isUncertainWriteError);
  const lifecycle = usePolicyLifecycleCommands({
    blocked: uncertainCommand,
    onUncertainWrite: (_error, targetCode) => { if (code === targetCode) navigate("/platform/mutation-policies"); },
    selectedCode: code,
    collectionPath: "/platform/mutation-policies",
    copy: commandContent,
    runners: {
      activate: { run: (target, onSuccess, onError) => activate.mutate(target, { onSuccess, onError }), pending: activate.isPending },
      deprecate: { run: (target, onSuccess, onError) => deprecate.mutate(target, { onSuccess, onError }), pending: deprecate.isPending },
      delete: { run: (target, onSuccess, onError) => remove.mutate(target, { onSuccess, onError }), pending: remove.isPending },
    },
  });
  const supportedTypes = (types.data ?? []).filter((type) => supportsMutationPolicyType(types.data, type.code));
  const confirm = lifecycle.confirm;

  return (
    <main className="workspace">
      <div className="page-heading">
        <div>
          <h1>变更规则定义</h1>
          <p>定义配置记录的新增、修改与删除权限，以及操作人和时间的自动填写方式。</p>
        </div>
        <Button variant="primary" icon={<Plus size={17} />} onClick={() => navigate("/platform/mutation-policies/new")} disabled={uncertainCommand || types.isPending || types.isError || !supportedTypes.length}>新建草稿</Button>
      </div>

      <section className="type-registry" aria-label="变更规则类型注册表">
        <span><FileCode2 size={18} />已注册规则类型</span>
        {types.isPending && <small>正在读取…</small>}
        {types.isError && <small className="danger-color">类型注册表不可用</small>}
        {types.data?.map((type) => (
          <span className="mutation-type" key={type.code}>
            <code className={supportedMutationPolicyTypes.has(type.code) ? "type-supported" : "type-unsupported"}>{type.code}</code>
            <small>{type.operations.join(" · ")}</small>
          </span>
        ))}
      </section>

      <section className="catalog" aria-label="变更规则目录">
        {policies.isPending ? <LoadingState label="正在读取变更规则目录…" /> : policies.isError && !policies.data ? (
          <ErrorState error={policies.error} onRetry={() => void policies.refetch()} />
        ) : !policies.data.length ? <EmptyState entity="变更规则" /> : (
          <div className="table-scroll">
            <table className="policy-table mutation-policy-table">
              <thead><tr><th>变更规则</th><th>新增</th><th>修改</th><th>删除</th><th>自动填写字段</th><th>状态</th><th>最近更新</th><th>操作</th></tr></thead>
              <tbody>{policies.data.map((policy) => {
                const supported = supportsMutationPolicyType(types.data, policy.typeCode);
                return <tr key={policy.code} className={code === policy.code ? "selected-row" : ""}>
                  <td className="rule-identity"><strong title={policy.description}>{policy.name}</strong><code>{policy.code}</code><small>{policy.typeCode}</small><PolicyTypeAvailability policy={policy} supported={supported} /></td>
                  <td><Capability allowed={policy.allowAdd} /></td>
                  <td><Capability allowed={policy.allowModify} /></td>
                  <td><Capability allowed={policy.allowDelete} /></td>
                  <td><div className="auto-fill-targets">{autoFillTargets(policy).length ? autoFillTargets(policy).map((target) => <code key={target}>{target}</code>) : <span>无</span>}</div></td>
                  <td><span className={`status-badge status-${policy.status.toLowerCase()}`}>{policyStatusLabels[policy.status]}</span></td>
                  <td className="timestamp">{formatTimestamp(policy.modifiedAt)}</td>
                  <td><PolicyActions policy={policy} supported={supported} commandsBlocked={uncertainCommand} onCommand={lifecycle.request} /></td>
                </tr>;
              })}</tbody>
            </table>
          </div>
        )}
        <footer className="catalog-footer">
          <span>共 {policies.data?.length ?? 0} 个变更规则</span>
          <span>固定字段 · 生命周期受保护</span>
          <Button className="catalog-refresh" variant="ghost" icon={<RefreshCw size={15} />} onClick={() => void policies.refetch()} disabled={policies.isFetching}>刷新</Button>
        </footer>
      </section>
      <MutationPolicyDrawer code={code} commandsBlocked={uncertainCommand} onRequestCommand={lifecycle.request} />
      {!confirm && <WriteRecovery onResume={() => {
        activate.reset(); deprecate.reset(); remove.reset();
        lifecycle.finishCheck();
        void policies.refetch();
      }} resumeLabel="我已核对，结束本次核对" error={lifecycle.recovery.error} onCheck={() => getMutationPolicy(lifecycle.targetCode!)} />}
      {confirm && <ConfirmDialog open title={confirm.title} description={confirm.description} confirmLabel={confirm.label} destructive={confirm.destructive} pending={lifecycle.pending} confirmDisabled={uncertainCommand || lifecycle.recovery.blocked.current} children={<WriteRecovery onResume={() => {
        activate.reset(); deprecate.reset(); remove.reset();
        lifecycle.finishCheck();
        void policies.refetch();
      }} resumeLabel="我已核对，结束本次核对" error={lifecycle.recovery.error} onCheck={() => getMutationPolicy(lifecycle.targetCode!)} />} onCancel={lifecycle.cancel} onConfirm={lifecycle.execute} />}
    </main>
  );
}
