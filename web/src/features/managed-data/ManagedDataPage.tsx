import {type DraftContentInput} from "../../api/release-orders";
import {ReleaseItemPager,releasePageSize} from "../release-orders/ReleaseItemPager";
import {useDraftDestination} from "../release-orders/useDraftDestination";
import {Drawer} from "../../components/ui/Drawer";
import {useReleaseWrite} from "../release-orders/useReleaseWrite";
import {ReleaseRecovery} from "../release-orders/ReleaseRecovery";
import {ApiError} from "../../api/client";
import { useAccountRole } from "../accounts/roles";
import { ManagedTextInput } from "./ManagedTextInput";
import { Input } from "../../components/shadcn/input";
import { Checkbox } from "../../components/shadcn/checkbox";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Label } from "../../components/shadcn/label";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Database, Pencil, Plus, RefreshCw, RotateCcw, Search, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { Button } from "../../components/ui/Button";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useTablePolicies } from "../table-policies/queries";
import { useQueryPolicy, useQueryPolicyTypes } from "../query-policies/queries";
import { MutationPolicyEffect, QueryPolicyEffect } from "../policies/PolicyEffect";
import {
  allowedOperators,
  conditionFromDraft,
  createConditionDraft,
  queryOperatorLabels,
  validateQueryDraft,
  type ManagedDataColumn,
  type QuerySpec,
  type QueryConditionDraft,
  type QueryOperator,
} from "./model";
import { useManagedDataQuery } from "./queries";
import { useManagedDataMutationWorkflow, type ManagedDataMutationIntent } from "./mutation-workflow";
import { ManagedRowEditor } from "./ManagedRowEditor";
import { ChangeSetDialog } from "./ChangeSetDialog";

