import { ChevronLeft, ChevronRight, Database, Pencil, Plus, RefreshCw, RotateCcw, Search, Trash2 } from "lucide-react";
import { useState } from "react";
import { Button } from "../../components/ui/Button";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useTablePolicies } from "../table-policies/queries";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import { useMutationPolicy, useMutationPolicyTypes } from "../mutation-policies/queries";
import {
  allowedOperators,
  conditionFromDraft,
  createConditionDraft,
  queryOperatorLabels,
  validateQueryDraft,
  type ManagedDataColumn,
  buildChangeSet,
  type ChangeSetOperation,
  type MutationContent,
  type QuerySpec,
  type QueryConditionDraft,
  type QueryOperator,
} from "./model";
import { useManagedDataMutation, useManagedDataQuery, useManagedDataRowRefetch, type ManagedDataMutationOutcome } from "./queries";
import { ManagedRowEditor } from "./ManagedRowEditor";
import { ChangeSetDialog } from "./ChangeSetDialog";
import { MutationSuccessDialog } from "./MutationSuccessDialog";

const initialQuerySpec: QuerySpec = { conditions: [], pageNumber: 1 };

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

function isInitialQuerySpec(querySpec: QuerySpec) {
  return querySpec.conditions.length === 0
    && querySpec.order === undefined
    && querySpec.pageNumber === 1
    && querySpec.pageSize === undefined;
}

function CellValue({ value }: { value: string | null }) {
  if (value === null) return <span className="cell-state cell-null">NULL</span>;
  if (value === "") return <span className="cell-state cell-empty">空字符串</span>;
  return <span className="cell-value">{value}</span>;
}

function ScalarInput({ column, label, value, onChange }: { column: ManagedDataColumn; label: string; value: string; onChange: (value: string) => void }) {
  if (column.type === "boolean") {
    return (
      <select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="0">0（false）</option>
        <option value="1">1（true）</option>
      </select>
    );
  }
  const inputType = column.type === "date" ? "date" : column.type === "time" ? "time" : "text";
  const inputMode = ["uint64", "int64", "decimal", "float64"].includes(column.type) ? "decimal" : undefined;
  return <input aria-label={label} type={inputType} inputMode={inputMode} step={column.type === "time" ? "0.000001" : undefined} value={value} onChange={(event) => onChange(event.target.value)} />;
}

function ConditionValueEditor({ index, condition, column, update }: {
  index: number;
  condition: QueryConditionDraft;
  column: ManagedDataColumn;
  update: (update: Partial<QueryConditionDraft>) => void;
}) {
  if (condition.operator === "is_null" || condition.operator === "is_not_null") {
    return <div className="condition-no-value">无需输入值</div>;
  }
  if (condition.operator === "open_range" || condition.operator === "closed_range") {
    return (
      <div className="range-inputs">
        <label className="range-boundary-toggle">
          <input type="checkbox" aria-label={`条件 ${index} 使用下界`} checked={condition.fromEnabled} onChange={(event) => update({ fromEnabled: event.target.checked })} />下界
        </label>
        {condition.fromEnabled && <ScalarInput column={column} label={`条件 ${index} 下界`} value={condition.from} onChange={(from) => update({ from })} />}
        <label className="range-boundary-toggle">
          <input type="checkbox" aria-label={`条件 ${index} 使用上界`} checked={condition.toEnabled} onChange={(event) => update({ toEnabled: event.target.checked })} />上界
        </label>
        {condition.toEnabled && <ScalarInput column={column} label={`条件 ${index} 上界`} value={condition.to} onChange={(to) => update({ to })} />}
      </div>
    );
  }
  if (condition.operator === "in" || condition.operator === "not_in") {
    return (
      <div className="membership-values">
        {condition.values.map((value, valueIndex) => (
          <div key={valueIndex}>
            <ScalarInput
              column={column}
              label={`条件 ${index} 集合值 ${valueIndex + 1}`}
              value={value}
              onChange={(next) => update({ values: condition.values.map((item, itemIndex) => itemIndex === valueIndex ? next : item) })}
            />
            {condition.values.length > 1 && <button className="icon-button" aria-label={`条件 ${index} 删除集合值 ${valueIndex + 1}`} onClick={() => update({ values: condition.values.filter((_, itemIndex) => itemIndex !== valueIndex) })}><Trash2 size={14} /></button>}
          </div>
        ))}
        <Button aria-label={`条件 ${index} 添加集合值`} variant="ghost" disabled={condition.values.length >= 100} onClick={() => update({ values: [...condition.values, ""] })}>添加集合值</Button>
        <span className="sr-only" aria-live="polite">条件 {index} 当前 {condition.values.length} 个集合值</span>
      </div>
    );
  }
  return <ScalarInput column={column} label={`条件 ${index} 值`} value={condition.value} onChange={(value) => update({ value })} />;
}

