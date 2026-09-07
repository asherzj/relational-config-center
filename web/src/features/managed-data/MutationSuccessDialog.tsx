

import { Button } from "../../components/ui/Button";
import { ErrorState } from "../../components/ui/Feedback";
import { ModalSurface } from "../../components/ui/ModalSurface";
import { DialogTitle } from "../../components/shadcn/dialog";
import type { ManagedDataMutationOutcome } from "./model";

function FinalValue({ value }: { value: string | null }) {
  if (value === null) return <span className="cell-state cell-null">NULL</span>;
  if (value === "") return <span className="cell-state cell-empty">空字符串</span>;
  return <span>{value}</span>;
}

export function MutationSuccessDialog({ outcome, retryPending, onRetry, onClose }: { outcome: ManagedDataMutationOutcome | null; retryPending: boolean; onRetry: () => void; onClose: () => void }) {
  if (!outcome) return null;
  const readbackIncomplete = outcome.operation !== "DELETE" && !outcome.row;
  return (
    <ModalSurface open onClose={onClose} pending={retryPending} label={`${outcome.operation} 写入结果`} dismissLabel="关闭写入结果" className="mutation-success-dialog gap-3 p-0 sm:max-w-[760px]">
        <DialogTitle>{readbackIncomplete ? `${outcome.operation} 已执行，回查未完成` : `${outcome.operation} 已完成`}</DialogTitle>
        {outcome.operation === "DELETE" ? <p>已删除记录 id：<strong>{outcome.id}</strong></p> : (
          readbackIncomplete ? <><p>写入请求已经成功返回，Web 不会重复执行写入。请仅重试 exact id 回查。</p>{outcome.retrievalError !== undefined && <ErrorState error={outcome.retrievalError} />}</>
            : <><p>以下为通过 exact id 回查得到的数据库最终值。</p><dl>{outcome.columns?.map((column) => <div key={column.name}><dt>{column.name}</dt><dd><FinalValue value={outcome.row?.[column.name] ?? null} /></dd></div>)}</dl></>
        )}
        <footer>{readbackIncomplete && <Button variant="primary" disabled={retryPending} onClick={onRetry}>{retryPending ? "正在回查…" : "重新回查"}</Button>}<Button disabled={retryPending} onClick={onClose}>关闭</Button></footer>
    </ModalSurface>
  );
}
