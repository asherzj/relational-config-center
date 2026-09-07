import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { useBlocker } from "react-router-dom";
import { Button } from "./Button";
import { useModalFocus } from "./useModalFocus";

type DraftStatus = { dirty: boolean; pending: boolean };
type Protection = {
  register: (status: RefObject<DraftStatus>) => () => void;
  requestLeave: (action: () => void) => void;
  afterSave: (action: () => void) => void;
  submissionSettled: () => void;
};
const Context = createContext<Protection | null>(null);

function LeaveDialog({ busy, onStay, onLeave }: { busy: boolean; onStay: () => void; onLeave: () => void }) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const stayRef = useRef<HTMLButtonElement>(null);
  useModalFocus({ open: true, dialogRef, initialFocusRef: stayRef, onEscape: onStay });
  return createPortal(
    <div className="modal-layer leave-protection-layer">
      <button className="drawer-scrim" aria-label="关闭离开提示" onClick={onStay} />
      <div ref={dialogRef} tabIndex={-1} className="confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="leave-title" aria-describedby="leave-description" data-modal-surface="true">
        <h2 id="leave-title">{busy ? "正在提交，请稍候" : "放弃未保存的修改？"}</h2>
        <p id="leave-description">{busy ? "请求已发出，请等待结果后再离开。此次离开已取消，不会重复提交。" : "离开后，本次输入和字段选择将丢失。继续编辑可保留当前内容。"}</p>
        <div className="confirm-actions">
          <Button ref={stayRef} onClick={onStay}>{busy ? "留在此页" : "继续编辑"}</Button>
          {!busy && <Button variant="danger" onClick={onLeave}>放弃修改并离开</Button>}
        </div>
      </div>
    </div>, document.body,
  );
}

/** One router blocker handles all active drafts, including browser POP navigation. */
export function LeaveProtectionProvider({ children }: { children: ReactNode }) {
  const statuses = useRef(new Set<RefObject<DraftStatus>>());
  const bypass = useRef(false);
  const [localLeave, setLocalLeave] = useState<(() => void) | null>(null);
  const [busyNotice, setBusyNotice] = useState(false);
  const status = useCallback(() => ({
    dirty: [...statuses.current].some((item) => item.current.dirty),
    pending: [...statuses.current].some((item) => item.current.pending),
  }), []);
  const blocker = useBlocker(() => !bypass.current && (status().dirty || status().pending));
  const submissionSettled = useCallback(() => setBusyNotice(false), []);
  const afterSave = useCallback((action: () => void) => {
    setBusyNotice(false);
    // React Router evaluates its public blocker synchronously during navigate().
    bypass.current = true;
    try { action(); } finally { bypass.current = false; }
  }, []);
  const register = useCallback((entry: RefObject<DraftStatus>) => {
    statuses.current.add(entry);
    return () => { statuses.current.delete(entry); };
  }, []);
  const requestLeave = useCallback((action: () => void) => {
    const current = status();
    if (current.pending) { setBusyNotice(true); return; }
    if (current.dirty) setLocalLeave(() => action);
    else action();
  }, [status]);
  useEffect(() => {
    if (blocker.state === "blocked" && status().pending) {
      // Never queue a navigation behind a write: a completed write must show its outcome.
      blocker.reset();
      setBusyNotice(true);
    }
  }, [blocker, status]);
  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      const current = status();
      if (!current.dirty && !current.pending) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [status]);
  const stay = () => {
    setLocalLeave(null);
    setBusyNotice(false);
    if (blocker.state === "blocked") blocker.reset();
  };
  const leave = () => {
    if (status().pending) { stay(); setBusyNotice(true); return; }
    const action = localLeave;
    setLocalLeave(null);
    if (blocker.state === "blocked") blocker.proceed();
    else if (action) afterSave(action);
  };
  const value = useMemo(() => ({ register, requestLeave, afterSave, submissionSettled }), [register, requestLeave, afterSave, submissionSettled]);
  return <Context.Provider value={value}>{children}{(localLeave || blocker.state === "blocked" || busyNotice) && <LeaveDialog busy={busyNotice || status().pending} onStay={stay} onLeave={leave} />}</Context.Provider>;
}

export function useLeaveProtection() {
  const value = useContext(Context);
  if (!value) throw new Error("LeaveProtectionProvider is required");
  return value;
}

export function useDraftProtection(dirty: boolean, pending = false) {
  const protection = useLeaveProtection();
  const wasPending = useRef(pending);
  useEffect(() => {
    if (wasPending.current && !pending) protection.submissionSettled();
    wasPending.current = pending;
  }, [pending, protection.submissionSettled]);
  const status = useRef({ dirty, pending });
  status.current = { dirty, pending };
  useLayoutEffect(() => protection.register(status), [protection.register]);
  return protection;
}
