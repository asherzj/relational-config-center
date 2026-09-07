import { useState } from "react";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { ErrorState } from "../../components/ui/Feedback";
import { Drawer } from "../../components/ui/Drawer";
import { Button } from "../../components/ui/Button";
import type { ManagedDataColumn, MutationContent } from "./model";

type FieldDraft = { included: boolean; value: string; isNull: boolean };

type Props = {
  open: boolean;
  error?: unknown;
  tableName: string;
  operation: "ADD" | "MODIFY";
  columns: readonly ManagedDataColumn[];
  original?: Record<string, string | null>;
  autoFillFields: ReadonlySet<string>;
  onClose: () => void;
  onReview: (content: MutationContent) => void;
};

function initialFields(columns: readonly ManagedDataColumn[], original?: Record<string, string | null>) {
  return Object.fromEntries(columns.map((column) => [column.name, {
    included: false,
    value: original?.[column.name] ?? "",
    isNull: original?.[column.name] === null,
  }])) as Record<string, FieldDraft>;
}

export function ManagedRowEditor({ open, error, tableName, operation, columns, original, autoFillFields, onClose, onReview }: Props) {
  const writableColumns = columns.filter((column) => column.name !== "id" && !autoFillFields.has(column.name));
  const [baseline] = useState(() => initialFields(writableColumns, original));
  const [fields, setFields] = useState<Record<string, FieldDraft>>(baseline);
  // Include the controls as well as values: omitted, NULL and empty are distinct,
  // and temporarily omitted typed input still belongs to this draft.
  useDraftProtection(JSON.stringify(fields) !== JSON.stringify(baseline));

  const update = (field: string, change: Partial<FieldDraft>) => {
    setFields((current) => ({ ...current, [field]: { ...current[field]!, ...change } }));
  };
  const content = Object.fromEntries(writableColumns.flatMap((column) => {
    const draft = fields[column.name];
    if (!draft?.included) return [];
    return [[column.name, draft.isNull ? null : draft.value]];
  })) as MutationContent;

  return (
    <Drawer
      open={open}
      title={`${operation === "ADD" ? "新增" : "修改"} ${tableName} 记录`}
      eyebrow="Mutation Content"
      onClose={onClose}
      footer={<><Button className="drawer-close-action" onClick={onClose}>取消</Button><Button variant="primary" onClick={() => onReview(content)}>查看 Change Set</Button></>}
    >
      {error != null && <ErrorState error={error} />}
      <p className="form-note">每个字段分别选择是否包含在请求中；NULL 与空字符串具有不同语义。</p>
      <div className="mutation-content-fields">
        {writableColumns.map((column) => {
          const draft = fields[column.name] ?? { included: false, value: "", isNull: false };
          return (
            <fieldset key={column.name} className="mutation-content-field">
              <legend>{column.name}<small>{column.type} · {column.nullable ? "可为 NULL" : "非 NULL"}</small></legend>
              <label className="include-field"><input type="checkbox" aria-label={`包含 ${column.name}`} checked={draft.included} onChange={(event) => update(column.name, { included: event.target.checked })} />包含在请求中</label>
              <label className="field"><span>值</span><input aria-label={`${column.name} 值`} disabled={!draft.included || draft.isNull} value={draft.value} onChange={(event) => update(column.name, { value: event.target.value })} /></label>
              <label className="include-field"><input type="checkbox" aria-label={`${column.name} 使用 NULL`} disabled={!draft.included || !column.nullable} checked={draft.isNull} onChange={(event) => update(column.name, { isNull: event.target.checked })} />NULL</label>
            </fieldset>
          );
        })}
      </div>
    </Drawer>
  );
}
