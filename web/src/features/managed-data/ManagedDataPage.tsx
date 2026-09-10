import {type DraftContentInput} from "../../api/release-orders";
import {ReleaseItemPager,releasePageSize} from "../release-orders/ReleaseItemPager";
import {useDraftDestination} from "../release-orders/useDraftDestination";
import {Drawer} from "../../components/ui/Drawer";
import {useReleaseWrite} from "../release-orders/useReleaseWrite";
import {ReleaseConflictReview} from "../release-orders/ReleaseRequestReview";
import {ApiError} from "../../api/client";
import { useAccountRole } from "../accounts/roles";
import { CombinedQueryForm } from "./CombinedQueryForm";
import { Checkbox } from "../../components/shadcn/checkbox";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Label } from "../../components/shadcn/label";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Database, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { Button } from "../../components/ui/Button";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useTablePolicies } from "../table-policies/queries";
import { useQueryPolicy, useQueryPolicyTypes } from "../query-policies/queries";
import { MutationPolicyEffect, QueryPolicyEffect } from "../policies/PolicyEffect";
import { type ManagedDataColumn, type QuerySpec } from "./model";
import { useManagedDataQuery } from "./queries";
import { useManagedDataMutationWorkflow, type ManagedDataMutationIntent } from "./mutation-workflow";
import { ConfiguredRowEditor } from "./ConfiguredRowEditor";
import { ChangeSetDialog } from "./ChangeSetDialog";
import { CurrentFieldDisplayStatus, CurrentFieldName, CurrentFieldValue, orderDisplayedFields, useCurrentFieldDisplay } from "../field-display/CurrentFieldDisplay";

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

