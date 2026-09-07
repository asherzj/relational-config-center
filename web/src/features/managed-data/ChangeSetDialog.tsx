import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";
import { ErrorState } from "../../components/ui/Feedback";
import { Button } from "../../components/ui/Button";
import type { ChangeSet, ChangeSetCell } from "./model";
import { isUncertainWriteError } from "../../api/client";

import { ModalSurface } from "../../components/ui/ModalSurface";
import { DialogTitle } from "../../components/shadcn/dialog";
import { WriteRecovery } from "../../components/ui/WriteRecovery";

function Cell({ cell, autoFill }: { cell: ChangeSetCell; autoFill?: boolean }) {
  const content = cell.state === "value" ? cell.value
    : cell.state === "null" ? "NULL"
      : cell.state === "empty" ? '\"\"'
        : cell.state === "unsubmitted" ? "未提交"
          : "不存在";
  return <><span className={`change-cell-value cell-${cell.state}`}>{content}</span>{autoFill && cell.state === "unsubmitted" && <small className="auto-fill-label">Auto Fill</small>}</>;
}

type Props = {
  changeSet: ChangeSet | null;
  error?: unknown;
  recheckError?: unknown;
  pending: boolean;
  confirmDisabled?: boolean;
  onEdit: () => void;
  onCancel: () => void;
  onConfirm: () => void;
  onCheck: () => Promise<unknown>;
  onResume: () => void;
  onRetryRecheck?: () => void;
};

export function ChangeSetDialog({ changeSet, error, recheckError, pending, confirmDisabled, onEdit, onCancel, onConfirm, onCheck, onResume, onRetryRecheck }: Props) {
  const operation = changeSet?.operation;
  const uncertain = isUncertainWriteError(error);
  if (!changeSet) return null;
  return (
    <ModalSurface open onClose={onCancel} pending={pending} label={`${changeSet.operation} Change Set`} dismissLabel="取消 Change Set" className={`change-set-dialog change-set-${changeSet.operation.toLowerCase()} gap-0 overflow-hidden p-0 sm:max-w-[980px]`}>
        <header><span>{pending ? "请求正在执行，请稍候。" : uncertain ? "请求已发出，执行结果尚未确认。" : operation === "DELETE" ? "尚未执行删除；取消删除会直接关闭此预览。" : "请确认以下变更内容："}</span><DialogTitle>{changeSet.operation} Change Set</DialogTitle></header>
        <div className="change-set-scroll">
          <Table className="change-set-table">
            <TableHeader><TableRow><TableHead scope="col">字段</TableHead><TableHead scope="col">原值</TableHead><TableHead scope="col">新值</TableHead></TableRow></TableHeader>
            <TableBody>{changeSet.rows.map((row) => (
              <TableRow key={row.field} className={row.changed ? "change-row-changed" : "change-row-unchanged"}>
                <TableHead scope="row">{row.field}{row.changed && <small>变化</small>}</TableHead>
                <TableCell className={row.changed ? "change-original" : ""}><Cell cell={row.original} /></TableCell>
                <TableCell className={row.changed ? "change-next" : ""}><Cell cell={row.next} autoFill={row.autoFill} /></TableCell>
              </TableRow>
            ))}</TableBody>
          </Table>
          <WriteRecovery onResume={onResume} resumeLabel={operation === "DELETE" ? "我已核对，关闭并重新查询" : "我已核对，返回修改"} error={error} onCheck={onCheck}>{operation === "ADD" && <p>如果新增记录的 id 未返回，只能查询当前第一页供核对，不能据此判断新增失败或推测自增 id。</p>}</WriteRecovery>
          {!uncertain && error !== undefined && error !== null && <ErrorState error={error} />}
          {recheckError !== undefined && recheckError !== null && <ErrorState error={recheckError} onRetry={onRetryRecheck} />}
        </div>
        <footer>
          <Button onClick={onCancel} disabled={pending}>{uncertain ? "关闭本次预览" : operation === "DELETE" ? "取消删除" : "放弃本次编辑"}</Button>
          {operation !== "DELETE" && <Button onClick={onEdit} disabled={pending || uncertain}>返回修改</Button>}
          <Button variant={changeSet.operation === "DELETE" ? "danger" : "primary"} onClick={onConfirm} disabled={pending || confirmDisabled || uncertain}>{pending ? "正在执行…" : "确认并执行"}</Button>
        </footer>
    </ModalSurface>
  );
}
