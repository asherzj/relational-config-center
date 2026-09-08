import type {ReactNode} from "react";
import { RecordConflictReview, type RecordConflictReviewProps } from "./RecordConflictReview";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";
import { ErrorState } from "../../components/ui/Feedback";
import { Button } from "../../components/ui/Button";
import type { ChangeSet, ChangeSetCell } from "./model";

import { ModalSurface } from "../../components/ui/ModalSurface";
import { DialogTitle } from "../../components/shadcn/dialog";

function Cell({ cell, autoFill }: { cell: ChangeSetCell; autoFill?: boolean }) {
  const content = cell.state === "value" ? cell.value
    : cell.state === "null" ? "NULL"
      : cell.state === "empty" ? '\"\"'
        : cell.state === "unsubmitted" ? "未提交"
          : "不存在";
  return <><span className={`change-cell-value cell-${cell.state}`}>{content}</span>{autoFill && cell.state === "unsubmitted" && <small className="auto-fill-label">Auto Fill</small>}</>;
}

type Props = RecordConflictReviewProps & {
  changeSet: ChangeSet | null;
 draftAction?:ReactNode;
 draftFeedback?:ReactNode;
 draftLocked?:boolean;
  error?: unknown;
  pending: boolean;
  confirmDisabled?: boolean;
  onEdit: () => void;
  onCancel: () => void;
  onRetryRecheck?: () => void;
};

export function ChangeSetDialog({ changeSet, error, pending, onEdit, onCancel, onRetryRecheck, draftAction,draftFeedback,draftLocked,...conflictReview }: Props) {
  const operation = changeSet?.operation;
  if (!changeSet) return null;
  return (
    <ModalSurface open onClose={onCancel} pending={pending||draftLocked} label={`${changeSet.operation} Change Set`} dismissLabel="取消 Change Set" className={`change-set-dialog change-set-${changeSet.operation.toLowerCase()} gap-0 overflow-hidden p-0 sm:max-w-[980px]`}>
        <header><span>{operation === "DELETE" ? "尚未执行删除；取消删除会直接关闭此预览。" : "请确认以下变更内容："}</span><DialogTitle>{changeSet.operation} Change Set</DialogTitle></header>
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
        </div>
        <div className="change-set-feedback">
        {error !== undefined && error !== null && <ErrorState error={error} onRetry={onRetryRecheck} />}
        {draftFeedback}
 <RecordConflictReview {...conflictReview} pending={pending} />
        </div>
        <footer>
            <Button onClick={onCancel} disabled={pending||draftLocked}>{operation === "DELETE" ? "取消删除" : "放弃本次编辑"}</Button>
            {operation !== "DELETE" && <Button onClick={onEdit} disabled={pending||draftLocked}>返回修改</Button>}
            {draftAction}
        </footer>
    </ModalSurface>
  );
}
