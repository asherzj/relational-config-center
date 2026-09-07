import { Input } from "../../components/shadcn/input";
import { Checkbox } from "../../components/shadcn/checkbox";
import { Label } from "../../components/shadcn/label";
import { useState } from "react";
import { ManagedTextInput } from "./ManagedTextInput";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { Drawer } from "../../components/ui/Drawer";
import { Button } from "../../components/ui/Button";
import { ErrorState } from "../../components/ui/Feedback";
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
  reviewDisabled?: boolean;
  recheckError?: unknown;
  onRetryRecheck?: () => void;
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

export function ManagedRowEditor({ open, error, tableName, operation, columns, original, autoFillFields, reviewDisabled, recheckError, onRetryRecheck, onClose, onReview }: Props) {
  const writableColumns = columns.filter((column) => (operation === "ADD" || column.name !== "id") && !autoFillFields.has(column.name));
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
      footer={<><Button className="drawer-close-action" onClick={onClose}>取消</Button><Button variant="primary" disabled={reviewDisabled} onClick={() => onReview(content)}>查看 Change Set</Button></>}
    >
      {error != null && <ErrorState error={error} />}
      <p className="form-note">每个字段分别选择是否包含在请求中；NULL 与空字符串具有不同语义。</p>
      {operation === "ADD" && <p className="form-note">id 由数据库自增生成时，请保持不包含；非自增主键需要填写 id。</p>}
      {recheckError !== undefined && recheckError !== null && <ErrorState error={recheckError} onRetry={onRetryRecheck} />}
      <div className="mutation-content-fields">
        {writableColumns.map((column) => {
          const draft = fields[column.name] ?? { included: false, value: "", isNull: false };
          return (
            <fieldset key={column.name} className="mutation-content-field">
              <legend>{column.name}<small>{column.type} · {column.nullable ? "可为 NULL" : "非 NULL"}</small></legend>
              <Label className="include-field"><Checkbox aria-label={`包含 ${column.name}`} checked={draft.included} onCheckedChange={(checked) => update(column.name, { included: checked === true })} />包含在请求中</Label>
              <div className="field"><span>值</span>{column.type === "string" || column.type === "json"
                ? <ManagedTextInput label={`${column.name} 值`} disabled={!draft.included || draft.isNull} value={draft.value} onChange={(value) => update(column.name, { value })} />
                : <Input aria-label={`${column.name} 值`} disabled={!draft.included || draft.isNull} value={draft.value} onChange={(event) => update(column.name, { value: event.target.value })} />}</div>
              <Label className="include-field"><Checkbox aria-label={`${column.name} 使用 NULL`} disabled={!draft.included || !column.nullable} checked={draft.isNull} onCheckedChange={(checked) => update(column.name, { isNull: checked === true })} />NULL</Label>
            </fieldset>
          );
        })}
      </div>
    </Drawer>
  );
}
