import { useAccountRole } from "../accounts/roles";
import { AlertCircle } from "lucide-react";
import { useMemo, useRef, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { isUncertainWriteError, prioritizeUncertainWriteError } from "../../api/client";
import { useLeaveProtection } from "../../components/ui/LeaveProtection";
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
  queryPolicyKeys,
  useCreateQueryPolicy,
  useQueryPolicy,
  useQueryPolicyTypes,
  useReplaceQueryPolicy,
  useUpdateQueryPolicyMetadata,
} from "./queries";
import { QueryPolicyForm } from "./QueryPolicyForm";

type Props = {
  code?: string;
  commandsBlocked: boolean;
  onRequestCommand: (command: PolicyLifecycleCommand, code: string) => void;
};

export function QueryPolicyDrawer(props: Props) {
  const [searchParams] = useSearchParams();
  // Keep write outcomes across keyed editing sessions, including close/reopen.
  const create = useCreateQueryPolicy();
  const replace = useReplaceQueryPolicy();
  const metadata = useUpdateQueryPolicyMetadata();
  return <QueryPolicySession key={`${props.code}:${searchParams.get("mode")}`} {...props} create={create} replace={replace} metadata={metadata} />;
}

type SessionProps = Props & {
  create: ReturnType<typeof useCreateQueryPolicy>;
  replace: ReturnType<typeof useReplaceQueryPolicy>;
  metadata: ReturnType<typeof useUpdateQueryPolicyMetadata>;
};

function QueryPolicySession({ code, commandsBlocked, onRequestCommand, create, replace, metadata }: SessionProps) {
  const navigate = useNavigate();
  const canManage = useAccountRole("ADMIN");
  const editingAllowedAtOpen = useRef(canManage).current;
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const protection = useLeaveProtection();
  const creating = code === "new";
  const detail = useQueryPolicy(creating ? undefined : code);
  const types = useQueryPolicyTypes();

  const requestedFormMode = editingAllowedAtOpen ? requestedPolicyFormMode(creating, searchParams.get("mode")) : "view";
  const close = () => navigate("/platform/query-policies");
  const editingPolicy = useRef(detail.data);
  if (!editingPolicy.current && detail.data) editingPolicy.current = detail.data;
  const policy = requestedFormMode === "view" ? detail.data : editingPolicy.current;
  const currentSupport = policy ? supportedQueryPolicyTypes.has(policy.typeCode) && Boolean(types.data?.includes(policy.typeCode)) : true;
  const editingSupport = useRef<boolean | undefined>(undefined);
  if (editingSupport.current === undefined && policy && types.data) editingSupport.current = currentSupport;
  const supported = requestedFormMode === "view" ? currentSupport : editingSupport.current ?? currentSupport;
  const mode = resolvePolicyFormMode(requestedFormMode, policy?.status, supported);
  const actions = policy ? policyActionAvailability(policy.status, supported) : null;
  const serverError = prioritizeUncertainWriteError([create.error, replace.error, metadata.error]);
  const uncertain = isUncertainWriteError(serverError);
  const verifyCurrentState = async () => {
    try {
      await queryClient.refetchQueries({ queryKey: queryPolicyKeys.all }, { throwOnError: true });
    } catch {
      // Keep the uncertain write blocked until its current state can be read.
      return;
    }
    create.reset(); replace.reset(); metadata.reset();
    protection.afterSave(close);
  };
  const form = usePolicyFormSubmission<QueryPolicyDraft, QueryPolicyMetadata>({
    mode,
    code,
    blocked: uncertain || commandsBlocked || !canManage,
    collectionPath: "/platform/query-policies",
    copy: { created: "查询规则草稿已创建", replaced: "查询规则草稿已更新", metadataUpdated: "查询规则显示信息已更新" },
    create: (value, onSuccess, onError) => create.mutate(value, { onSuccess: (created) => onSuccess(created.code), onError }),
    replace: (target, value, onSuccess, onError) => replace.mutate({ code: target, draft: value }, { onSuccess, onError }),
    updateMetadata: (target, value, onSuccess, onError) => metadata.mutate({ code: target, metadata: value }, { onSuccess, onError }),
    pending: create.isPending || replace.isPending || metadata.isPending,
    error: serverError,
  });

  const title = useMemo(() => {
    if (mode === "create") return "新建查询规则草稿";
    if (mode === "replace") return "编辑查询规则草稿";
    if (mode === "metadata") return "修改查询规则名称和描述";
    return "查询规则详情";
  }, [mode]);

  let content: ReactNode;
  if (!creating && detail.isPending) content = <LoadingState label="正在读取查询规则…" />;
  else if (!creating && detail.isError && !detail.data) content = <ErrorState error={detail.error} onRetry={() => void detail.refetch()} />;
  else content = (
    <>
      {!supported && (
        <div className="inline-alert"><AlertCircle size={18} /><strong>规则类型目录未确认 {policy?.typeCode} 的查询能力；无法确认执行规则，{actions?.metadata ? "仍可安全查看或修改名称和描述。" : "当前只能安全查看。"}</strong></div>
      )}
      <QueryPolicyForm
        key={`${code}:${mode}`}
        pending={form.pending}
        readOnly={!canManage}
        mode={mode}
        policy={policy}
        typeCodes={types.data ?? []}
        registryState={types.isPending ? "loading" : types.isError ? "error" : "ready"}
        serverError={form.error}
        onVerify={verifyCurrentState}
        onSubmit={form.submit}
      />
    </>
  );

  let footer: ReactNode = <Button onClick={close}>关闭</Button>;
  if (mode === "create" || mode === "replace" || mode === "metadata") {
    footer = (
      <>
        <Button variant="primary" type="submit" form="query-policy-form" disabled={!canManage || form.pending || uncertain || commandsBlocked || (mode === "create" && !types.data?.some((type) => supportedQueryPolicyTypes.has(type)))}>
          {form.pending ? "正在保存…" : mode === "create" ? "创建草稿" : mode === "metadata" ? "保存名称和描述" : "保存执行规则"}
        </Button>
        <Button onClick={close} disabled={form.pending}>取消</Button>
      </>
    );
  } else if (canManage && policy && actions && !uncertain && !commandsBlocked) {
    footer = (
      <>
        {actions.replace && <Button variant="primary" onClick={() => navigate(`?mode=edit`)}>修改执行规则</Button>}
        {actions.activate && <Button variant="primary" onClick={() => onRequestCommand("activate", policy.code)}>激活</Button>}
        {actions.metadata && <Button onClick={() => navigate(`?mode=metadata`)}>修改名称和描述</Button>}
        {actions.deprecate && <Button variant="danger" onClick={() => onRequestCommand("deprecate", policy.code)}>弃用</Button>}
        {actions.delete && <Button variant="danger" onClick={() => onRequestCommand("delete", policy.code)}>删除</Button>}
        <Button className="drawer-close-action" onClick={close}>关闭</Button>
      </>
    );
  }

  return <Drawer open={Boolean(code)} title={title} eyebrow="查询规则" onClose={close} footer={footer}>{content}</Drawer>;
}