export function ManagedDataPage() {
  const policies = useTablePolicies();
  const [requestedTable, setRequestedTable] = useState("");
  const [querySpec, setQuerySpec] = useState<QuerySpec>(initialQuerySpec);
  const [conditions, setConditions] = useState<QueryConditionDraft[]>([]);
  const [orderField, setOrderField] = useState("");
  const [orderDirection, setOrderDirection] = useState<"ASC" | "DESC">("DESC");
  const [pageSize, setPageSize] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [editorSequence, setEditorSequence] = useState(0);
  const [editor, setEditor] = useState<ManagedDataEditorState | null>(null);
  const [pendingChange, setPendingChange] = useState<PendingManagedDataChange | null>(null);
  const [outcome, setOutcome] = useState<ManagedDataMutationOutcome | null>(null);
  const enabledPolicies = (policies.data ?? []).filter((policy) => policy.enabled);
  const selectedTable = enabledPolicies.some((policy) => policy.tableName === requestedTable)
    ? requestedTable
    : enabledPolicies[0]?.tableName ?? "";
  const result = useManagedDataQuery(selectedTable, querySpec);
  const mutation = useManagedDataMutation();
  const rowRefetch = useManagedDataRowRefetch();
  const selectedPolicy = enabledPolicies.find((policy) => policy.tableName === selectedTable);
  const mutationPolicy = useMutationPolicy(selectedPolicy?.mutationPolicyCode);
  const mutationTypes = useMutationPolicyTypes(Boolean(selectedPolicy));
  const executableMutationPolicy = mutationPolicy.data
    && mutationPolicy.data.code === selectedPolicy?.mutationPolicyCode
    && mutationPolicy.data.status !== "DRAFT"
    && supportsMutationPolicyType(mutationTypes.data, mutationPolicy.data.typeCode)
    ? mutationPolicy.data
    : undefined;
  const autoFillTargets = (operation: ChangeSetOperation): readonly AutoFillTarget[] => operation === "ADD" ? [
    [executableMutationPolicy?.createOperatorField, "operator"],
    [executableMutationPolicy?.createTimeField, "time"],
    [executableMutationPolicy?.modifyOperatorField, "operator"],
    [executableMutationPolicy?.modifyTimeField, "time"],
  ] : operation === "MODIFY" ? [
    [executableMutationPolicy?.modifyOperatorField, "operator"],
    [executableMutationPolicy?.modifyTimeField, "time"],
  ] : [];
  const fieldsFromTargets = (targets: readonly AutoFillTarget[]) => targets.flatMap(([field]) => field ? [field] : []);
  const autoFillFields = new Set([...fieldsFromTargets(autoFillTargets("ADD")), ...fieldsFromTargets(autoFillTargets("MODIFY"))]);
  const changeSetAutoFillFields = (operation: ChangeSetOperation) => new Set(fieldsFromTargets(autoFillTargets(operation)));
  const capabilityReason = (operation: "ADD" | "MODIFY" | "DELETE") => {
    if (!executableMutationPolicy) return "当前 Policy Snapshot 的 Mutation 能力尚不可执行";
    const allowed = operation === "ADD" ? executableMutationPolicy.allowAdd : operation === "MODIFY" ? executableMutationPolicy.allowModify : executableMutationPolicy.allowDelete;
    if (!allowed) return `${operation} 未由当前 Mutation Policy 授权`;
    if (!result.data || operation === "DELETE") return undefined;
    for (const [field, kind] of autoFillTargets(operation)) {
      if (!field) continue;
      const column = result.data.columns.find((candidate) => candidate.name === field);
      if (!column) return `${operation} Auto Fill 字段 ${field} 不存在于实时 Schema`;
      const valid = kind === "operator" ? column.type === "string" : ["date", "time", "datetime", "timestamp"].includes(column.type);
      if (!valid) return `${operation} Auto Fill 字段 ${field} 的实时类型不兼容`;
    }
    return undefined;
  };
  const startEditor = (operation: "ADD" | "MODIFY", row?: Record<string, string | null>) => {
    if (!result.data) return;
    const sequence = editorSequence + 1;
    setEditorSequence(sequence);
    mutation.reset();
    setPendingChange(null);
    setEditor({
      operation,
      tableName: selectedTable,
      columns: [...result.data.columns],
      ...(row ? { row: { ...row } } : {}),
      allAutoFillFields: [...autoFillFields],
      changeSetAutoFillFields: [...changeSetAutoFillFields(operation)],
      sequence,
    });
  };
  const reviewDelete = (row: Record<string, string | null>) => {
    if (!result.data || typeof row.id !== "string") return;
    mutation.reset();
    setEditor(null);
    setPendingChange({ operation: "DELETE", tableName: selectedTable, columns: [...result.data.columns], row: { ...row }, id: row.id, content: {}, changeSetAutoFillFields: [] });
  };
  const changeSet = pendingChange
    ? buildChangeSet(pendingChange.operation, pendingChange.columns, pendingChange.row, pendingChange.content, new Set(pendingChange.changeSetAutoFillFields))
    : null;
  const executePendingChange = () => {
    if (!pendingChange) return;
    mutation.mutate({ operation: pendingChange.operation, tableName: pendingChange.tableName, ...(pendingChange.id !== undefined ? { id: pendingChange.id } : {}), content: pendingChange.content }, {
      onSuccess(nextOutcome) {
        setOutcome(nextOutcome);
        setPendingChange(null);
        setEditor(null);
      },
    });
  };
  const submitQuerySpec = () => {
    if (!result.data) return;
    const error = validateQueryDraft(result.data.columns, conditions, pageSize);
    setValidationError(error);
    if (error) return;
    setQuerySpec({
      conditions: conditions.map(conditionFromDraft),
      ...(orderField ? { order: { field: orderField, direction: orderDirection } } : {}),
      pageNumber: 1,
      ...(pageSize ? { pageSize: Number(pageSize) } : {}),
    });
  };

  return (
    <main className="workspace managed-data-workspace">
      <div className="page-heading">
        <div>
          <h1>配置内容管理</h1>
          <p>依据 enabled Table Policy 查询 Managed Table；字段与类型来自实时 Schema。</p>
        </div>
        <div className="page-heading-actions">
          <Button variant="secondary" icon={<RefreshCw size={16} />} disabled={!selectedTable || result.isFetching} onClick={() => void result.refetch()}>重新查询</Button>
          <Button variant="primary" icon={<Plus size={16} />} disabled={!result.data || Boolean(capabilityReason("ADD"))} title={capabilityReason("ADD")} aria-describedby={capabilityReason("ADD") ? "mutation-add-reason" : undefined} onClick={() => startEditor("ADD")}>新增记录</Button>
        </div>
      </div>

      {policies.isPending ? <LoadingState label="正在读取 Managed Table…" /> : policies.isError ? (
        <ErrorState error={policies.error} onRetry={() => void policies.refetch()} />
      ) : enabledPolicies.length === 0 ? (
        <section className="feedback-state managed-data-empty">
          <Database aria-hidden="true" />
          <strong>没有可用的 Managed Table</strong>
          <span>请先为真实数据库表创建并启用完整的 Table Policy。</span>
        </section>
      ) : (
        <>
          <section className="managed-data-toolbar" aria-label="Managed Table 选择">
            <label className="field managed-table-select">
              <span>Managed Table</span>
              <select
                aria-label="Managed Table"
                value={selectedTable}
                onChange={(event) => {
                  setRequestedTable(event.target.value);
                  setQuerySpec(initialQuerySpec);
                  setConditions([]);
                  setOrderField("");
                  setOrderDirection("DESC");
                  setPageSize("");
                  setValidationError(null);
                }}
              >
                {enabledPolicies.map((policy) => <option key={policy.tableName} value={policy.tableName}>{policy.tableName}</option>)}
              </select>
            </label>
            <span className="managed-table-status"><i className="ready-dot" />enabled Table Policy</span>
          </section>

          {result.data && (
            <section className="query-builder" aria-label="查询条件">
              <header>
                <div><strong>查询条件</strong><small>全部条件以 AND 连接，最多 20 个。</small></div>
                <Button
                  variant="secondary"
                  icon={<Plus size={15} />}
                  disabled={conditions.length >= 20 || result.data.columns.length === 0}
                  onClick={() => setConditions((current) => [...current, createConditionDraft(result.data.columns[0]!.name)])}
                >添加条件</Button>
              </header>
              {conditions.length === 0 ? <p className="query-builder-empty">未添加条件，将查询当前 Managed Table 的全部数据。</p> : (
                <div className="condition-list">{conditions.map((condition, index) => {
                  const column = result.data.columns.find((item) => item.name === condition.field) ?? result.data.columns[0]!;
                  const operators = allowedOperators(column);
                  const update = (change: Partial<QueryConditionDraft>) => setConditions((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, ...change } : item));
                  return (
                  <fieldset key={index} className="condition-row" aria-label={`条件 ${index + 1}`}>
                    <label className="field">
                      <span>字段</span>
                      <select
                        aria-label={`条件 ${index + 1} 字段`}
                        value={condition.field}
                        onChange={(event) => {
                          const nextColumn = result.data.columns.find((item) => item.name === event.target.value)!;
                          const nextOperators = allowedOperators(nextColumn);
                          update({ field: event.target.value, operator: nextOperators.includes(condition.operator) ? condition.operator : "exact" });
                        }}
                      >{result.data.columns.map((column) => <option key={column.name} value={column.name}>{column.name}</option>)}</select>
                    </label>
                    <label className="field">
                      <span>操作符</span>
                      <select aria-label={`条件 ${index + 1} 操作符`} value={condition.operator} onChange={(event) => update({ operator: event.target.value as QueryOperator })}>
                        {operators.map((operator) => <option key={operator} value={operator}>{queryOperatorLabels[operator]}</option>)}
                      </select>
                    </label>
                    <div className="field condition-value">
                      <span>值</span>
                      <ConditionValueEditor index={index + 1} condition={condition} column={column} update={update} />
                    </div>
                    <button
                      className="icon-button condition-remove"
                      aria-label={`删除条件 ${index + 1}`}
                      onClick={() => setConditions((current) => current.filter((_, itemIndex) => itemIndex !== index))}
                    ><Trash2 size={16} /></button>
                  </fieldset>
                )})}</div>
              )}
              <div className="query-options">
                <label className="field">
                  <span>排序字段</span>
                  <select aria-label="排序字段" value={orderField} onChange={(event) => setOrderField(event.target.value)}>
                    <option value="">使用 Query Policy 默认排序</option>
                    {result.data.columns.map((column) => <option key={column.name} value={column.name}>{column.name}</option>)}
                  </select>
                </label>
                <label className="field">
                  <span>排序方向</span>
                  <select aria-label="排序方向" value={orderDirection} disabled={!orderField} onChange={(event) => setOrderDirection(event.target.value as "ASC" | "DESC")}>
                    <option value="ASC">ASC</option>
                    <option value="DESC">DESC</option>
                  </select>
                </label>
                <label className="field">
                  <span>每页数量</span>
                  <input aria-label="每页数量" type="number" min="1" max="200" placeholder={`策略默认（当前 ${result.data.page.pageSize}）`} value={pageSize} onChange={(event) => setPageSize(event.target.value)} />
                </label>
              </div>
              {validationError && <div className="inline-alert query-validation" role="alert">{validationError}</div>}
              <footer>
                <Button
                  variant="ghost"
                  icon={<RotateCcw size={15} />}
                  onClick={() => {
                    const needsExplicitRefetch = isInitialQuerySpec(querySpec);
                    setConditions([]);
                    setOrderField("");
                    setOrderDirection("DESC");
                    setPageSize("");
                    setValidationError(null);
                    setQuerySpec(initialQuerySpec);
                    if (needsExplicitRefetch) void result.refetch();
                  }}
                >清空</Button>
                <Button
                  variant="primary"
                  icon={<Search size={15} />}
                  onClick={submitQuerySpec}
                >查询</Button>
              </footer>
            </section>
          )}

          <section className="catalog managed-data-results" aria-label="Managed Data 查询结果">
            {result.isPending ? <LoadingState label="正在查询 Managed Table…" /> : result.isError ? (
              <ErrorState error={result.error} onRetry={() => void result.refetch()} />
            ) : (
              <>
                <div className="table-scroll">
                  <table className="policy-table managed-data-table">
                    <thead><tr>{result.data.columns.map((column) => (
                      <th key={column.name} scope="col">
                        <strong>{column.name}</strong>
                        <small>{column.type} · {column.nullable ? "可为 NULL" : "非 NULL"}</small>
                      </th>
                    ))}<th scope="col">操作</th></tr></thead>
                    <tbody>{result.data.rows.length === 0 ? (
                      <tr><td className="managed-data-no-rows" colSpan={result.data.columns.length + 1}>没有符合条件的配置内容</td></tr>
                    ) : result.data.rows.map((row, rowIndex) => (
                      <tr key={String(row.id ?? rowIndex)}>{result.data.columns.map((column) => (
                        <td key={column.name}><CellValue value={row[column.name] ?? null} /></td>
                      ))}<td className="managed-data-actions">
                        <Button variant="ghost" icon={<Pencil size={14} />} aria-label={`修改记录 ${row.id ?? "未知"}`} disabled={typeof row.id !== "string" || Boolean(capabilityReason("MODIFY"))} title={typeof row.id !== "string" ? "记录缺少可用的 id" : capabilityReason("MODIFY")} aria-describedby={capabilityReason("MODIFY") ? "mutation-modify-reason" : undefined} onClick={() => startEditor("MODIFY", row)}>修改</Button>
                        <Button variant="ghost" icon={<Trash2 size={14} />} aria-label={`删除记录 ${row.id ?? "未知"}`} disabled={typeof row.id !== "string" || Boolean(capabilityReason("DELETE"))} title={typeof row.id !== "string" ? "记录缺少可用的 id" : capabilityReason("DELETE")} aria-describedby={capabilityReason("DELETE") ? "mutation-delete-reason" : undefined} onClick={() => reviewDelete(row)}>删除</Button>
                      </td></tr>
                    ))}</tbody>
                  </table>
                </div>
                <footer className="catalog-footer">
                  <span>共 {result.data.page.totalCount} 条</span>
                  <span>第 {result.data.page.pageNumber} / {result.data.page.totalPages || 0} 页</span>
                  <Button
                    variant="ghost"
                    icon={<ChevronLeft size={15} />}
                    disabled={result.data.page.pageNumber <= 1 || result.isFetching}
                    onClick={() => setQuerySpec((current) => ({ ...current, pageNumber: result.data.page.pageNumber - 1 }))}
                  >上一页</Button>
                  <Button
                    variant="ghost"
                    icon={<ChevronRight size={15} />}
                    disabled={result.data.page.totalPages === 0 || result.data.page.pageNumber >= result.data.page.totalPages || result.isFetching}
                    onClick={() => setQuerySpec((current) => ({ ...current, pageNumber: result.data.page.pageNumber + 1 }))}
                  >下一页</Button>
                </footer>
              </>
            )}
          </section>
          <section className="mutation-capability-notes" aria-label="Mutation Policy 权限">
            {(["ADD", "MODIFY", "DELETE"] as const).map((operation) => capabilityReason(operation) && <span id={`mutation-${operation.toLowerCase()}-reason`} key={operation}>{capabilityReason(operation)}</span>)}
          </section>
          {editor && <ManagedRowEditor
            key={editor?.sequence}
            open={Boolean(editor) && !pendingChange}
            tableName={editor.tableName}
            operation={editor.operation}
            columns={editor.columns}
            original={editor.row}
            autoFillFields={new Set(editor.allAutoFillFields)}
            onClose={() => setEditor(null)}
            onReview={(content) => {
              setPendingChange({
                operation: editor.operation,
                tableName: editor.tableName,
                columns: editor.columns,
                ...(editor.row ? { row: editor.row } : {}),
                ...(typeof editor.row?.id === "string" ? { id: editor.row.id } : {}),
                content,
                changeSetAutoFillFields: editor.changeSetAutoFillFields,
              });
            }}
          />}
          <ChangeSetDialog
            changeSet={changeSet}
            error={mutation.error}
            pending={mutation.isPending}
            onEdit={() => {
              mutation.reset();
              setPendingChange(null);
            }}
            onCancel={() => { mutation.reset(); setPendingChange(null); setEditor(null); }}
            onConfirm={executePendingChange}
          />
          <MutationSuccessDialog
            outcome={outcome}
            retryPending={rowRefetch.isPending}
            onRetry={() => {
              if (!outcome || outcome.operation === "DELETE") return;
              rowRefetch.mutate(
                { operation: outcome.operation, tableName: outcome.tableName, id: outcome.id },
                {
                  onSuccess: setOutcome,
                  onError: (retrievalError) => setOutcome((current) => current ? { ...current, retrievalError } : current),
                },
              );
            }}
            onClose={() => { rowRefetch.reset(); setOutcome(null); }}
          />
        </>
      )}
    </main>
  );
}