export function ManagedDataPage() {
  const canEdit = useAccountRole("EDITOR");
 const draftWrite=useReleaseWrite("create");
 const navigate=useNavigate();
 const [search]=useSearchParams();
 const [selectedRows,setSelectedRows]=useState<Map<string,{id:string;version:string;row:Record<string,string|null>}>>(()=>new Map());
 const [batchReview,setBatchReview]=useState(false);
 const [batchPage,setBatchPage]=useState(0);
 const [saving,setSaving]=useState(false);
  const policies = useTablePolicies();
  const [requestedTable, setRequestedTable] = useState(search.get("table_name")??"");
  const [expandedPolicyTable, setExpandedPolicyTable] = useState<string>();
  const policyDetailsId = useId();
  const [querySpec, setQuerySpec] = useState<QuerySpec>(initialQuerySpec);
  const enabledPolicies = (policies.data ?? []).filter((policy) => policy.enabled);
  const selectedTable = enabledPolicies.some((policy) => policy.tableName === requestedTable)
    ? requestedTable
    : enabledPolicies[0]?.tableName ?? "";
  const destination=useDraftDestination(selectedTable,search.get("draft")??"");
  const result = useManagedDataQuery(selectedTable, querySpec);
  const currentDisplay = useCurrentFieldDisplay(selectedTable, Boolean(result.data));
  const [displayReadyTable,setDisplayReadyTable]=useState("");
  useEffect(()=>{if(currentDisplay.display.configuration)setDisplayReadyTable(selectedTable)},[currentDisplay.display.configuration,selectedTable]);
  const [queryColumns, setQueryColumns] = useState<{table: string; columns: ManagedDataColumn[]}>();
  useEffect(() => { if (result.data) setQueryColumns({table: selectedTable, columns: result.data.columns}); }, [selectedTable, result.data]);
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
   const matchesInput=!draftWrite.unresolved||Boolean(draftWrite.storedRequest&&destination.matchesInput(draftWrite.storedRequest,input));
   const request=draftWrite.unresolved?undefined:await destination.prepare(input);
   const saved=draftWrite.unresolved?await draftWrite.retry():request?await draftWrite.send({...request,label:`保存 ${selectedTable} 草稿`}):undefined;
   if(saved&&matchesInput)protection.afterSave(()=>{changes.send({type:"cancel-pending"});setBatchReview(false);setSelectedRows(new Map());navigate(`/configuration/release-orders/${saved.id}`)});
  }finally{setSaving(false)}
 };

  return (
    <main className="workspace managed-data-workspace">
      <ReleaseConflictReview scopeFilter="create"/>
 <div className="page-heading">
        <div>
          <h1>配置内容管理</h1>
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

          {queryColumns?.table === selectedTable && <CombinedQueryForm key={selectedTable} tableName={selectedTable} columns={queryColumns.columns} maxPageSize={queryPolicy.data?.maxPageSize} defaultPageSize={queryPolicy.data?.defaultPageSize}
            openingConfigurationSource={{configuration:currentDisplay.display.configuration,error:currentDisplay.error,retry:currentDisplay.retry}}
            onSubmit={setQuerySpec} onClear={() => { const needsRefetch = isInitialQuerySpec(querySpec); setQuerySpec(initialQuerySpec); if (needsRefetch) void result.refetch(); }} />}


          <section className="catalog managed-data-results" aria-label="Managed Data 查询结果">
            {result.isPending ? <LoadingState label="正在查询 Managed Table…" /> : result.isError ? (
              <ErrorState error={result.error} onRetry={() => void result.refetch()} />
            ) : (
              <>
                {displayReadyTable===selectedTable&&<CurrentFieldDisplayStatus pending={currentDisplay.pending} error={currentDisplay.error} onRetry={()=>void currentDisplay.retry()}/>}
                {canEdit&&<div className="flex items-center gap-3 p-4"><p>已明确选择 {selectedRows.size} 项</p><Button disabled={!selectedRows.size||Boolean(capabilityReasons.DELETE)} onClick={()=>{setBatchPage(0);setBatchReview(true)}}>删除已选 {selectedRows.size} 项</Button><Button disabled={!selectedRows.size} onClick={()=>setSelectedRows(new Map())}>清空选择</Button></div>}
                <div className="table-scroll">
                  <Table className="policy-table managed-data-table">
                    <TableHeader><TableRow>{canEdit&&<TableHead>选择</TableHead>}{orderDisplayedFields(currentDisplay.display, result.data.columns, column => column.name, true).map((column) => (
                      <TableHead key={column.name} scope="col">
                        <CurrentFieldName display={currentDisplay.display} name={column.name} detail={`${column.type} · ${column.nullable ? "可为 NULL" : "非 NULL"}`} />
                      </TableHead>
                    ))}<TableHead scope="col">操作</TableHead></TableRow></TableHeader>
                    <TableBody>{result.data.rows.length === 0 ? (
                      <TableRow><TableCell className="managed-data-no-rows" colSpan={orderDisplayedFields(currentDisplay.display,result.data.columns,column=>column.name,true).length+1+(canEdit?1:0)}>没有符合条件的配置内容</TableCell></TableRow>
                    ) : result.data.rows.map((row, rowIndex) => (
                      <TableRow key={String(row.id ?? rowIndex)}>{canEdit&&<TableCell><Checkbox aria-label={`选择记录 ${row.id??"未知"}`} checked={typeof row.id==="string"&&selectedRows.has(row.id)} disabled={typeof row.id!=="string"||Boolean(capabilityReasons.DELETE)||(!selectedRows.has(row.id)&&selectedRows.size>=1000)} onCheckedChange={checked=>{const id=row.id;if(typeof id!=="string")return;setSelectedRows(current=>{const next=new Map(current);if(checked===true)next.set(id,{id,version:result.data.recordVersions[rowIndex]!,row});else next.delete(id);return next})}}/></TableCell>}{orderDisplayedFields(currentDisplay.display, result.data.columns, column => column.name, true).map((column) => (
                        <TableCell key={column.name}><CurrentFieldValue display={currentDisplay.display} name={column.name} value={row[column.name] ?? null}><CellValue value={row[column.name] ?? null} /></CurrentFieldValue></TableCell>
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
          {editor && <ConfiguredRowEditor

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
            onReview={(content, snapshot) => changes.send({ type: "review-content", content, snapshot })}
          />}
          <ChangeSetDialog
            fieldDisplay={currentDisplay.display}
            draftAction={<Button variant="primary" disabled={!canEdit||!destination.valid||changes.view.reviewDisabled||saving||draftWrite.pending||draftRecordConflict} onClick={()=>{if(changes.view.draftInput)void saveDraft(changes.view.draftInput)}}>{draftWrite.pending?"正在保存草稿…":"确认并保存草稿"}</Button>}
            draftFeedback={<>{destination.picker(saving||draftWrite.pending||draftWrite.unresolved)}{draftWrite.unresolved&&<p role="alert">原请求已保留；再次保存将提交同一份草稿。</p>}</>}

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
    {batchReview&&<Drawer open eyebrow="发布草稿" title="删除所选记录" onClose={()=>{if(!saving&&!draftWrite.pending)protection.requestLeave(()=>setBatchReview(false))}} footer={<><Button disabled={saving||draftWrite.pending} onClick={()=>protection.requestLeave(()=>setBatchReview(false))}>取消删除</Button><Button disabled={!canEdit||!destination.valid||Boolean(capabilityReasons.DELETE)||saving||draftWrite.pending} onClick={()=>void saveDraft({items:Array.from(selectedRows.values()).map(row=>({table_name:selectedTable,operation:"DELETE",id:row.id,expected_record_version:row.version,content:{}}))})}>{"确认并保存草稿"}</Button></>}><p>将所选 {selectedRows.size} 项加入发布草稿。现在不会删除配置。</p><fieldset disabled={saving||draftWrite.pending}><ReleaseItemPager count={selectedRows.size} page={batchPage} onPage={setBatchPage} label="待删除明细"/></fieldset><ol start={batchPage*releasePageSize+1}>{Array.from(selectedRows.values()).slice(batchPage*releasePageSize,(batchPage+1)*releasePageSize).map((selected,index)=><li key={selected.id}><details><summary>明细 {batchPage*releasePageSize+index+1} · 记录 {selected.id} · 记录基线 {selected.version}</summary><dl>{orderDisplayedFields(currentDisplay.display,result.data?.columns??[],column=>column.name).map(column=><div key={column.name} className="border-b py-2"><dt><CurrentFieldName display={currentDisplay.display} name={column.name} detail={column.type}/></dt><dd><CurrentFieldValue display={currentDisplay.display} name={column.name} value={selected.row[column.name]??null}><CellValue value={selected.row[column.name]??null}/></CurrentFieldValue></dd></div>)}</dl></details></li>)}</ol>{destination.picker(saving||draftWrite.pending||draftWrite.unresolved)}{Boolean(draftWrite.error)&&<ErrorState error={draftWrite.error}/>}</Drawer>}
    </main>
  );
}
