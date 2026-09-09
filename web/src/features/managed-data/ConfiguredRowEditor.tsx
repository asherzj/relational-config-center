import { useEffect, useState } from "react";
import { ApiError } from "../../api/client";
import { queryManagedTable } from "../../api/managed-data";
import type { ManagedDataColumn, ManagedRowSnapshot } from "./model";
import { fieldPolicyColumns, getFieldPolicies, type FieldPolicies } from "../../api/field-policies";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { ManagedRowEditor, type ManagedRowEditorProps } from "./ManagedRowEditor";

function sameSchema(left: readonly ManagedDataColumn[], right: readonly ManagedDataColumn[]) {
  return left.length === right.length && left.every(column => right.some(other =>
    other.name === column.name && other.type === column.type && other.nullable === column.nullable && Boolean(other.generated) === Boolean(column.generated)));
}

// Each opening reads current configuration once. Background refresh/session
// recovery must not replace an already-open form's interaction rules or input.
export function ConfiguredRowEditor(props: ManagedRowEditorProps) {
  const [opening, setOpening] = useState<{ configuration: FieldPolicies; snapshot: ManagedRowSnapshot; sourceOriginal: ManagedRowEditorProps["original"] }>();
  const [error, setError] = useState<unknown>();
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    const load = async () => {
      const configuration = await getFieldPolicies(props.tableName);
      const columns = fieldPolicyColumns(configuration);
      let original = props.original;
      const schemaChanged = !sameSchema(columns, props.columns);
      if (props.operation === "MODIFY" && schemaChanged) {
        if (typeof original?.id !== "string") throw new ApiError("incompatible_table", "Cannot identify the current record.", 422);
        const fresh = await queryManagedTable(props.tableName, { conditions: [{ field: "id", operator: "exact", value: original.id }], pageNumber: 1, pageSize: 1 });
        original = fresh.rows.length === 1 ? fresh.rows[0] : undefined;
        if (!sameSchema(columns, fresh.columns) || !original) {
          throw new ApiError("incompatible_table", "The current record no longer matches the opening schema. Retry the read.", 422);
        }
      }
      return { configuration, snapshot: { columns, original }, sourceOriginal: props.original };
    };
    void load().then(value => { if (active) setOpening(value); }, failure => { if (active) setError(failure); });
    return () => { active = false; };
  }, [props.tableName, attempt]);
  if (opening) {
    // Keep interaction/schema frozen, but honor the existing recovery or explicit
    // conflict-rebuild baseline. Never advance its expected record version here.
    const snapshot = { ...opening.snapshot, original: props.original === opening.sourceOriginal ? opening.snapshot.original : props.original };
    return <ManagedRowEditor {...props} {...snapshot} fieldPolicies={opening.configuration.fields}
      onReview={content => props.onReview(content, snapshot)} />;
  }
  return <Drawer eyebrow="字段录入配置" open={props.open} title={`${props.operation === "ADD" ? "新增" : "修改"} ${props.tableName} 记录`} onClose={props.onClose}>
    {error ? <ErrorState error={error} onRetry={() => { setError(undefined); setAttempt(value => value + 1); }} /> : <LoadingState label="正在读取字段录入配置…" />}
  </Drawer>;
}
