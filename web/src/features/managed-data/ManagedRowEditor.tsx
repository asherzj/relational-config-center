import { RecordConflictReview, type RecordConflictReviewProps } from "./RecordConflictReview";
import type { FieldPolicyField } from "../../api/field-policies";
import { numberConstraintError } from "./number-constraints";
import { FieldValueInput } from "./FieldValueInput";
import { Checkbox } from "../../components/shadcn/checkbox";
import { Badge } from "../../components/shadcn/badge";
import { Label } from "../../components/shadcn/label";
import { useState } from "react";

import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { Drawer } from "../../components/ui/Drawer";
import { Button } from "../../components/ui/Button";
import { ErrorState } from "../../components/ui/Feedback";
import type { ManagedDataColumn, ManagedRowSnapshot, MutationContent } from "./model";

type FieldDraft = { included: boolean; value: string; isNull: boolean };

export type ManagedRowEditorProps = RecordConflictReviewProps & {
  open: boolean;
  error?: unknown;
  tableName: string;
  operation: "ADD" | "MODIFY";
  columns: readonly ManagedDataColumn[];
  fieldPolicies?: readonly FieldPolicyField[];
  original?: Record<string, string | null>;
  autoFillFields: ReadonlySet<string>;
  reviewDisabled?: boolean;
  recheckError?: unknown;
  onRetryRecheck?: () => void;
  onClose: () => void;
  onReview: (content: MutationContent, snapshot?: ManagedRowSnapshot) => void;
};

function initialFields(columns: readonly ManagedDataColumn[], operation: ManagedRowEditorProps["operation"], original?: Record<string, string | null>, policies: readonly FieldPolicyField[] = []) {
  return Object.fromEntries(columns.map(column => {
    const policy = policies.find(field => field.field_name === column.name)?.effective;
    const value = operation === "MODIFY" ? original?.[column.name] : policy?.default_value;
    return [column.name, { included: operation === "MODIFY" || value !== undefined || Boolean(policy?.is_required), value: value ?? "", isNull: value === null }];
  })) as Record<string, FieldDraft>;
}

