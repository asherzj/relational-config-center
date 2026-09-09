import { useEffect, useId, useState } from "react";
import { fieldPolicyColumns, getFieldPolicies, type FieldPolicies, type FieldPolicyField } from "../../api/field-policies";
import { Button } from "../../components/ui/Button";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { Input } from "../../components/shadcn/input";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Checkbox } from "../../components/shadcn/checkbox";
import { Label } from "../../components/shadcn/label";
import { FieldValueInput } from "./FieldValueInput";
import { conditionFromDraft, createConditionDraft, queryOperatorLabels, validateQueryDraft, type ManagedDataColumn, type QueryConditionDraft, type QuerySpec, type QueryOperator } from "./model";

type Props = { tableName: string; columns: ManagedDataColumn[]; maxPageSize?: number; defaultPageSize?: number; onSubmit: (spec: QuerySpec) => void; onClear: () => void };

// The opening-time configuration and drafts survive collapsing and session recovery.
export function CombinedQueryForm({ tableName, columns: previousColumns, onSubmit, onClear, maxPageSize = 200, defaultPageSize }: Props) {
  const [configuration, setConfiguration] = useState<FieldPolicies>();
  const [failure, setFailure] = useState<unknown>();
  const [attempt, setAttempt] = useState(0);
  const [drafts, setDrafts] = useState<Record<string, FieldQueryDraft>>({});
  const [orderField, setOrderField] = useState("");
  const [orderDirection, setOrderDirection] = useState<"ASC" | "DESC">("DESC");
  const [pageSize, setPageSize] = useState("");
  const [reset, setReset] = useState(0);
  const [expanded, setExpanded] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const contentId = useId();
  useEffect(() => {
    let active = true;
    void getFieldPolicies(tableName).then(data => { if (active) setConfiguration(data); }, error => { if (active) setFailure(error); });
    return () => { active = false; };
  }, [tableName, attempt]);
  const columns = configuration ? fieldPolicyColumns(configuration) : previousColumns;
  const fields = configuration?.fields.filter(field => field.effective.is_queryable && columns.some(column => column.name === field.field_name)) ?? [];
  fields.sort((a, b) => a.effective.display_order - b.effective.display_order || a.field_name.localeCompare(b.field_name));
  const submit = () => {
    if (!configuration || !configuration.query_capacity.supported) return;
    const entered = fields.flatMap(field => { const draft = drafts[field.field_name]; const active = draft && enteredDraft(draft); return active ? [active] : []; });
    const nextError = validateQueryDraft(columns, entered, pageSize, configuration.query_capacity);
    setError(nextError);
    if (!nextError) onSubmit({ conditions: entered.map(conditionFromDraft), pageNumber: 1, ...(orderField ? { order: { field: orderField, direction: orderDirection } } : {}), ...(pageSize ? { pageSize: Number(pageSize) } : {}) });
  };
  return <section className="query-builder" aria-label="查询条件">
    <header><div><strong>组合筛选</strong><small>填写的条件以 AND 连接，未填写的字段不参与筛选。</small></div><Button variant="secondary" aria-expanded={expanded} aria-controls={contentId} onClick={() => setExpanded(!expanded)}>{expanded ? "收起筛选" : "展开筛选"}</Button></header>
    {failure ? <ErrorState error={failure} onRetry={() => { setFailure(undefined); setAttempt(attempt + 1); }} /> : !configuration ? <LoadingState label="正在读取字段查询配置…" /> : <div id={contentId} hidden={!expanded}>
      {!configuration.query_capacity.supported && <p role="alert" className="inline-alert">真实可查询字段共 {configuration.query_capacity.queryable_fields} 个，超过平台上限 {configuration.query_capacity.max_conditions}。请管理员关闭部分字段的查询后重新打开。</p>}
      {fields.length === 0 && <p className="p-4 text-sm text-muted-foreground">此表没有可查询字段，可直接查询全部数据。</p>}
      <div className="grid max-h-[32rem] min-w-0 gap-4 overflow-y-auto p-4 md:grid-cols-2 xl:grid-cols-3">{fields.map(field => {
        const column = columns.find(column => column.name === field.field_name)!;
        const draft = drafts[field.field_name] ?? newDraft(field);
        const label = field.effective.display_name || field.field_name;
        const updateDraft = (next: FieldQueryDraft) => { setError(null); setDrafts(current => ({ ...current, [field.field_name]: next })); };
        return <fieldset key={`${field.field_name}-${reset}`} className="grid min-w-0 content-start gap-2 rounded-lg border p-3"><legend className="max-w-full break-all">{label}</legend><span className="break-all text-sm text-muted-foreground">{field.field_name} · {column.type}</span>
          {field.effective.query_operators.length > 1 ? <Label className="field"><span>运算符</span><NativeSelect aria-label={`${label} 运算符`} value={draft.operator} onChange={event => {
            const operator = event.target.value as QueryOperator;
            updateDraft({ ...(shape(operator) === shape(draft.operator) ? draft : newDraft(field)), operator, active: shape(operator) === "none" });
          }}>{field.effective.query_operators.map(operator => <option key={operator} value={operator}>{queryOperatorLabels[operator]}</option>)}</NativeSelect></Label> : <span className="text-sm text-muted-foreground">{queryOperatorLabels[draft.operator]}</span>}
          {field.warning && <p role="status" className="inline-alert">{field.warning}；已回退文本输入。</p>}
          {field.effective.description && <p className="text-sm text-muted-foreground">{field.effective.description}</p>}
          <QueryValues key={shape(draft.operator)} column={column} field={field} draft={draft} label={label} update={updateDraft} />
        </fieldset>;
      })}</div>
    </div>}
    <div className="query-options">
      <Label className="field"><span>排序字段</span><NativeSelect aria-label="排序字段" value={orderField} onChange={event => setOrderField(event.target.value)}><option value="">使用查询规则默认排序</option>{columns.map(column => <option key={column.name} value={column.name}>{column.name}</option>)}</NativeSelect></Label>
      <Label className="field"><span>排序方向</span><NativeSelect aria-label="排序方向" disabled={!orderField} value={orderDirection} onChange={event => setOrderDirection(event.target.value as "ASC" | "DESC")}><option value="ASC">ASC</option><option value="DESC">DESC</option></NativeSelect></Label>
      <Label className="field"><span>每页数量</span><Input aria-label="每页数量" type="number" min="1" max={maxPageSize} placeholder={defaultPageSize ? `规则默认 ${defaultPageSize}，最多 ${maxPageSize}` : "使用查询规则默认数量"} value={pageSize} onChange={event => setPageSize(event.target.value)} /></Label>
    </div>
    {error && <div role="alert" className="inline-alert query-validation">{error}</div>}
    <footer><Button variant="ghost" onClick={() => { setDrafts({}); setReset(reset + 1); setOrderField(""); setOrderDirection("DESC"); setPageSize(""); setError(null); onClear(); }}>清空</Button><Button variant="primary" disabled={!configuration?.query_capacity.supported} onClick={submit}>查询</Button></footer>
  </section>;
}


