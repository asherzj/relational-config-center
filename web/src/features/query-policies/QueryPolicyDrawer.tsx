import { AlertCircle } from "lucide-react";
import { useMemo, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import {
  policyActionAvailability,
  requestedPolicyFormMode,
  resolvePolicyFormMode,
  usePolicyFormSubmission,
  type PolicyLifecycleCommand,
} from "../policies/lifecycle";
import { supportedQueryPolicyTypes, type QueryPolicyDraft, type QueryPolicyMetadata } from "./model";
import {
  useCreateQueryPolicy,
  useQueryPolicy,
  useQueryPolicyTypes,
  useReplaceQueryPolicy,
  useUpdateQueryPolicyMetadata,
} from "./queries";
import { QueryPolicyForm } from "./QueryPolicyForm";

type Props = {
  code?: string;
  onRequestCommand: (command: PolicyLifecycleCommand, code: string) => void;
};

export function QueryPolicyDrawer({ code, onRequestCommand }: Props) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const creating = code === "new";
  const detail = useQueryPolicy(creating ? undefined : code);
  const types = useQueryPolicyTypes();
  const create = useCreateQueryPolicy();
  const replace = useReplaceQueryPolicy();
  const metadata = useUpdateQueryPolicyMetadata();

  const requestedFormMode = requestedPolicyFormMode(creating, searchParams.get("mode"));
  const close = () => navigate("/platform/query-policies");
  const policy = detail.data;
  const supported = policy ? supportedQueryPolicyTypes.has(policy.typeCode) : true;
  const mode = resolvePolicyFormMode(requestedFormMode, policy?.status, supported);
  const actions = policy ? policyActionAvailability(policy.status, supported) : null;
  const form = usePolicyFormSubmission<QueryPolicyDraft, QueryPolicyMetadata>({
    mode,
    code,
    collectionPath: "/platform/query-policies",
    copy: { created: "查询规则草稿已创建", replaced: "查询规则草稿已更新", metadataUpdated: "查询规则显示信息已更新" },
    create: (value, onSuccess) => create.mutate(value, { onSuccess: (created) => onSuccess(created.code) }),
    replace: (target, value, onSuccess) => replace.mutate({ code: target, draft: value }, { onSuccess }),
    updateMetadata: (target, value, onSuccess) => metadata.mutate({ code: target, metadata: value }, { onSuccess }),
    pending: create.isPending || replace.isPending || metadata.isPending,
    error: create.error || replace.error || metadata.error,
  });

  const title = useMemo(() => {
    if (mode === "create") return "新建查询规则草稿";
    if (mode === "replace") return "编辑查询规则草稿";
    if (mode === "metadata") return "更新查询规则信息";
    return "查询规则详情";
  }, [mode]);

  let content: ReactNode;
  if (!creating && detail.isPending) content = <LoadingState label="正在读取查询规则…" />;
  else if (!creating && detail.isError) content = <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  else content = (
    <>
      {!supported && (
        <div className="inline-alert"><AlertCircle size={18} /><strong>Web 尚不支持类型 {policy?.typeCode} 的执行内容；不能编辑或执行，但仍可更新名称和描述。</strong></div>
      )}
      <QueryPolicyForm
        mode={mode}
        policy={policy}
        typeCodes={types.data ?? []}
        serverError={form.error}
        onSubmit={form.submit}
      />
    </>
  );

  let footer: ReactNode = <Button onClick={close}>关闭</Button>;
  if (mode === "create" || mode === "replace" || mode === "metadata") {
    footer = (
      <>
        <Button variant="primary" type="submit" form="query-policy-form" disabled={form.pending || (mode === "create" && !types.data?.some((type) => supportedQueryPolicyTypes.has(type)))}>
          {form.pending ? "正在保存…" : mode === "create" ? "创建草稿" : "保存"}
        </Button>
        <Button onClick={close} disabled={form.pending}>取消</Button>
      </>
    );
  } else if (policy && actions) {
    footer = (
      <>
        {actions.replace && <Button variant="primary" onClick={() => navigate(`?mode=edit`)}>编辑草稿</Button>}
        {actions.activate && <Button variant="primary" onClick={() => onRequestCommand("activate", policy.code)}>激活</Button>}
        {actions.metadata && <Button onClick={() => navigate(`?mode=metadata`)}>更新元数据</Button>}
        {actions.deprecate && <Button variant="danger" onClick={() => onRequestCommand("deprecate", policy.code)}>弃用</Button>}
        {actions.delete && <Button variant="danger" onClick={() => onRequestCommand("delete", policy.code)}>删除</Button>}
        <Button className="drawer-close-action" onClick={close}>关闭</Button>
      </>
    );
  }

  return <Drawer open={Boolean(code)} title={title} eyebrow="查询规则" onClose={close} footer={footer}>{content}</Drawer>;
}