export function ManagedRowEditor({ open, error, tableName, operation, columns, original, fieldPolicies = [], autoFillFields, reviewDisabled, recheckError, onRetryRecheck, onClose, onReview, ...conflictReview }: ManagedRowEditorProps) {
  const [openingPolicies] = useState(fieldPolicies);
  const fieldRule = (name: string) => openingPolicies.find(field => field.field_name === name);
  const visibleColumns = columns.filter(column => !column.generated && (operation === "ADD" || column.name !== "id") && !autoFillFields.has(column.name)
    && (operation === "MODIFY" || fieldRule(column.name)?.effective.editable_on_add !== false));
  const writableColumns = visibleColumns.filter(column => operation === "ADD" || fieldRule(column.name)?.effective.editable_on_modify !== false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [baseline] = useState(() => initialFields(visibleColumns, operation, original, openingPolicies));
  const [fields, setFields] = useState<Record<string, FieldDraft>>(baseline);
  // Include the controls as well as values: omitted, NULL and empty are distinct,
  // and temporarily omitted typed input still belongs to this draft.
  useDraftProtection(JSON.stringify(fields) !== JSON.stringify(baseline));

  const update = (field: string, change: Partial<FieldDraft>) => {
    setFields((current) => ({ ...current, [field]: { ...current[field]!, ...change } }));
    setErrors(current => { const next = { ...current }; delete next[field]; return next; });
  };
  const content = Object.fromEntries(writableColumns.flatMap((column) => {
    const draft = fields[column.name];
    if (!draft || (operation === "ADD" && !draft.included)) return [];
    return [[column.name, draft.isNull ? null : draft.value]];
  })) as MutationContent;

  return (
    <Drawer
      open={open}
      title={`${operation === "ADD" ? "新增" : "修改"} ${tableName} 记录`}
      eyebrow="Mutation Content"
      onClose={onClose}
      footer={<><Button className="drawer-close-action" onClick={onClose}>取消</Button><Button variant="primary" disabled={reviewDisabled} onClick={() => {
        const next = Object.fromEntries(writableColumns.flatMap(column => {
          const policy = fieldRule(column.name)?.effective;
          const value = content[column.name];
          let message = policy?.is_required && (value == null || value === "") ? "请填写此字段，不能为 NULL 或空字符串" : undefined;
          if (!message && typeof value === "string" && policy?.ui_type === "number") message = numberConstraintError(value, policy.ui_options);
          return message ? [[column.name, message]] : [];
        }));
        setErrors(next);
        if (Object.keys(next).length === 0) onReview(content);
      }}>查看 Change Set</Button></>}
    >
      {error != null && <ErrorState error={error} />}
      <RecordConflictReview {...conflictReview} />
      <p className="form-note">{operation === "MODIFY"
        ? "已带入原记录的值，可直接编辑。未改动的字段保留原值；NULL 与空字符串不同。"
        : "每个字段分别选择是否包含在请求中；NULL 与空字符串具有不同语义。"}</p>
      {operation === "ADD" && <p className="form-note">id 由数据库自增生成时，请保持不包含；非自增主键需要填写 id。</p>}
      {recheckError !== undefined && recheckError !== null && <ErrorState error={recheckError} onRetry={onRetryRecheck} />}
      <div className="mutation-content-fields">
        {visibleColumns.map((column) => {
          const rule = fieldRule(column.name);
          const policy = rule?.effective;
          const readonly = operation === "MODIFY" && policy?.editable_on_modify === false;
          const errorId = errors[column.name] ? `field-error-${column.name}` : undefined;
          const draft = fields[column.name] ?? { included: false, value: "", isNull: false };
          const included = operation === "MODIFY" || draft.included;
          return (
            <fieldset key={column.name} className={`mutation-content-field${operation === "MODIFY" ? " mutation-content-field--modify" : ""}${!column.nullable ? " mutation-content-field--not-null" : ""}`}>
              <legend><span className="mutation-field-heading">{policy?.display_name || column.name}<small>{policy?.display_name && policy.display_name !== column.name ? `${column.name} · ` : ""}{column.type}</small><Badge variant={column.nullable ? "outline" : "secondary"}>{column.nullable ? "允许 NULL" : "不允许 NULL"}</Badge>{policy?.is_required && <Badge variant="outline">必填</Badge>}</span></legend>
              {policy?.description && <p className="form-note col-span-full">{policy.description}</p>}
              {rule?.warning && <p role="status" className="form-note col-span-full">{rule.warning}；已回退文本录入。</p>}
              {readonly && <p className="form-note col-span-full">仅查看原值</p>}
              {operation === "ADD" && <Label className="include-field"><Checkbox aria-label={`包含 ${column.name}`} checked={draft.included} onCheckedChange={(checked) => update(column.name, { included: checked === true })} />包含在请求中</Label>}
              <div className="field"><span>值</span><FieldValueInput column={column} policy={policy} label={`${column.name} 值`} disabled={readonly || !included || draft.isNull} required={policy?.is_required} errorId={errorId} value={draft.value} onChange={value => update(column.name, { value })} />{errorId && <p id={errorId} role="alert" className="text-destructive">{errors[column.name]}</p>}</div>
              {column.nullable && <Label className="include-field mutation-null-toggle" data-disabled={!included || readonly} data-checked={draft.isNull}>
                <Checkbox aria-label={`${column.name} 使用 NULL`} disabled={!included || readonly} checked={draft.isNull} onCheckedChange={(checked) => update(column.name, { isNull: checked === true })} />
                {draft.isNull ? "已设为 NULL" : "设为 NULL"}
              </Label>}
            </fieldset>
          );
        })}
      </div>
    </Drawer>
  );
}