type FieldQueryDraft = QueryConditionDraft & { active: boolean; empty: boolean; fromEmpty: boolean; toEmpty: boolean; emptyValues: boolean[]; valueKeys: number[]; nextValueKey: number };
function shape(operator: QueryOperator) {
  return operator === "in" || operator === "not_in" ? "set" : operator === "open_range" || operator === "closed_range" ? "range" : operator === "is_null" || operator === "is_not_null" ? "none" : "single";
}
function newDraft(field: FieldPolicyField): FieldQueryDraft {
  return { ...createConditionDraft(field.field_name), operator: field.effective.query_operators[0]!, active: false, empty: false, fromEmpty: false, toEmpty: false, emptyValues: [false], valueKeys: [0], nextValueKey: 1 };
}
function enteredDraft(draft: FieldQueryDraft): QueryConditionDraft | undefined {
  switch (shape(draft.operator)) {
    case "none": return draft.active ? draft : undefined;
    case "single": return draft.value !== "" || draft.empty ? { ...draft, value: draft.empty ? "" : draft.value } : undefined;
    case "range": {
      const fromEnabled = draft.from !== "" || draft.fromEmpty;
      const toEnabled = draft.to !== "" || draft.toEmpty;
      return fromEnabled || toEnabled ? { ...draft, fromEnabled, toEnabled, from: draft.fromEmpty ? "" : draft.from, to: draft.toEmpty ? "" : draft.to } : undefined;
    }
    case "set": {
      const values = draft.values.flatMap((value, index) => draft.emptyValues[index] ? [""] : value !== "" ? [value] : []);
      return values.length ? { ...draft, values } : undefined;
    }
  }
}
function QueryValues({ column, field, draft, label, update }: { column: ManagedDataColumn; field: FieldPolicyField; draft: FieldQueryDraft; label: string; update: (draft: FieldQueryDraft) => void }) {
  const input = (suffix: string, value: string, empty: boolean, change: (value: string, empty: boolean) => void, setEmpty: (empty: boolean) => void) => <div className="grid min-w-0 gap-2">
    {suffix !== "值" && <span className="text-sm text-muted-foreground">{suffix}</span>}
    <FieldValueInput column={column} policy={field.effective} label={`筛选 ${label} ${suffix}`} value={empty ? "" : value} unselected={value === "" && !empty} disabled={empty && field.effective.ui_type !== "radio"} onChange={(value, origin) => change(value, origin === "choice" && value === "")} />
    {column.type === "string" && field.effective.ui_type !== "radio" && <Label className="flex items-center gap-2"><Checkbox aria-label={`${label} ${suffix} 空字符串`} checked={empty} onCheckedChange={checked => setEmpty(checked === true)} />筛选空字符串</Label>}
  </div>;
  if (shape(draft.operator) === "none") return <Label className="flex items-center gap-2"><Checkbox aria-label={`${label} 参与筛选`} checked={draft.active} onCheckedChange={checked => update({ ...draft, active: checked === true })} />参与筛选（无需输入值）</Label>;
  if (shape(draft.operator) === "range") return <div className="grid gap-3">
    <span className="text-sm text-muted-foreground">可填写单边或双边范围</span>
    {input("下界", draft.from, draft.fromEmpty, (from, fromEmpty) => update({ ...draft, from, fromEmpty }), fromEmpty => update({ ...draft, fromEmpty }))}
    {input("上界", draft.to, draft.toEmpty, (to, toEmpty) => update({ ...draft, to, toEmpty }), toEmpty => update({ ...draft, toEmpty }))}
  </div>;
  if (shape(draft.operator) === "set") return <div className="grid gap-3">{draft.values.map((value, index) => <div className="grid gap-1" key={draft.valueKeys[index]}>
    {input(`集合值 ${index + 1}`, value, draft.emptyValues[index] ?? false, (next, empty) => update({ ...draft, values: draft.values.map((value, i) => i === index ? next : value), emptyValues: draft.values.map((_, i) => i === index ? empty : draft.emptyValues[i] ?? false) }), empty => update({ ...draft, emptyValues: draft.values.map((_, i) => i === index ? empty : draft.emptyValues[i] ?? false) }))}
    {draft.values.length > 1 && <Button variant="ghost" aria-label={`${label} 删除集合值 ${index + 1}`} onClick={() => update({ ...draft, values: draft.values.filter((_, i) => i !== index), emptyValues: draft.emptyValues.filter((_, i) => i !== index), valueKeys: draft.valueKeys.filter((_, i) => i !== index) })}>删除此值</Button>}
  </div>)}<Button variant="secondary" aria-label={`${label} 添加集合值`} onClick={() => update({ ...draft, values: [...draft.values, ""], emptyValues: [...draft.emptyValues, false], valueKeys: [...draft.valueKeys, draft.nextValueKey], nextValueKey: draft.nextValueKey + 1 })}>添加集合值</Button></div>;
  return input("值", draft.value, draft.empty, (value, empty) => update({ ...draft, value, empty }), empty => update({ ...draft, empty }));
}