const initialQuerySpec: QuerySpec = { conditions: [], pageNumber: 1 };

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
      <NativeSelect aria-label={label} value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="0">0（false）</option>
        <option value="1">1（true）</option>
      </NativeSelect>
    );
  }
  if (column.type === "string" || column.type === "json") {
    return <ManagedTextInput label={label} rows={2} value={value} onChange={onChange} />;
  }
  const inputType = column.type === "date" ? "date" : column.type === "time" ? "time" : "text";
  const inputMode = ["uint64", "int64", "decimal", "float64"].includes(column.type) ? "decimal" : undefined;
  return <Input aria-label={label} type={inputType} inputMode={inputMode} step={column.type === "time" ? "0.000001" : undefined} value={value} onChange={(event) => onChange(event.target.value)} />;
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
        <Label className="range-boundary-toggle">
          <Checkbox aria-label={`条件 ${index} 使用下界`} checked={condition.fromEnabled} onCheckedChange={(checked) => update({ fromEnabled: checked === true })} />下界
        </Label>
        {condition.fromEnabled && <ScalarInput column={column} label={`条件 ${index} 下界`} value={condition.from} onChange={(from) => update({ from })} />}
        <Label className="range-boundary-toggle">
          <Checkbox aria-label={`条件 ${index} 使用上界`} checked={condition.toEnabled} onCheckedChange={(checked) => update({ toEnabled: checked === true })} />上界
        </Label>
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
            {condition.values.length > 1 && <Button variant="ghost" className="icon-button" aria-label={`条件 ${index} 删除集合值 ${valueIndex + 1}`} onClick={() => update({ values: condition.values.filter((_, itemIndex) => itemIndex !== valueIndex) })}><Trash2 size={14} /></Button>}
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
  const canEdit = useAccountRole("EDITOR");
 const draftWrite=useReleaseWrite("create");
 const navigate=useNavigate();
 const [search]=useSearchParams();
 const [selectedRows,setSelectedRows]=useState<Map<string,{id:string;version:string}>>(()=>new Map());
 const [batchReview,setBatchReview]=useState(false);
 const [batchPage,setBatchPage]=useState(0);
 const [saving,setSaving]=useState(false);
  const policies = useTablePolicies();
  const [requestedTable, setRequestedTable] = useState(search.get("table_name")??"");
  const [expandedPolicyTable, setExpandedPolicyTable] = useState<string>();
  const policyDetailsId = useId();
  const [querySpec, setQuerySpec] = useState<QuerySpec>(initialQuerySpec);
  const [conditions, setConditions] = useState<QueryConditionDraft[]>([]);
  const [orderField, setOrderField] = useState("");
  const [orderDirection, setOrderDirection] = useState<"ASC" | "DESC">("DESC");
  const [pageSize, setPageSize] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const enabledPolicies = (policies.data ?? []).filter((policy) => policy.enabled);
  const selectedTable = enabledPolicies.some((policy) => policy.tableName === requestedTable)
    ? requestedTable
    : enabledPolicies[0]?.tableName ?? "";
  const destination=useDraftDestination(selectedTable,search.get("draft")??"");
  const result = useManagedDataQuery(selectedTable, querySpec);
  const selectedPolicy = enabledPolicies.find((policy) => policy.tableName === selectedTable);
  const policyDetailsOpen = Boolean(selectedTable) && expandedPolicyTable === selectedTable;
  const queryPolicy = useQueryPolicy(selectedPolicy?.queryPolicyCode);
  const queryPolicyTypes = useQueryPolicyTypes(Boolean(selectedPolicy));
  const changes = useManagedDataMutationWorkflow({
    canEdit,
    writeError:draftWrite.error,pending:draftWrite.pending,
    tableName: selectedTable,
    mutationPolicyCode: selectedPolicy?.mutationPolicyCode,
    columns: result.data?.columns,
  });
  const draftRecordConflict=draftWrite.error instanceof ApiError&&draftWrite.error.code==="record_version_conflict";
 const { editor, changeSet, capabilityReasons, mutationPolicy, mutationRegistry, mutationRegistryState } = changes.view;
  const queryRegistryState = queryPolicyTypes.isPending ? "loading" : queryPolicyTypes.isError ? "error" : "ready";
  const protection = useDraftProtection(draftWrite.unresolved, draftWrite.pending);
  const send = (intent: ManagedDataMutationIntent) => {
    if (["open-editor", "review-delete", "cancel-pending"].includes(intent.type)) {
      protection.requestLeave(() => changes.send(intent));
    } else changes.send(intent);
  };
 const saveDraft=async(input:DraftContentInput)=>{
  if(saving||draftWrite.pending)return;setSaving(true);
  try{
   const request=draftWrite.unresolved?undefined:await destination.prepare(input);
   const saved=draftWrite.unresolved?await draftWrite.retry():request?await draftWrite.send({...request,label:`保存 ${selectedTable} 草稿`}):undefined;
   if(saved)protection.afterSave(()=>{changes.send({type:"cancel-pending"});setBatchReview(false);setSelectedRows(new Map());navigate(`/configuration/release-orders/${saved.id}`)});
  }finally{setSaving(false)}
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
      <ReleaseRecovery scopeFilter="create"/>
 <div className="page-heading">
        <div>
          <h1>配置内容管理</h1>
          <p>查找和维护配置记录，按当前表规则核对每一次变更。</p>
        </div>
        <div className="page-heading-actions">
          <Button variant="secondary" icon={<RefreshCw size={16} />} disabled={!selectedTable || result.isFetching} onClick={() => void result.refetch()}>重新查询</Button>
          <Button variant="primary" icon={<Plus size={16} />} disabled={!result.data || Boolean(capabilityReasons.ADD)} title={capabilityReasons.ADD} aria-describedby={capabilityReasons.ADD ? "mutation-add-reason" : undefined} onClick={() => send({ type: "open-editor", operation: "ADD" })}>新增记录</Button>
        </div>
      </div>

      {policies.isPending ? <LoadingState label="正在读取 Managed Table…" /> : policies.isError ? (
        <ErrorState error={policies.error} onRetry={() => void policies.refetch()} />
      ) : enabledPolicies.length === 0 ? (
        <section className="feedback-state managed-data-empty">
          <Database aria-hidden="true" />
          <strong>没有可用的 Managed Table</strong>
          <span>请先为真实数据库表创建并启用完整的表规则。</span>
          <Link className="button button-primary" to="/platform/table-policies">前往表规则分配</Link>
        </section>
      ) : (
        <>
          <section className="managed-data-toolbar" aria-label="Managed Table 选择">
            <Label className="field managed-table-select">
              <span>Managed Table</span>
              <NativeSelect
                aria-label="Managed Table"
                value={selectedTable}
                onChange={(event) => {
                  const target = event.target.value;
                  if (target === selectedTable) return;
                  protection.requestLeave(() => {
                  changes.send({ type: "cancel-pending" });
                  setRequestedTable(target);setSelectedRows(new Map());
                  setExpandedPolicyTable(undefined);
                  setQuerySpec(initialQuerySpec);
                  setConditions([]);
                  setOrderField("");
                  setOrderDirection("DESC");
                  setPageSize("");
                  setValidationError(null);
                  });
                }}
              >
                {enabledPolicies.map((policy) => <option key={policy.tableName} value={policy.tableName}>{policy.tableName}</option>)}
              </NativeSelect>
            </Label>
            <Button
              variant="secondary"
              icon={policyDetailsOpen ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
              aria-expanded={policyDetailsOpen}
              aria-controls={policyDetailsId}
              onClick={() => setExpandedPolicyTable(policyDetailsOpen ? undefined : selectedTable)}
            >{policyDetailsOpen ? "收起当前表规则能力" : "查看当前表规则能力"}</Button>
            <span className="managed-table-status"><i className="ready-dot" />已启用表规则</span>
          </section>

          {selectedPolicy && (
            <section id={policyDetailsId} className="managed-policy-effects" aria-label="当前表规则能力" hidden={!policyDetailsOpen}>
              <div className="form-section-heading">
                <h2>当前表规则能力</h2>
                <p>查询与变更都按每次请求读取到的规则和实时表结构执行。</p>
              </div>
              {queryPolicy.data ? <QueryPolicyEffect policy={queryPolicy.data} registeredTypes={queryPolicyTypes.data} registryState={queryRegistryState} heading="查询" /> : <div className="policy-effect policy-effect-unconfirmed"><strong>无法确认查询能力</strong><p>{queryPolicy.isError ? "查询规则详情加载失败。" : "正在读取查询规则详情。"} 当前界面不会猜测规则效果。</p></div>}
              {mutationPolicy ? <MutationPolicyEffect policy={mutationPolicy} registeredTypes={mutationRegistry} registryState={mutationRegistryState} heading="变更" /> : <div className="policy-effect policy-effect-unconfirmed"><strong>无法确认变更能力</strong><p>正在读取或未能读取变更规则详情，当前界面不会猜测规则效果。</p></div>}
              {result.data && <p className="schema-effect">本次实时表结构确认了 {result.data.columns.length} 列：{result.data.columns.map((column) => column.name).join("、")}。</p>}
            </section>
          )}

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
                    <Label className="field">
                      <span>字段</span>
                      <NativeSelect
                        aria-label={`条件 ${index + 1} 字段`}
                        value={condition.field}
                        onChange={(event) => {
                          const nextColumn = result.data.columns.find((item) => item.name === event.target.value)!;
                          const nextOperators = allowedOperators(nextColumn);
                          update({ field: event.target.value, operator: nextOperators.includes(condition.operator) ? condition.operator : "exact" });
                        }}
                      >{result.data.columns.map((column) => <option key={column.name} value={column.name}>{column.name}</option>)}</NativeSelect>
                    </Label>
                    <Label className="field">
                      <span>操作符</span>
                      <NativeSelect aria-label={`条件 ${index + 1} 操作符`} value={condition.operator} onChange={(event) => update({ operator: event.target.value as QueryOperator })}>
                        {operators.map((operator) => <option key={operator} value={operator}>{queryOperatorLabels[operator]}</option>)}
                      </NativeSelect>
                    </Label>
                    <div className="field condition-value">
                      <span>值</span>
                      <ConditionValueEditor index={index + 1} condition={condition} column={column} update={update} />
                    </div>
                    <Button variant="ghost"
                      className="icon-button condition-remove"
                      aria-label={`删除条件 ${index + 1}`}
                      onClick={() => setConditions((current) => current.filter((_, itemIndex) => itemIndex !== index))}
                    ><Trash2 size={16} /></Button>
                  </fieldset>
                )})}</div>
              )}
              <div className="query-options">
                <Label className="field">
                  <span>排序字段</span>
                  <NativeSelect aria-label="排序字段" value={orderField} onChange={(event) => setOrderField(event.target.value)}>
                    <option value="">使用查询规则默认排序</option>
                    {result.data.columns.map((column) => <option key={column.name} value={column.name}>{column.name}</option>)}
                  </NativeSelect>
                </Label>
                <Label className="field">
                  <span>排序方向</span>
                  <NativeSelect aria-label="排序方向" value={orderDirection} disabled={!orderField} onChange={(event) => setOrderDirection(event.target.value as "ASC" | "DESC")}>
                    <option value="ASC">ASC</option>
                    <option value="DESC">DESC</option>
                  </NativeSelect>
                </Label>
                <Label className="field">
                  <span>每页数量</span>
                  <Input aria-label="每页数量" type="number" min="1" max={queryPolicy.data?.maxPageSize ?? 200} placeholder={queryPolicy.data ? `规则默认 ${queryPolicy.data.defaultPageSize}，最多 ${queryPolicy.data.maxPageSize}` : `规则默认（当前 ${result.data.page.pageSize}）`} value={pageSize} onChange={(event) => setPageSize(event.target.value)} />
                </Label>
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
                {canEdit&&<div className="flex items-center gap-3 p-4"><p>已明确选择 {selectedRows.size} 项</p><Button disabled={!selectedRows.size||Boolean(capabilityReasons.DELETE)} onClick={()=>{setBatchPage(0);setBatchReview(true)}}>删除已选 {selectedRows.size} 项</Button><Button disabled={!selectedRows.size} onClick={()=>setSelectedRows(new Map())}>清空选择</Button></div>}
                <div className="table-scroll">
                  <Table className="policy-table managed-data-table">
                    <TableHeader><TableRow>{canEdit&&<TableHead>选择</TableHead>}{result.data.columns.map((column) => (
                      <TableHead key={column.name} scope="col">
                        <strong>{column.name}</strong>
                        <small>{column.type} · {column.nullable ? "可为 NULL" : "非 NULL"}</small>
                      </TableHead>
                    ))}<TableHead scope="col">操作</TableHead></TableRow></TableHeader>
                    <TableBody>{result.data.rows.length === 0 ? (
                      <TableRow><TableCell className="managed-data-no-rows" colSpan={result.data.columns.length + 1}>没有符合条件的配置内容</TableCell></TableRow>
                    ) : result.data.rows.map((row, rowIndex) => (
                      <TableRow key={String(row.id ?? rowIndex)}>{canEdit&&<TableCell><Checkbox aria-label={`选择记录 ${row.id??"未知"}`} checked={typeof row.id==="string"&&selectedRows.has(row.id)} disabled={typeof row.id!=="string"||Boolean(capabilityReasons.DELETE)||(!selectedRows.has(row.id)&&selectedRows.size>=1000)} onCheckedChange={checked=>{const id=row.id;if(typeof id!=="string")return;setSelectedRows(current=>{const next=new Map(current);if(checked===true)next.set(id,{id,version:result.data.recordVersions[rowIndex]!});else next.delete(id);return next})}}/></TableCell>}{result.data.columns.map((column) => (
                        <TableCell key={column.name}><CellValue value={row[column.name] ?? null} /></TableCell>
                      ))}<TableCell className="managed-data-actions">
                        <Button variant="ghost" icon={<Pencil size={14} />} aria-label={`修改记录 ${row.id ?? "未知"}`} disabled={typeof row.id !== "string" || Boolean(capabilityReasons.MODIFY)} title={typeof row.id !== "string" ? "记录缺少可用的 id" : capabilityReasons.MODIFY} aria-describedby={capabilityReasons.MODIFY ? "mutation-modify-reason" : undefined} onClick={() => send({ type: "open-editor", operation: "MODIFY", row, expectedVersion: result.data.recordVersions[rowIndex] })}>修改</Button>
                        <Button variant="ghost" icon={<Trash2 size={14} />} aria-label={`删除记录 ${row.id ?? "未知"}`} disabled={typeof row.id !== "string" || Boolean(capabilityReasons.DELETE)} title={typeof row.id !== "string" ? "记录缺少可用的 id" : capabilityReasons.DELETE} aria-describedby={capabilityReasons.DELETE ? "mutation-delete-reason" : undefined} onClick={() => send({ type: "review-delete", row, expectedVersion: result.data.recordVersions[rowIndex] })}>删除</Button>
                      </TableCell></TableRow>
                    ))}</TableBody>
                  </Table>
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
          <section className="mutation-capability-notes" aria-label="变更规则权限">
            {(["ADD", "MODIFY", "DELETE"] as const).map((operation) => capabilityReasons[operation] && <span id={`mutation-${operation.toLowerCase()}-reason`} key={operation}>{capabilityReasons[operation]}</span>)}
          </section>

        </>
      )}
          {editor && <ManagedRowEditor

            recordConflict={changes.view.recordConflict||(draftWrite.error instanceof ApiError&&draftWrite.error.code==="record_version_conflict")}
            latest={changes.view.latest}
            onInspectLatest={() => changes.send({ type: "inspect-latest" })}
            onRebuildLatest={() => {changes.send({ type: "rebuild-latest" });draftWrite.confirmRebuild();draftWrite.clearError()}}
            key={editor?.sequence}
            open={Boolean(editor) && !changeSet}
            error={draftWrite.error}
            tableName={editor.tableName}
            operation={editor.operation}
            columns={editor.columns}
            original={editor.row}
            autoFillFields={new Set(editor.allAutoFillFields)}
            reviewDisabled={changes.view.reviewDisabled}
            recheckError={changes.view.recheckError}
            onRetryRecheck={() => changes.send({ type: "retry-recheck" })}
            onClose={() => send({ type: "cancel-pending" })}
            onReview={(content) => changes.send({ type: "review-content", content })}
          />}
          <ChangeSetDialog
            draftAction={<Button disabled={!canEdit||!destination.valid||changes.view.reviewDisabled||saving||draftWrite.pending||draftRecordConflict} onClick={()=>{if(changes.view.draftInput)void saveDraft(changes.view.draftInput)}}>{draftWrite.pending?"正在保存草稿…":draftWrite.unresolved?"使用原请求重试":"确认并保存草稿"}</Button>}
            draftLocked={draftWrite.pending||draftWrite.unresolved}
            draftFeedback={<>{destination.picker(saving||draftWrite.pending||draftWrite.unresolved)}{draftWrite.unresolved&&<p role="alert">草稿保存结果待确认。原请求已保留，刷新后仍可找回。</p>}</>}

            recordConflict={changes.view.recordConflict||(draftWrite.error instanceof ApiError&&draftWrite.error.code==="record_version_conflict")}
            latest={changes.view.latest}
            onInspectLatest={() => changes.send({ type: "inspect-latest" })}
            onRebuildLatest={() => {changes.send({ type: "rebuild-latest" });draftWrite.confirmRebuild();draftWrite.clearError()}}
            changeSet={changeSet}
            error={changes.view.recheckError || draftWrite.error}
            pending={draftWrite.pending || changes.view.recheckingChange}
            confirmDisabled={changes.view.reviewDisabled||draftRecordConflict}
            onRetryRecheck={changes.view.recheckError ? () => changes.send({ type: "retry-recheck" }) : undefined}
            onEdit={() => changes.send({ type: "edit-pending" })}
            onCancel={() => send({ type: "cancel-pending" })}
          />
    {batchReview&&<Drawer open eyebrow="发布草稿" title="删除所选记录" onClose={()=>{if(!saving&&!draftWrite.pending&&!draftWrite.unresolved)setBatchReview(false)}} footer={<><Button disabled={saving||draftWrite.pending||draftWrite.unresolved} onClick={()=>setBatchReview(false)}>取消删除</Button><Button disabled={!canEdit||!destination.valid||Boolean(capabilityReasons.DELETE)||saving||draftWrite.pending} onClick={()=>void saveDraft({table_name:selectedTable,items:Array.from(selectedRows.values()).map(row=>({operation:"DELETE",id:row.id,expected_record_version:row.version,content:{}}))})}>{draftWrite.unresolved?"使用原请求重试":"确认并保存草稿"}</Button></>}><p>将所选 {selectedRows.size} 项加入同表草稿。现在不会删除配置。</p><fieldset disabled={saving||draftWrite.pending||draftWrite.unresolved}><ReleaseItemPager count={selectedRows.size} page={batchPage} onPage={setBatchPage} label="待删除明细"/></fieldset><ol start={batchPage*releasePageSize+1}>{Array.from(selectedRows.values()).slice(batchPage*releasePageSize,(batchPage+1)*releasePageSize).map((row,index)=><li key={row.id}>明细 {batchPage*releasePageSize+index+1} · 记录 {row.id} · 记录基线 {row.version}</li>)}</ol>{destination.picker(saving||draftWrite.pending||draftWrite.unresolved)}{Boolean(draftWrite.error)&&<ErrorState error={draftWrite.error}/>}</Drawer>}
    </main>
  );
}
