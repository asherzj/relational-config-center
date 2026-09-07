import { isUncertainWriteError } from "../../api/client";
import { queryManagedTable } from "../../api/managed-data";
import { useEffect, useRef, useState } from "react";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { useWorkspaceRecovery } from "../accounts/ProtectedWorkspace";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import { useMutationPolicy, useMutationPolicyTypes } from "../mutation-policies/queries";
import {
  buildChangeSet,
  type ChangeSetOperation,
  type ManagedDataColumn,
  type ManagedDataMutationOutcome,
  type MutationContent,
} from "./model";
import { useManagedDataMutation, useManagedDataRowRefetch } from "./queries";

type ManagedDataEditorState = {
  operation: "ADD" | "MODIFY";
  tableName: string;
  columns: ManagedDataColumn[];
  row?: Record<string, string | null>;
  allAutoFillFields: string[];
  changeSetAutoFillFields: string[];
  sequence: number;
};

type PendingManagedDataChange = {
  operation: ChangeSetOperation;
  tableName: string;
  columns: ManagedDataColumn[];
  row?: Record<string, string | null>;
  id?: string;
  content: MutationContent;
  changeSetAutoFillFields: string[];
};

type AutoFillTarget = readonly [field: string | null | undefined, kind: "operator" | "time"];

export type ManagedDataMutationIntent =
  | { type: "open-editor"; operation: "ADD" | "MODIFY"; row?: Record<string, string | null> }
  | { type: "review-delete"; row: Record<string, string | null> }
  | { type: "review-content"; content: MutationContent }
  | { type: "edit-pending" }
  | { type: "cancel-pending" }
  | { type: "confirm-pending" }
  | { type: "retry-readback" }
  | { type: "retry-recheck" }
  | { type: "resume-after-check" }
  | { type: "close-outcome" };

type Options = {
  tableName: string;
  mutationPolicyCode?: string;
  columns?: readonly ManagedDataColumn[];
};

