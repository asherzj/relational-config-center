import { getMutationPolicy } from "../../api/mutation-policies";
import { WriteRecovery } from "../../components/ui/WriteRecovery";
import { AlertCircle } from "lucide-react";
import { useMemo, useRef, type ReactNode } from "react";
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
import { supportsMutationPolicyType, supportedMutationPolicyTypes, type MutationPolicyDraft, type MutationPolicyMetadata } from "./model";
import {
  useCreateMutationPolicy,
  useMutationPolicy,
  useMutationPolicyTypes,
  useReplaceMutationPolicy,
  useUpdateMutationPolicyMetadata,
} from "./queries";
import { MutationPolicyForm } from "./MutationPolicyForm";

type Props = {
  code?: string;
  onRequestCommand: (command: PolicyLifecycleCommand, code: string) => void;
};

export function MutationPolicyDrawer(props: Props) {
  const [searchParams] = useSearchParams();
  return <MutationPolicySession key={`${props.code}:${searchParams.get("mode")}`} {...props} />;
}

function MutationPolicySession({ code, onRequestCommand }: Props) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const creating = code === "new";
  const detail = useMutationPolicy(creating ? undefined : code);
  const types = useMutationPolicyTypes();
  const create = useCreateMutationPolicy();
  const replace = useReplaceMutationPolicy();
  const metadata = useUpdateMutationPolicyMetadata();
  const requestedFormMode = requestedPolicyFormMode(creating, searchParams.get("mode"));
  const close = () => navigate("/platform/mutation-policies");
  const editingPolicy = useRef(detail.data);
  if (!editingPolicy.current && detail.data) editingPolicy.current = detail.data;
  const policy = requestedFormMode === "view" ? detail.data : editingPolicy.current;
  const currentSupport = policy ? supportsMutationPolicyType(types.data, policy.typeCode) : true;
  const editingSupport = useRef<boolean | undefined>(undefined);
  if (editingSupport.current === undefined && policy && types.data) editingSupport.current = currentSupport;
  // Keep an open edit session intact if the registry refreshes. The write still
  // goes through Admin's current validation; view mode uses current capabilities.
  const supported = requestedFormMode === "view" ? currentSupport : editingSupport.current ?? currentSupport;
  const mode = resolvePolicyFormMode(requestedFormMode, policy?.status, supported);
  const actions = policy ? policyActionAvailability(policy.status, supported) : null;
  const form = usePolicyFormSubmission<MutationPolicyDraft, MutationPolicyMetadata>({
    mode,
    code,
    collectionPath: "/platform/mutation-policies",
    copy: { created: "变更规则草稿已创建", replaced: "变更规则草稿已更新", metadataUpdated: "变更规则显示信息已更新" },
    create: (value, onSuccess, onError) => create.mutate(value, { onSuccess: (created) => onSuccess(created.code), onError }),
    replace: (target, value, onSuccess, onError) => replace.mutate({ code: target, draft: value }, { onSuccess, onError }),
    updateMetadata: (target, value, onSuccess, onError) => metadata.mutate({ code: target, metadata: value }, { onSuccess, onError }),
    pending: create.isPending || replace.isPending || metadata.isPending,
    error: create.error || replace.error || metadata.error,
  });

  const title = useMemo(() => {
    if (mode === "create") return "新建变更规则草稿";
    if (mode === "replace") return "编辑变更规则草稿";
    if (mode === "metadata") return "修改变更规则名称和描述";
    return "变更规则详情";
  }, [mode]);

  let content: ReactNode;
  if (!creating && detail.isPending) content = <LoadingState label="正在读取变更规则…" />;
  else if (!creating && detail.isError && !detail.data) content = <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  else content = <>{!supported && <div className="inline-alert"><AlertCircle size={18} /><strong>规则类型目录未确认 {policy?.typeCode} 的完整新增、修改和删除能力；无法确认执行规则，{actions?.metadata ? "仍可安全查看或修改名称和描述。" : "当前只能安全查看。"}</strong></div>}<MutationPolicyForm key={`${code}:${mode}`} pending={form.pending} mode={mode} policy={policy} typeCodes={(types.data ?? []).map((type) => type.code)} registryTypes={types.data} registryState={types.isPending ? "loading" : types.isError ? "error" : "ready"} serverError={form.error} onSubmit={form.submit} /></>;

  let footer: ReactNode = <Button onClick={close}>关闭</Button>;
  if (mode === "create" || mode === "replace" || mode === "metadata") footer = <><Button variant="primary" type="submit" form="mutation-policy-form" disabled={form.pending || form.recovery.blocked.current || (mode === "create" && !types.data?.some((type) => supportedMutationPolicyTypes.has(type.code)))}>{form.pending ? "正在保存…" : mode === "create" ? "创建草稿" : mode === "metadata" ? "保存名称和描述" : "保存执行规则"}</Button><Button onClick={close} disabled={form.pending}>取消</Button></>;
  else if (policy && actions) footer = <>{actions.replace && <Button variant="primary" onClick={() => navigate("?mode=edit")}>修改执行规则</Button>}{actions.activate && <Button variant="primary" onClick={() => onRequestCommand("activate", policy.code)}>激活</Button>}{actions.metadata && <Button onClick={() => navigate("?mode=metadata")}>修改名称和描述</Button>}{actions.deprecate && <Button variant="danger" onClick={() => onRequestCommand("deprecate", policy.code)}>弃用</Button>}{actions.delete && <Button variant="danger" onClick={() => onRequestCommand("delete", policy.code)}>删除</Button>}<Button className="drawer-close-action" onClick={close}>关闭</Button></>;

  return <Drawer open={Boolean(code)} title={title} eyebrow="变更规则" onClose={close} footer={footer}>{content}<WriteRecovery onResume={form.recovery.reset} error={form.recovery.error} onCheck={() => getMutationPolicy(form.submittedCode!)} /></Drawer>;
}
