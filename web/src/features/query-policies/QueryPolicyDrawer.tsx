import { AlertCircle } from "lucide-react";
import { useMemo, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { allowedActions, supportedQueryPolicyTypes, type QueryPolicyDraft, type QueryPolicyMetadata } from "./model";
import {
  useCreateQueryPolicy,
  useQueryPolicy,
  useQueryPolicyTypes,
  useReplaceQueryPolicy,
  useUpdateQueryPolicyMetadata,
} from "./queries";
import { QueryPolicyForm, type FormMode } from "./QueryPolicyForm";

type Props = {
  code?: string;
  onRequestCommand: (command: "activate" | "deprecate" | "delete", code: string) => void;
};

export function QueryPolicyDrawer({ code, onRequestCommand }: Props) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();
  const creating = code === "new";
  const detail = useQueryPolicy(creating ? undefined : code);
  const types = useQueryPolicyTypes();
  const create = useCreateQueryPolicy();
  const replace = useReplaceQueryPolicy();
  const metadata = useUpdateQueryPolicyMetadata();

  const requestedMode = searchParams.get("mode");
  const requestedFormMode: FormMode = creating ? "create" : requestedMode === "edit" ? "replace" : requestedMode === "metadata" ? "metadata" : "view";
  const close = () => navigate("/platform/query-policies");
  const policy = detail.data;
  const supported = policy ? supportedQueryPolicyTypes.has(policy.typeCode) : true;
  const requestedAction = requestedFormMode === "replace" ? "replace" : requestedFormMode === "metadata" ? "metadata" : null;
  const requestedActionAllowed = !policy || !requestedAction || allowedActions[policy.status].includes(requestedAction);
  const mode: FormMode = !creating && policy && (!supported || !requestedActionAllowed) ? "view" : requestedFormMode;
  const pending = create.isPending || replace.isPending || metadata.isPending;
  const serverError = create.error || replace.error || metadata.error;

  const title = useMemo(() => {
    if (mode === "create") return "新建查询策略草稿";
    if (mode === "replace") return "编辑查询策略草稿";
    if (mode === "metadata") return "更新查询策略信息";
    return "查询策略详情";
  }, [mode]);

  const submit = (value: QueryPolicyDraft | QueryPolicyMetadata) => {
    if (mode === "create") {
      create.mutate(value as QueryPolicyDraft, {
        onSuccess(created) {
          showToast("查询策略草稿已创建");
          navigate(`/platform/query-policies/${encodeURIComponent(created.code)}`);
        },
      });
    } else if (mode === "replace" && code) {
      replace.mutate({ code, draft: value as QueryPolicyDraft }, {
        onSuccess() { showToast("查询策略草稿已更新"); navigate(`/platform/query-policies/${encodeURIComponent(code)}`); },
      });
    } else if (mode === "metadata" && code) {
      metadata.mutate({ code, metadata: value as QueryPolicyMetadata }, {
        onSuccess() { showToast("查询策略显示信息已更新"); navigate(`/platform/query-policies/${encodeURIComponent(code)}`); },
      });
    }
  };

  let content: ReactNode;
  if (!creating && detail.isPending) content = <LoadingState label="正在读取查询策略…" />;
  else if (!creating && detail.isError) content = <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  else content = (
    <>
      {!supported && (
        <div className="inline-alert"><AlertCircle size={18} /><strong>Web 尚不支持类型 {policy?.typeCode}，当前仅可查看。</strong></div>
      )}
      <QueryPolicyForm
        mode={mode}
        policy={policy}
        typeCodes={types.data ?? []}
        serverError={serverError}
        onSubmit={submit}
      />
    </>
  );

  let footer: ReactNode = <Button onClick={close}>关闭</Button>;
  if (mode === "create" || mode === "replace" || mode === "metadata") {
    footer = (
      <>
        <Button variant="primary" type="submit" form="query-policy-form" disabled={pending || (mode === "create" && !types.data?.some((type) => supportedQueryPolicyTypes.has(type)))}>
          {pending ? "正在保存…" : mode === "create" ? "创建草稿" : "保存"}
        </Button>
        <Button onClick={close} disabled={pending}>取消</Button>
      </>
    );
  } else if (policy && supported) {
    footer = (
      <>
        {policy.status === "DRAFT" && supported && <Button variant="primary" onClick={() => navigate(`?mode=edit`)}>编辑草稿</Button>}
        {policy.status === "DRAFT" && <Button variant="primary" onClick={() => onRequestCommand("activate", policy.code)}>激活</Button>}
        {policy.status !== "DRAFT" && supported && <Button onClick={() => navigate(`?mode=metadata`)}>更新元数据</Button>}
        {policy.status === "ACTIVE" && <Button variant="danger" onClick={() => onRequestCommand("deprecate", policy.code)}>弃用</Button>}
        {policy.status === "DRAFT" && <Button variant="danger" onClick={() => onRequestCommand("delete", policy.code)}>删除</Button>}
        <Button className="drawer-close-action" onClick={close}>关闭</Button>
      </>
    );
  }

  return <Drawer open={Boolean(code)} title={title} eyebrow="查询策略" onClose={close} footer={footer}>{content}</Drawer>;
}
