import { isUncertainWriteError } from "../../api/client";
import { useWriteRecovery } from "../../components/ui/WriteRecovery";
import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { presentError } from "../../api/error-messages";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { useToast } from "../../components/ui/Toast";
import { allowedActions, type PolicyAction, type PolicyStatus } from "./model";

export type PolicyLifecycleCommand = "activate" | "deprecate" | "delete";
export type PolicyFormMode = "create" | "replace" | "metadata" | "view";

export type PolicyCommandContent = {
  title: string;
  description: string;
  label: string;
  success: string;
  destructive?: boolean;
};

export type PolicyCommandCopy = Record<PolicyLifecycleCommand, PolicyCommandContent>;

type CommandRunner = {
  run: (code: string, onSuccess: () => void, onError: (error: unknown) => void) => void;
  pending: boolean;
};

type PolicyCommandRunners = Record<PolicyLifecycleCommand, CommandRunner>;

export function policyActionAvailability(status: PolicyStatus, executionSupported: boolean) {
  const allowed = allowedActions[status];
  const canExecute = (action: PolicyAction) => executionSupported && allowed.includes(action);
  return {
    replace: canExecute("replace"),
    activate: canExecute("activate"),
    delete: canExecute("delete"),
    deprecate: canExecute("deprecate"),
    // The display name and description do not affect execution. They remain editable
    // even when this Web version does not understand the policy's execution type.
    metadata: allowed.includes("metadata"),
  };
}

export function policyTypeAvailabilityHint(status: PolicyStatus, executionSupported: boolean): string | null {
  if (executionSupported) return null;
  return policyActionAvailability(status, executionSupported).metadata
    ? "仅可修改名称和描述"
    : "仅可查看";
}

export function requestedPolicyFormMode(creating: boolean, requestedMode: string | null): PolicyFormMode {
  if (creating) return "create";
  if (requestedMode === "edit") return "replace";
  if (requestedMode === "metadata") return "metadata";
  return "view";
}

export function resolvePolicyFormMode(
  requestedMode: PolicyFormMode,
  status: PolicyStatus | undefined,
  executionSupported: boolean,
): PolicyFormMode {
  if (requestedMode === "create" || requestedMode === "view" || !status) return requestedMode;
  const availability = policyActionAvailability(status, executionSupported);
  return availability[requestedMode] ? requestedMode : "view";
}

export function usePolicyLifecycleCommands({
  selectedCode,
  collectionPath,
  copy,
  runners,
}: {
  selectedCode?: string;
  collectionPath: string;
  copy: PolicyCommandCopy;
  runners: PolicyCommandRunners;
}) {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const [pendingCommand, setPendingCommand] = useState<{ command: PolicyLifecycleCommand; code: string } | null>(null);

  const inFlight = useRef(false);
  const lastTarget = useRef<string | undefined>(undefined);
  const uncertainTargets = useRef(new Map<string, unknown>());
  const recovery = useWriteRecovery();
  const request = (command: PolicyLifecycleCommand, code: string) => {
    if (inFlight.current) return;
    lastTarget.current = code;
    recovery.reset();
    if (uncertainTargets.current.has(code)) recovery.fail(uncertainTargets.current.get(code));
    setPendingCommand({ command, code });
  };
  const cancel = () => { if (!inFlight.current) setPendingCommand(null); };
  const confirm = pendingCommand ? copy[pendingCommand.command] : null;
  const pending = pendingCommand ? runners[pendingCommand.command].pending : false;

  useDraftProtection(false, pending);

  const execute = () => {
    if (!pendingCommand || inFlight.current || pending || recovery.blocked.current) return;
    inFlight.current = true;
    const { command, code } = pendingCommand;
    lastTarget.current = code;
    runners[command].run(
      code,
      () => {
        inFlight.current = false;
        showToast(copy[command].success);
        setPendingCommand(null);
        if (command === "delete" && selectedCode === code) navigate(collectionPath);
      },
      (error) => {
        inFlight.current = false;
        recovery.fail(error);
        if (recovery.blocked.current) { uncertainTargets.current.set(code, error); return; }
        const shown = presentError(error);
        showToast(shown.requestId ? `${shown.message}（请求编号：${shown.requestId}）` : shown.message);
        setPendingCommand(null);
      },
    );
  };

  const finishCheck = () => {
    if (lastTarget.current) uncertainTargets.current.delete(lastTarget.current);
    recovery.reset(); setPendingCommand(null);
    navigate(collectionPath);
  };
  return { request, cancel, execute, confirm, pending, recovery, finishCheck, targetCode: lastTarget.current };
}

export function usePolicyFormSubmission<TDraft, TMetadata>({
  mode,
  code,
  collectionPath,
  copy,
  create,
  replace,
  updateMetadata,
  pending,
  error,
}: {
  mode: PolicyFormMode;
  code?: string;
  collectionPath: string;
  copy: { created: string; replaced: string; metadataUpdated: string };
  create: (value: TDraft, onSuccess: (code: string) => void, onError: (error: unknown) => void) => void;
  replace: (code: string, value: TDraft, onSuccess: () => void, onError: (error: unknown) => void) => void;
  updateMetadata: (code: string, value: TMetadata, onSuccess: () => void, onError: (error: unknown) => void) => void;
  pending: boolean;
  error: unknown;
}) {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const inFlight = useRef(false);
  const protection = useDraftProtection(false, pending);
  const recovery = useWriteRecovery();
  const submittedCode = useRef(code);
  const unlock = () => { inFlight.current = false; };
  const fail = (cause: unknown) => { recovery.fail(cause); unlock(); };
  const openDetail = (targetCode: string) => {
    unlock();
    protection.afterSave(() => navigate(`${collectionPath}/${encodeURIComponent(targetCode)}`));
  };

  const submit = (value: TDraft | TMetadata) => {
    if (inFlight.current || pending || recovery.blocked.current || mode === "view") return;
    submittedCode.current = mode === "create" ? (value as { code: string }).code : code;
    inFlight.current = true;
    if (mode === "create") {
      create(value as TDraft, (createdCode) => {
        showToast(copy.created);
        openDetail(createdCode);
      }, fail);
    } else if (mode === "replace" && code) {
      replace(code, value as TDraft, () => {
        showToast(copy.replaced);
        openDetail(code);
      }, fail);
    } else if (mode === "metadata" && code) {
      updateMetadata(code, value as TMetadata, () => {
        showToast(copy.metadataUpdated);
        openDetail(code);
      }, fail);
    }
  };

  return { submit, pending, error: isUncertainWriteError(error) ? null : error, recovery, submittedCode: submittedCode.current };
}
