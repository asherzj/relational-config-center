import { AlertCircle } from "lucide-react";
import { useMemo, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { isUncertainWriteError } from "../../api/client";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { allowedActions, supportsMutationPolicyType, supportedMutationPolicyTypes, type MutationPolicyDraft, type MutationPolicyMetadata } from "./model";
import {
  useCreateMutationPolicy,
  useMutationPolicy,
  useMutationPolicyTypes,
  useReplaceMutationPolicy,
  useUpdateMutationPolicyMetadata,
  mutationPolicyKeys,
} from "./queries";
import { MutationPolicyForm, type FormMode } from "./MutationPolicyForm";

type Props = {
  code?: string;
  onRequestCommand: (command: "activate" | "deprecate" | "delete", code: string) => void;
};

export function MutationPolicyDrawer({ code, onRequestCommand }: Props) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();
  const queryClient = useQueryClient();
  const creating = code === "new";
  const detail = useMutationPolicy(creating ? undefined : code);
  const types = useMutationPolicyTypes();
  const create = useCreateMutationPolicy();
  const replace = useReplaceMutationPolicy();
  const metadata = useUpdateMutationPolicyMetadata();
  const requestedMode = searchParams.get("mode");
  const requestedFormMode: FormMode = creating ? "create" : requestedMode === "edit" ? "replace" : requestedMode === "metadata" ? "metadata" : "view";
  const close = () => navigate("/platform/mutation-policies");
  const policy = detail.data;
  const supported = policy ? supportsMutationPolicyType(types.data, policy.typeCode) : true;
  const requestedAction = requestedFormMode === "replace" ? "replace" : requestedFormMode === "metadata" ? "metadata" : null;
  const requestedActionAllowed = !policy || !requestedAction || allowedActions[policy.status].includes(requestedAction);
  const unknownMetadataAllowed = !supported && requestedFormMode === "metadata" && Boolean(policy && allowedActions[policy.status].includes("metadata"));
  const mode: FormMode = !creating && policy && ((!supported && !unknownMetadataAllowed) || !requestedActionAllowed) ? "view" : requestedFormMode;
  const pending = create.isPending || replace.isPending || metadata.isPending;
  const serverError = create.error || replace.error || metadata.error;
  const uncertain = isUncertainWriteError(serverError);
  const verifyCurrentState = () => {
    create.reset(); replace.reset(); metadata.reset();
    void queryClient.refetchQueries({ queryKey: mutationPolicyKeys.all }).finally(() => close());
  };

  const title = useMemo(() => {
    if (mode === "create") return "新建变更规则草稿";
    if (mode === "replace") return "编辑变更规则草稿";
    if (mode === "metadata") return "更新变更规则信息";
    return "变更规则详情";
  }, [mode]);

  const submit = (value: MutationPolicyDraft | MutationPolicyMetadata) => {
    if (mode === "create") create.mutate(value as MutationPolicyDraft, { onSuccess(created) { showToast("变更规则草稿已创建"); navigate(`/platform/mutation-policies/${encodeURIComponent(created.code)}`); } });
    else if (mode === "replace" && code) replace.mutate({ code, draft: value as MutationPolicyDraft }, { onSuccess() { showToast("变更规则草稿已更新"); navigate(`/platform/mutation-policies/${encodeURIComponent(code)}`); } });
    else if (mode === "metadata" && code) metadata.mutate({ code, metadata: value as MutationPolicyMetadata }, { onSuccess() { showToast("变更规则显示信息已更新"); navigate(`/platform/mutation-policies/${encodeURIComponent(code)}`); } });
  };

  let content: ReactNode;
  if (!creating && detail.isPending) content = <LoadingState label="正在读取变更规则…" />;
  else if (!creating && detail.isError && !detail.data) content = <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  else content = <>{!supported && <div className="inline-alert"><AlertCircle size={18} /><strong>Type Registry 未确认 {policy?.typeCode} 的完整 ADD/MODIFY/DELETE 能力；执行规则失败关闭，仅可安全查看或更新元数据。</strong></div>}<MutationPolicyForm mode={mode} policy={policy} typeCodes={(types.data ?? []).map((type) => type.code)} serverError={serverError} onVerify={verifyCurrentState} onSubmit={submit} /></>;

  let footer: ReactNode = <Button onClick={close}>关闭</Button>;
  if (mode === "create" || mode === "replace" || mode === "metadata") footer = <><Button variant="primary" type="submit" form="mutation-policy-form" disabled={pending || uncertain || (mode === "create" && !types.data?.some((type) => supportedMutationPolicyTypes.has(type.code)))}>{pending ? "正在保存…" : mode === "create" ? "创建草稿" : "保存"}</Button><Button onClick={close} disabled={pending}>取消</Button></>;
  else if (policy) footer = <>{policy.status === "DRAFT" && supported && <Button variant="primary" onClick={() => navigate("?mode=edit")}>编辑草稿</Button>}{policy.status === "DRAFT" && supported && <Button variant="primary" onClick={() => onRequestCommand("activate", policy.code)}>激活</Button>}{policy.status !== "DRAFT" && <Button onClick={() => navigate("?mode=metadata")}>更新元数据</Button>}{policy.status === "ACTIVE" && supported && <Button variant="danger" onClick={() => onRequestCommand("deprecate", policy.code)}>弃用</Button>}{policy.status === "DRAFT" && supported && <Button variant="danger" onClick={() => onRequestCommand("delete", policy.code)}>删除</Button>}<Button className="drawer-close-action" onClick={close}>关闭</Button></>;

  return <Drawer open={Boolean(code)} title={title} eyebrow="变更规则" onClose={close} footer={footer}>{content}</Drawer>;
}