export function useManagedDataMutationWorkflow({ tableName, mutationPolicyCode, columns }: Options) {
  const recoveryVersion = useWorkspaceRecovery();
  const mutationPolicy = useMutationPolicy(mutationPolicyCode);
  const mutationTypes = useMutationPolicyTypes(Boolean(mutationPolicyCode));
  const mutation = useManagedDataMutation();
  const inFlight = useRef(false);
  const protection = useDraftProtection(false, mutation.isPending, inFlight);
  const uncertain = useRef(false);
  const rowRefetch = useManagedDataRowRefetch();
  const [editorSequence, setEditorSequence] = useState(0);
  const [editor, setEditor] = useState<ManagedDataEditorState | null>(null);
  const [pendingChange, setPendingChange] = useState<PendingManagedDataChange | null>(null);
  const [reviewedRecoveryVersion, setReviewedRecoveryVersion] = useState(recoveryVersion);
  const [recheckingChange, setRecheckingChange] = useState(false);
  const [recheckError, setRecheckError] = useState<unknown>(null);
  const recoveryVersionRef = useRef(recoveryVersion);
  recoveryVersionRef.current = recoveryVersion;
  const recheckSequence = useRef(0);
  const [outcome, setOutcome] = useState<ManagedDataMutationOutcome | null>(null);

  const executablePolicy = mutationPolicy.data
    && mutationPolicy.data.code === mutationPolicyCode
    && mutationPolicy.data.status !== "DRAFT"
    && supportsMutationPolicyType(mutationTypes.data, mutationPolicy.data.typeCode)
    ? mutationPolicy.data
    : undefined;

  const autoFillTargets = (operation: ChangeSetOperation): readonly AutoFillTarget[] => operation === "ADD" ? [
    [executablePolicy?.createOperatorField, "operator"],
    [executablePolicy?.createTimeField, "time"],
    [executablePolicy?.modifyOperatorField, "operator"],
    [executablePolicy?.modifyTimeField, "time"],
  ] : operation === "MODIFY" ? [
    [executablePolicy?.modifyOperatorField, "operator"],
    [executablePolicy?.modifyTimeField, "time"],
  ] : [];
  const fieldsFromTargets = (targets: readonly AutoFillTarget[]) => targets.flatMap(([field]) => field ? [field] : []);
  const allAutoFillFields = new Set([...fieldsFromTargets(autoFillTargets("ADD")), ...fieldsFromTargets(autoFillTargets("MODIFY"))]);
  const autoFillFieldsFor = (operation: ChangeSetOperation) => new Set(fieldsFromTargets(autoFillTargets(operation)));

  const recheckOpenEditor = (version: number) => {
    if (!editor || editor.operation !== "MODIFY" || typeof editor.row?.id !== "string") return;
    const target = { sequence: editor.sequence, tableName: editor.tableName, id: editor.row.id };
    const sequence = ++recheckSequence.current;
    setRecheckingChange(true);
    setRecheckError(null);
    rowRefetch.mutate({ operation: "MODIFY", tableName: target.tableName, id: target.id }, {
      onSuccess(fresh) {
        if (recoveryVersionRef.current !== version || recheckSequence.current !== sequence) return;
        setEditor((current) => current?.sequence === target.sequence ? {
          ...current,
          row: fresh.row,
          columns: fresh.columns ?? current.columns,
          allAutoFillFields: [...allAutoFillFields],
          changeSetAutoFillFields: [...autoFillFieldsFor("MODIFY")],
        } : current);
        setReviewedRecoveryVersion(version);
        setRecheckingChange(false);
      },
      onError(error) {
        if (recoveryVersionRef.current !== version || recheckSequence.current !== sequence) return;
        setRecheckError(error);
        setRecheckingChange(false);
      },
    });
  };
  const recheckPendingTarget = (version: number) => {
    if (!pendingChange?.id) return;
    const target = { operation: pendingChange.operation, tableName: pendingChange.tableName, id: pendingChange.id };
    const sequence = ++recheckSequence.current;
    setRecheckingChange(true);
    setRecheckError(null);
    rowRefetch.mutate(target, {
      onSuccess(fresh) {
        if (recoveryVersionRef.current !== version || recheckSequence.current !== sequence) return;
        setPendingChange((current) => current?.id === target.id && current.tableName === target.tableName ? {
          ...current,
          row: fresh.row,
          columns: fresh.columns ?? current.columns,
          changeSetAutoFillFields: [...autoFillFieldsFor(current.operation)],
        } : current);
        setReviewedRecoveryVersion(version);
        setRecheckingChange(false);
      },
      onError(error) {
        if (recoveryVersionRef.current !== version || recheckSequence.current !== sequence) return;
        setRecheckError(error);
        setRecheckingChange(false);
      },
    });
  };

  const capabilityReason = (operation: ChangeSetOperation): string | undefined => {
    if (!executablePolicy) return "当前规则快照的变更能力尚不可执行";
    const allowed = operation === "ADD" ? executablePolicy.allowAdd : operation === "MODIFY" ? executablePolicy.allowModify : executablePolicy.allowDelete;
    if (!allowed) return `${operation} 未由当前变更规则授权`;
    if (!columns || operation === "DELETE") return undefined;
    for (const [field, kind] of autoFillTargets(operation)) {
      if (!field) continue;
      const column = columns.find((candidate) => candidate.name === field);
      if (!column) return `${operation} Auto Fill 字段 ${field} 不存在于实时 Schema`;
      const valid = kind === "operator" ? column.type === "string" : ["date", "time", "datetime", "timestamp"].includes(column.type);
      if (!valid) return `${operation} Auto Fill 字段 ${field} 的实时类型不兼容`;
    }
    return undefined;
  };

  const capabilityReasons: Record<ChangeSetOperation, string | undefined> = {
    ADD: capabilityReason("ADD"),
    MODIFY: capabilityReason("MODIFY"),
    DELETE: capabilityReason("DELETE"),
  };
  const changeSet = pendingChange
    ? buildChangeSet(pendingChange.operation, pendingChange.columns, pendingChange.row, pendingChange.content, new Set(pendingChange.changeSetAutoFillFields))
    : null;

  useEffect(() => {
    if (recoveryVersion === 0) return;
    if (!inFlight.current && !uncertain.current && !isUncertainWriteError(mutation.error)) mutation.reset();
    recheckSequence.current++;
    setRecheckingChange(false);
    setRecheckError(null);
    if (editor && !pendingChange && columns && tableName === editor.tableName) {
      setEditor((current) => current ? {
        ...current,
        columns: [...columns!],
        allAutoFillFields: [...allAutoFillFields],
        changeSetAutoFillFields: [...autoFillFieldsFor(current.operation)],
      } : current);
      if (editor.operation === "MODIFY" && typeof editor.row?.id === "string") {
        recheckOpenEditor(recoveryVersion);
      } else {
        setReviewedRecoveryVersion(recoveryVersion);
      }
    }
    if (!pendingChange) {
      return;
    }
    if (!pendingChange.id) {
      if (columns && tableName === pendingChange.tableName) {
        setPendingChange((current) => current ? {
          ...current,
          columns: [...columns!],
          changeSetAutoFillFields: [...autoFillFieldsFor(current.operation)],
        } : current);
        setReviewedRecoveryVersion(recoveryVersion);
      }
      return;
    }
    recheckPendingTarget(recoveryVersion);
  // Recovery changes only after the workspace has refetched every active server snapshot.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [recoveryVersion]);

  const send = (intent: ManagedDataMutationIntent) => {
    if (inFlight.current || mutation.isPending) return;
    switch (intent.type) {
      case "open-editor": {
        if (!columns) return;
        const sequence = editorSequence + 1;
        setEditorSequence(sequence);
        uncertain.current = false;
        mutation.reset();
        recheckSequence.current++;
        setRecheckError(null);
        setRecheckingChange(false);
        setReviewedRecoveryVersion(recoveryVersion);
        setPendingChange(null);
        setEditor({
          operation: intent.operation,
          tableName,
          columns: [...columns],
          ...(intent.row ? { row: { ...intent.row } } : {}),
          allAutoFillFields: [...allAutoFillFields],
          changeSetAutoFillFields: [...autoFillFieldsFor(intent.operation)],
          sequence,
        });
        return;
      }
      case "review-delete":
        if (!columns || typeof intent.row.id !== "string") return;
        uncertain.current = false;
        mutation.reset();
        recheckSequence.current++;
        setRecheckError(null);
        setRecheckingChange(false);
        setReviewedRecoveryVersion(recoveryVersion);
        setEditor(null);
        setPendingChange({
          operation: "DELETE", tableName, columns: [...columns], row: { ...intent.row },
          id: intent.row.id, content: {}, changeSetAutoFillFields: [],
        });
        return;
      case "review-content":
        if (!editor || recheckingChange || reviewedRecoveryVersion !== recoveryVersion) return;
        setPendingChange({
          operation: editor.operation,
          tableName: editor.tableName,
          columns: editor.columns,
          ...(editor.row ? { row: editor.row } : {}),
          ...(typeof editor.row?.id === "string" ? { id: editor.row.id } : {}),
          content: intent.content,
          changeSetAutoFillFields: editor.changeSetAutoFillFields,
        });
        return;
      case "resume-after-check":
        uncertain.current = false;
        recheckSequence.current++;
        setRecheckingChange(false);
        setRecheckError(null);
        setReviewedRecoveryVersion(recoveryVersion);
        mutation.reset();
        setPendingChange(null);
        return;
      case "edit-pending":
        if (uncertain.current || isUncertainWriteError(mutation.error)) return;
        if (editor && pendingChange) setEditor({
          ...editor, row: pendingChange.row, columns: pendingChange.columns,
          allAutoFillFields: [...allAutoFillFields],
          changeSetAutoFillFields: pendingChange.changeSetAutoFillFields,
        });
        setPendingChange(null);
        return;
      case "cancel-pending":
        uncertain.current = false;
        recheckSequence.current++;
        setRecheckingChange(false);
        setRecheckError(null);
        mutation.reset();
        setPendingChange(null);
        setEditor(null);
        return;
      case "confirm-pending":
        if (!pendingChange || recheckingChange || reviewedRecoveryVersion !== recoveryVersion || uncertain.current || isUncertainWriteError(mutation.error)) return;
        inFlight.current = true;
        mutation.mutate({
          operation: pendingChange.operation,
          tableName: pendingChange.tableName,
          ...(pendingChange.id !== undefined ? { id: pendingChange.id } : {}),
          content: pendingChange.content,
        }, {
          onSuccess(nextOutcome) {
            setOutcome(nextOutcome);
            setPendingChange(null);
            setEditor(null);
          },
          onError(error) { uncertain.current = isUncertainWriteError(error); },
          onSettled() { inFlight.current = false; protection.submissionSettled(); },
        });
        return;
      case "retry-recheck":
        if (pendingChange) recheckPendingTarget(recoveryVersion);
        else recheckOpenEditor(recoveryVersion);
        return;
      case "retry-readback":
        if (!outcome || outcome.operation === "DELETE") return;
        rowRefetch.mutate(
          { operation: outcome.operation, tableName: outcome.tableName, id: outcome.id },
          {
            onSuccess: setOutcome,
            onError: (retrievalError) => setOutcome((current) => current ? { ...current, retrievalError } : current),
          },
        );
        return;
      case "close-outcome":
        rowRefetch.reset();
        setOutcome(null);
    }
  };

  return {
    view: {
      editor,
      changeSet,
      outcome,
      capabilityReasons,
      mutationPolicy: mutationPolicy.data,
      mutationRegistry: mutationTypes.data,
      mutationRegistryState: mutationTypes.isPending ? "loading" as const : mutationTypes.isError ? "error" as const : "ready" as const,
      recheckError,
      recheckingChange,
      reviewDisabled: recheckingChange || reviewedRecoveryVersion !== recoveryVersion,
      executionError: mutation.error,
      executionPending: mutation.isPending,
      retryPending: rowRefetch.isPending,
    },
    checkCurrent: async () => {
      if (!pendingChange) return;
      // Only an existing row gives us a known stored ID. ADD input may be
      // transformed by MySQL (for example auto-increment zero); a lost response
      // leaves its actual ID unknown even when the caller supplied one.
      const id = pendingChange.id;
      return queryManagedTable(pendingChange.tableName, {
        conditions: typeof id === "string" ? [{ field: "id", operator: "exact", value: id }] : [],
        pageNumber: 1, ...(typeof id === "string" ? { pageSize: 1 } : {}),
      });
    },
    send,
  };
}
