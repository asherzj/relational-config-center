import { ApiError } from "../../api/client";
import { useEffect, useRef, useState } from "react";
import { useWorkspaceRecovery } from "../accounts/ProtectedWorkspace";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import { useMutationPolicy, useMutationPolicyTypes } from "../mutation-policies/queries";
import {
  buildChangeSet,
  type ChangeSetOperation,
  type ManagedDataColumn,
  type ManagedRowSnapshot,
  type ManagedDataMutationOutcome,
  type MutationContent,
} from "./model";
import { useManagedDataRowRefetch } from "./queries";

type ManagedDataEditorState = {
  operation: "ADD" | "MODIFY";
  tableName: string;
  columns: ManagedDataColumn[];
  row?: Record<string, string | null>;
  expectedVersion?: string;
  allAutoFillFields: string[];
  changeSetAutoFillFields: string[];
  sequence: number;
};

type PendingManagedDataChange = {
  operation: ChangeSetOperation;
  tableName: string;
  columns: ManagedDataColumn[];
  row?: Record<string, string | null>;
  expectedVersion?: string;
  id?: string;
  content: MutationContent;
  changeSetAutoFillFields: string[];
};

type AutoFillTarget = readonly [field: string | null | undefined, kind: "operator" | "time"];

export type ManagedDataMutationIntent =
  | { type: "open-editor"; operation: "ADD" | "MODIFY"; row?: Record<string, string | null>; expectedVersion?: string }
  | { type: "review-delete"; row: Record<string, string | null>; expectedVersion?: string }
  | { type: "review-content"; content: MutationContent; snapshot?: ManagedRowSnapshot }
  | { type: "edit-pending" }
  | { type: "cancel-pending" }
  | { type: "retry-recheck" }
  | { type: "inspect-latest" }
  | { type: "rebuild-latest" };

type Options = {
  canEdit: boolean;
  tableName: string;
  mutationPolicyCode?: string;
  columns?: readonly ManagedDataColumn[];
  writeError?:unknown;
  pending?:boolean;
};

