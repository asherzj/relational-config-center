import { useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { isUncertainWriteError } from "../../api/client";
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
  blocked = false,
  onUncertainWrite,
}: {
  selectedCode?: string;
  collectionPath: string;
  copy: PolicyCommandCopy;
  runners: PolicyCommandRunners;
  blocked?: boolean;
  onUncertainWrite?: (error: unknown, code: string) => void;
}) {
  const navigate = useNavigate();
  const { showToast } = useToast();
  type Command = { command: PolicyLifecycleCommand; code: string };
  const [pendingCommand, setPendingCommand] = useState<Command | null>(null);
  const currentCommand = useRef<Command | null>(null);
  const inFlight = useRef(false);
  const setCommand = (command: Command | null) => {
    currentCommand.current = command;
    setPendingCommand(command);
  };

  const request = (command: PolicyLifecycleCommand, code: string) => {
    if (!blocked && !inFlight.current) setCommand({ command, code });
  };
  const cancel = () => { if (!inFlight.current) setCommand(null); };
  const confirm = pendingCommand ? copy[pendingCommand.command] : null;
  const pending = pendingCommand ? runners[pendingCommand.command].pending : false;

  const execute = () => {
    if (!pendingCommand || pendingCommand !== currentCommand.current || inFlight.current || blocked || pending) return;
    const acceptedCommand = pendingCommand;
    const { command, code } = acceptedCommand;
    // React Query publishes pending state after mutate(): guard the interval
    // synchronously, including repeated events within the same render.
    inFlight.current = true;
    const settle = () => {
      if (currentCommand.current !== acceptedCommand) return false;
      inFlight.current = false;
      setCommand(null);
      return true;
    };
    const onError = (error: unknown) => {
      if (!settle()) return;
      if (isUncertainWriteError(error)) {
        onUncertainWrite?.(error, code);
        return;
      }
      const shown = presentError(error);
      showToast(shown.requestId ? `${shown.message}（请求编号：${shown.requestId}）` : shown.message);
    };
    try {
      runners[command].run(
        code,
        () => {
          if (!settle()) return;
          showToast(copy[command].success);
          if (command === "delete" && selectedCode === code) navigate(collectionPath);
        },
        onError,
      );
    } catch (error) {
      onError(error);
    }
  };

  return { request, cancel, execute, confirm, pending };
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
  blocked = false,
}: {
  mode: PolicyFormMode;
  code?: string;
  collectionPath: string;
  copy: { created: string; replaced: string; metadataUpdated: string };
  create: (value: TDraft, onSuccess: (code: string) => void, onError: () => void) => void;
  replace: (code: string, value: TDraft, onSuccess: () => void, onError: () => void) => void;
  updateMetadata: (code: string, value: TMetadata, onSuccess: () => void, onError: () => void) => void;
  pending: boolean;
  error: unknown;
  blocked?: boolean;
}) {
  const navigate = useNavigate();
  const { showToast } = useToast();
  const inFlight = useRef(false);
  const protection = useDraftProtection(false, pending);
  const unlock = () => { inFlight.current = false; };
  const openDetail = (targetCode: string) => {
    unlock();
    protection.afterSave(() => navigate(`${collectionPath}/${encodeURIComponent(targetCode)}`));
  };

  const submit = (value: TDraft | TMetadata) => {
    if (inFlight.current || pending || blocked || mode === "view") return;
    inFlight.current = true;
    if (mode === "create") {
      create(value as TDraft, (createdCode) => {
        showToast(copy.created);
        openDetail(createdCode);
      }, unlock);
    } else if (mode === "replace" && code) {
      replace(code, value as TDraft, () => {
        showToast(copy.replaced);
        openDetail(code);
      }, unlock);
    } else if (mode === "metadata" && code) {
      updateMetadata(code, value as TMetadata, () => {
        showToast(copy.metadataUpdated);
        openDetail(code);
      }, unlock);
    }
  };

  return { submit, pending, error };
}
