import { AlertCircle, Check, Minus } from "lucide-react";
import type { MutationPolicy, MutationPolicyDraft, MutationPolicyType } from "../mutation-policies/model";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import type { QueryPolicy, QueryPolicyDraft } from "../query-policies/model";
import { supportedQueryPolicyTypes } from "../query-policies/model";

export type RegistryState = "loading" | "error" | "ready";

type QueryEffectProps = {
  policy: QueryPolicy | QueryPolicyDraft;
  registeredTypes: readonly string[] | undefined;
  registryState: RegistryState;
  heading?: string;
};

function UnconfirmedEffect({ reason }: { reason: string }) {
  return (
    <section className="policy-effect policy-effect-unconfirmed" aria-label="规则效果无法确认">
      <header><AlertCircle size={17} /><strong>无法确认执行能力</strong></header>
      <p>{reason} 当前界面不会猜测它允许哪些操作。</p>
    </section>
  );
}

export function QueryPolicyEffect({ policy, registeredTypes, registryState, heading = "实际查询效果" }: QueryEffectProps) {
  if (registryState === "loading") return <UnconfirmedEffect reason="正在读取规则类型目录。" />;
  if (registryState === "error") return <UnconfirmedEffect reason="规则类型目录加载失败。" />;
  if (!supportedQueryPolicyTypes.has(policy.typeCode)) return <UnconfirmedEffect reason={`当前界面不认识规则类型 ${policy.typeCode}。`} />;
  if (!registeredTypes?.includes(policy.typeCode)) return <UnconfirmedEffect reason={`规则类型目录没有确认 ${policy.typeCode}。`} />;

  const direction = policy.defaultOrderDirection === "ASC" ? "升序" : "降序";
  const shownHeading = "status" in policy && policy.status === "DRAFT" ? "执行效果预览" : heading;
  return (
    <section className="policy-effect" aria-label={shownHeading}>
      <header><Check size={17} /><strong>{shownHeading}</strong></header>
      <p>请求未指定排序时，按 <code>{policy.defaultOrderField}</code> {direction}排列；默认每页数量为 {policy.defaultPageSize}，请求的每页数量不能超过 {policy.maxPageSize}。</p>
      <p>可查询和排序的列以请求时读取到的实时表结构及现有值转换能力为准。这条规则没有配置字段白名单。</p>
    </section>
  );
}

type MutationEffectProps = {
  policy: MutationPolicy | MutationPolicyDraft;
  registeredTypes: readonly MutationPolicyType[] | undefined;
  registryState: RegistryState;
  heading?: string;
};

function targets(parts: Array<[string | null, string]>) {
  const configured = parts.filter((part): part is [string, string] => Boolean(part[0]));
  if (!configured.length) return "不自动填写字段";
  return `服务器自动填写 ${configured.map(([field, source]) => `${field}（${source}）`).join("、")}`;
}

export function MutationPolicyEffect({ policy, registeredTypes, registryState, heading = "实际变更效果" }: MutationEffectProps) {
  if (registryState === "loading") return <UnconfirmedEffect reason="正在读取规则类型目录。" />;
  if (registryState === "error") return <UnconfirmedEffect reason="规则类型目录加载失败。" />;
  if (!supportsMutationPolicyType(registeredTypes, policy.typeCode)) {
    return <UnconfirmedEffect reason={`规则类型目录没有确认 ${policy.typeCode} 的完整新增、修改和删除能力。`} />;
  }

  const operations = [
    { name: "新增", allowed: policy.allowAdd, detail: targets([[policy.createOperatorField, "部署配置中的 Operator"], [policy.createTimeField, "数据库时间"], [policy.modifyOperatorField, "部署配置中的 Operator"], [policy.modifyTimeField, "数据库时间"]]) },
    { name: "修改", allowed: policy.allowModify, detail: targets([[policy.modifyOperatorField, "部署配置中的 Operator"], [policy.modifyTimeField, "数据库时间"]]) },
    { name: "删除", allowed: policy.allowDelete, detail: "不自动填写字段" },
  ];
  const hasOperatorField = Boolean(policy.createOperatorField || policy.modifyOperatorField);
  const shownHeading = "status" in policy && policy.status === "DRAFT" ? "执行效果预览" : heading;

  return (
    <section className="policy-effect" aria-label={shownHeading}>
      <header><Check size={17} /><strong>{shownHeading}</strong></header>
      <div className="policy-effect-operations">
        {operations.map((operation) => (
          <div key={operation.name}>
            {operation.allowed ? <Check size={15} /> : <Minus size={15} />}
            <strong>{operation.name}：规则{operation.allowed ? "允许" : "禁止"}</strong>
            <span>{operation.allowed ? operation.detail : "不会提供此操作"}</span>
          </div>
        ))}
      </div>
      <p>可编辑列以实时表结构和现有值转换能力为准；主键 id 与服务器自动填写的列不会交给客户端填写。</p>
      {hasOperatorField && <p>Operator 来自部署配置，不代表当前登录用户。</p>}
    </section>
  );
}
