import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { useBlocker, useLocation } from "react-router-dom";
import { Button } from "./Button";
import { ModalSurface } from "./ModalSurface";
import { DialogTitle, DialogDescription } from "../shadcn/dialog";

type DraftStatus = { dirty: boolean; pending: boolean; locationKey: string };
type Protection = {
  register: (status: RefObject<DraftStatus>) => () => void;
  requestLeave: (action: () => void) => void;
  afterSave: (action: () => void) => void;
  submissionSettled: () => void;
};
const Context = createContext<Protection | null>(null);

function LeaveDialog({ busy, onStay, onLeave }: { busy: boolean; onStay: () => void; onLeave: () => void }) {
  const stayRef = useRef<HTMLButtonElement>(null);
  return createPortal(
    <ModalSurface open role="alertdialog" labelledBy="leave-title" describedBy="leave-description" onClose={onStay} initialFocusRef={stayRef} dismissLabel="关闭离开提示" className="confirm-dialog leave-protection-layer">
        <DialogTitle id="leave-title">{busy ? "正在提交，请稍候" : "放弃未保存的修改？"}</DialogTitle>
        <DialogDescription id="leave-description" className="leading-7">{busy ? "请求已发出，请等待结果后再离开。此次离开已取消，不会重复提交。" : "离开后，本次输入和字段选择将丢失。继续编辑可保留当前内容。"}</DialogDescription>
        <div className="confirm-actions">
          <Button ref={stayRef} onClick={onStay}>{busy ? "留在此页" : "继续编辑"}</Button>
          {!busy && <Button variant="danger" onClick={onLeave}>放弃修改并离开</Button>}
        </div>
    </ModalSurface>, document.body,
  );
}

/** One router blocker handles all active drafts, including browser POP navigation. */
export function LeaveProtectionProvider({ children, enabled = true }: { children: ReactNode; enabled?: boolean }) {
  const statuses = useRef(new Set<RefObject<DraftStatus>>());
  const bypass = useRef(false);
  const [localLeave, setLocalLeave] = useState<(() => void) | null>(null);
  const [busyNotice, setBusyNotice] = useState(false);
  const status = useCallback((locationKey?: string) => {
    const current = [...statuses.current].filter((item) => locationKey === undefined || item.current.locationKey === locationKey);
    return { dirty: current.some((item) => item.current.dirty), pending: current.some((item) => item.current.pending) };
  }, []);
  const blocker = useBlocker(({ currentLocation }) => {
    // The router advances before React unmounts the previous page. An outgoing
    // draft must not block a second navigation from the new history entry.
    const current = status(currentLocation.key);
    return enabled && !bypass.current && (current.dirty || current.pending);
  });
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
    if (!enabled) { action(); return; }
    const current = status();
    if (current.pending) { setBusyNotice(true); return; }
    if (current.dirty) setLocalLeave(() => action);
    else action();
  }, [enabled, status]);
  useEffect(() => {
    if (enabled) return;
    // Authentication interruption must not leave a hidden navigation or focus trap
    // queued behind re-login. Draft registrations stay available for recovery.
    setLocalLeave(null);
    setBusyNotice(false);
    if (blocker.state === "blocked") blocker.reset();
  }, [enabled, blocker]);
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
  return <Context.Provider value={value}>{children}{enabled && (localLeave || blocker.state === "blocked" || busyNotice) && <LeaveDialog busy={busyNotice || status().pending} onStay={stay} onLeave={leave} />}</Context.Provider>;
}

export function useLeaveProtection() {
  const value = useContext(Context);
  if (!value) throw new Error("LeaveProtectionProvider is required");
  return value;
}

export function useDraftProtection(dirty: boolean, pending = false, inFlight?: RefObject<boolean>) {
  const protection = useLeaveProtection();
  const location = useLocation();
  const wasPending = useRef(pending);
  useEffect(() => {
    if (wasPending.current && !pending) protection.submissionSettled();
    wasPending.current = pending;
  }, [pending, protection.submissionSettled]);
  const status = useRef({ dirty, pending, locationKey: location.key });
  // Mutation observers publish pending asynchronously. Navigation and native
  // beforeunload must also see a request started in the current event turn.
  status.current = { dirty, locationKey: location.key, get pending() { return pending || Boolean(inFlight?.current); } };
  useLayoutEffect(() => protection.register(status), [protection.register]);
  return protection;
}