export function useManagedDataMutationWorkflow({ canEdit, tableName, mutationPolicyCode, columns, writeError, pending }: Options) {
  const recoveryVersion = useWorkspaceRecovery();
  const mutationPolicy = useMutationPolicy(mutationPolicyCode);
  const mutationTypes = useMutationPolicyTypes(Boolean(mutationPolicyCode));
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

  const [latest, setLatest] = useState<ManagedDataMutationOutcome | null>(null);
  const [requiresRebuild, setRequiresRebuild] = useState(false);
  const recordConflict = requiresRebuild || (writeError instanceof ApiError && writeError.code === "record_version_conflict");

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
        if (fresh.recordVersion !== editor.expectedVersion) { setRequiresRebuild(true); setLatest(fresh); setReviewedRecoveryVersion(version); setRecheckingChange(false); return; }
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
    if (pendingChange?.id===undefined) return;
    const target = { operation: pendingChange.operation, tableName: pendingChange.tableName, id: pendingChange.id };
    const sequence = ++recheckSequence.current;
    setRecheckingChange(true);
    setRecheckError(null);
    rowRefetch.mutate(target, {
      onSuccess(fresh) {
        if (recoveryVersionRef.current !== version || recheckSequence.current !== sequence) return;
        if (fresh.recordVersion !== pendingChange.expectedVersion) { setRequiresRebuild(true); setLatest(fresh); setReviewedRecoveryVersion(version); setRecheckingChange(false); return; }
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
    if (!canEdit) return "当前账号只可查看；修改配置需要编辑者或管理员角色";
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
    if (pendingChange.id===undefined) {
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
    if (pending) return;
    switch (intent.type) {
      case "open-editor": {
        if (!columns) return;
        const sequence = editorSequence + 1;
        setEditorSequence(sequence);
        recheckSequence.current++;
        setRecheckError(null);
        setRecheckingChange(false);
        setReviewedRecoveryVersion(recoveryVersion);
        setPendingChange(null);
        setLatest(null);
        setRequiresRebuild(false);
        setEditor({
          expectedVersion: intent.expectedVersion,
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
        recheckSequence.current++;
        setRecheckError(null);
        setRecheckingChange(false);
        setReviewedRecoveryVersion(recoveryVersion);
        setEditor(null);
        setLatest(null);
        setRequiresRebuild(false);
        setPendingChange({
          expectedVersion: intent.expectedVersion,
          operation: "DELETE", tableName, columns: [...columns], row: { ...intent.row },
          id: intent.row.id, content: {}, changeSetAutoFillFields: [],
        });
        return;
      case "review-content":
        if (!editor || recheckingChange || reviewedRecoveryVersion !== recoveryVersion) return;
        setPendingChange({
          expectedVersion: editor.expectedVersion,
          operation: editor.operation,
          tableName: editor.tableName,
          columns: [...(intent.snapshot?.columns ?? editor.columns)],
          row: intent.snapshot?.original ?? editor.row,
          ...(typeof editor.row?.id === "string" ? { id: editor.row.id } : {}),
          content: intent.content,
          changeSetAutoFillFields: editor.changeSetAutoFillFields,
        });
        return;
      case "edit-pending":
        if (editor && pendingChange) setEditor({
          ...editor, expectedVersion: pendingChange.expectedVersion, row: pendingChange.row, columns: pendingChange.columns,
          allAutoFillFields: [...allAutoFillFields],
          changeSetAutoFillFields: pendingChange.changeSetAutoFillFields,
        });
        setPendingChange(null);
        return;
      case "cancel-pending":
        recheckSequence.current++;
        setRecheckingChange(false);
        setRecheckError(null);
        setPendingChange(null);
        setEditor(null);
        setLatest(null);
        setRequiresRebuild(false);
        return;
      case "inspect-latest": {
        const target = pendingChange ?? (editor?.row?.id !== undefined ? { ...editor, id: editor.row.id } : null);
        if (!target || target.id === undefined || target.id === null) return;
        const sequence = ++recheckSequence.current;
        setRecheckingChange(true); setRecheckError(null); setLatest(null);
        rowRefetch.mutate({ operation: target.operation, tableName: target.tableName, id: target.id }, {
          onSuccess(fresh) { if (sequence !== recheckSequence.current) return; setLatest(fresh); setRecheckingChange(false); },
          onError(error) { if (sequence !== recheckSequence.current) return; setRecheckError(error); setRecheckingChange(false); },
        });
        return;
      }
      case "rebuild-latest":
        if (!latest?.row || latest.recordVersion === undefined || recheckingChange) return;
        setPendingChange((current) => current ? { ...current, row: latest.row, expectedVersion: latest.recordVersion, columns: latest.columns ?? current.columns } : null);
        setEditor((current) => current ? { ...current, row: latest.row, expectedVersion: latest.recordVersion, columns: latest.columns ?? current.columns } : null);
        setLatest(null); setRequiresRebuild(false); setRecheckError(null);
        return;
      case "retry-recheck":
        if (pendingChange) recheckPendingTarget(recoveryVersion);
        else recheckOpenEditor(recoveryVersion);
        return;

    }
  };

  return {
    view: {
      draftInput: pendingChange ? {items:[{table_name:pendingChange.tableName,operation:pendingChange.operation,...(pendingChange.id!==undefined?{id:pendingChange.id}:{}),...(pendingChange.expectedVersion!==undefined?{expected_record_version:pendingChange.expectedVersion}:{}),content:pendingChange.content}]} : null,
      editor,
      changeSet,
      capabilityReasons,
      mutationPolicy: mutationPolicy.data,
      mutationRegistry: mutationTypes.data,
      mutationRegistryState: mutationTypes.isPending ? "loading" as const : mutationTypes.isError ? "error" as const : "ready" as const,
      recheckError,
      recheckingChange,
      latest,
      recordConflict,
      reviewDisabled: !canEdit || recordConflict || Boolean(latest) || recheckingChange || reviewedRecoveryVersion !== recoveryVersion,
    },
    send,
  };
}
