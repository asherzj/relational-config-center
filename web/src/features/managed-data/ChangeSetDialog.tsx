import { ErrorState } from "../../components/ui/Feedback";
import { Button } from "../../components/ui/Button";
import type { ChangeSet, ChangeSetCell } from "./model";
import { useRef } from "react";
import { useModalFocus } from "../../components/ui/useModalFocus";

function Cell({ cell, autoFill }: { cell: ChangeSetCell; autoFill?: boolean }) {
  const content = cell.state === "value" ? cell.value
    : cell.state === "null" ? "NULL"
      : cell.state === "empty" ? '""'
        : cell.state === "unsubmitted" ? "未提交"
          : "不存在";
  return <><span className={`change-cell-value cell-${cell.state}`}>{content}</span>{autoFill && cell.state === "unsubmitted" && <small className="auto-fill-label">Auto Fill</small>}</>;
}

type Props = {
  changeSet: ChangeSet | null;
  error?: unknown;
  pending: boolean;
  onEdit: () => void;
  onCancel: () => void;
  onConfirm: () => void;
};

export function ChangeSetDialog({ changeSet, error, pending, onEdit, onCancel, onConfirm }: Props) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const operation = changeSet?.operation;
  useModalFocus({ open: Boolean(operation), dialogRef, onEscape: pending ? undefined : onCancel });
  if (!changeSet) return null;
  return (
    <div className="modal-layer change-set-layer">
      <button className="drawer-scrim" aria-label="取消 Change Set" disabled={pending} onClick={onCancel} />
      <div ref={dialogRef} tabIndex={-1} className={`change-set-dialog change-set-${changeSet.operation.toLowerCase()}`} role="dialog" aria-modal="true" aria-label={`${changeSet.operation} Change Set`} data-modal-surface="true">
        <header><span>请确认以下变更内容：</span><h2>{changeSet.operation} Change Set</h2></header>
        <div className="change-set-scroll">
          <table className="change-set-table">
            <thead><tr><th scope="col">字段</th><th scope="col">原值</th><th scope="col">新值</th></tr></thead>
            <tbody>{changeSet.rows.map((row) => (
              <tr key={row.field} className={row.changed ? "change-row-changed" : "change-row-unchanged"}>
                <th scope="row">{row.field}{row.changed && <small>变化</small>}</th>
                <td className={row.changed ? "change-original" : ""}><Cell cell={row.original} /></td>
                <td className={row.changed ? "change-next" : ""}><Cell cell={row.next} autoFill={row.autoFill} /></td>
              </tr>
            ))}</tbody>
          </table>
        </div>
        {error !== undefined && error !== null && <ErrorState error={error} />}
        <footer>
          <Button onClick={onCancel} disabled={pending}>取消</Button>
          <Button onClick={onEdit} disabled={pending}>{changeSet.operation === "DELETE" ? "返回" : "返回修改"}</Button>
          <Button variant={changeSet.operation === "DELETE" ? "danger" : "primary"} onClick={onConfirm} disabled={pending}>{pending ? "正在执行…" : "确认并执行"}</Button>
        </footer>
      </div>
    </div>
  );
}
