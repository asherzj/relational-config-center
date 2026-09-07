import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { accounts, type CurrentIdentity } from "../../api/accounts";
import { ApiError } from "../../api/client";
import { businessSessionInvalid, setBusinessSession } from "../../api/business-session";
import { Button } from "../../components/ui/Button";
import { AccountPage } from "./AccountPage";
import { useForegroundActivity } from "./useForegroundActivity";
import { subscribeSessionEvents } from "./sessionCoordination";

const WorkspaceIdentity = createContext<CurrentIdentity | null>(null);
const WorkspaceRecovery = createContext(0);
export function useWorkspaceIdentity() { return useContext(WorkspaceIdentity); }
export function useWorkspaceRecovery() { return useContext(WorkspaceRecovery); }

type WorkspaceStatus = "loading" | "ready" | "anonymous" | "interrupted" | "checking" | "unavailable";

export function ProtectedWorkspace() {
  const location = useLocation();
  const queryClient = useQueryClient();
  const [identity, setIdentity] = useState<CurrentIdentity | null>(null);
  const [status, setStatus] = useState<WorkspaceStatus>("loading");
  const [workspaceEpoch, setWorkspaceEpoch] = useState(0);
  const [recoveryVersion, setRecoveryVersion] = useState(0);
  const generation = useRef(0);
  const identityRef = useRef<CurrentIdentity | null>(null);
  const statusRef = useRef<WorkspaceStatus>("loading");

  function transition(next: WorkspaceStatus) {
    statusRef.current = next;
    setStatus(next);
  }
  function suspend(next: "interrupted" | "checking") {
    generation.current++;
    setBusinessSession(null);
    void queryClient.cancelQueries();
    transition(next);
  }
  function discard() {
    generation.current++;
    setBusinessSession(null);
    queryClient.clear();
    identityRef.current = null;
    setIdentity(null);
    setWorkspaceEpoch((value) => value + 1);
  }

  async function accept(current: CurrentIdentity, currentGeneration: number) {
    if (generation.current !== currentGeneration) return;
    const previousAccountID = identityRef.current?.account.id;
    const sameAccount = previousAccountID === current.account.id;
    if (previousAccountID && !sameAccount) {
      queryClient.clear();
      setWorkspaceEpoch((value) => value + 1);
    }
    setBusinessSession({ accountID: current.account.id, csrf: current.csrf_token });
    identityRef.current = current;
    setIdentity(current);
    if (previousAccountID) {
      await queryClient.refetchQueries({ type: "active" }, { throwOnError: true });
      if (generation.current !== currentGeneration) return;
      if (sameAccount) setRecoveryVersion((value) => value + 1);
    }
    transition("ready");
  }

  async function inspect(initial = false) {
    const currentGeneration = ++generation.current;
    if (initial) transition("loading");
    try {
      await accept(await accounts.current(), currentGeneration);
    } catch (cause) {
      if (generation.current !== currentGeneration) return;
      if (cause instanceof ApiError && cause.status === 401) {
        if (cause.code === "account_disabled") {
          discard();
          transition("anonymous");
        } else {
          transition(identityRef.current ? "interrupted" : "anonymous");
        }
      } else {
        transition("unavailable");
      }
    }
  }
  async function resume(current: CurrentIdentity) {
    const currentGeneration = ++generation.current;
    try {
      await accept(current, currentGeneration);
    } catch {
      if (generation.current === currentGeneration) transition("unavailable");
    }
  }

  useEffect(() => {
    discard();
    void inspect(true);
    const invalid = (event: Event) => {
      const code = (event as CustomEvent<{ code?: string }>).detail?.code;
      if (code === "account_disabled") {
        discard();
        transition("anonymous");
        return;
      }
      suspend(code === "account_changed" ? "checking" : "interrupted");
      if (code === "account_changed") void inspect();
    };
    const synchronize = (type: "changed" | "ended") => {
      if (type === "ended") { discard(); transition("anonymous"); }
      if (type === "changed") { suspend("checking"); void inspect(); }
    };
    const verifyOnReturn = () => {
      if (document.visibilityState === "hidden" || statusRef.current !== "ready") return;
      suspend("checking");
      void inspect();
    };
    window.addEventListener(businessSessionInvalid, invalid);
    const unsubscribeSessionEvents = subscribeSessionEvents(synchronize);
    document.addEventListener("visibilitychange", verifyOnReturn);
    return () => {
      generation.current++;
      setBusinessSession(null);
      window.removeEventListener(businessSessionInvalid, invalid);
      unsubscribeSessionEvents();
      document.removeEventListener("visibilitychange", verifyOnReturn);
    };
  }, [queryClient]);

  useForegroundActivity(status === "ready" ? identity : null, {
    generation: () => generation.current,
    isCurrent: (value, csrf) => generation.current === value && identityRef.current?.csrf_token === csrf,
    onCurrent: (current) => { identityRef.current = current; setIdentity(current); },
    onInvalid: (code) => {
      if (code === "account_disabled") { discard(); transition("anonymous"); }
      else suspend("interrupted");
    },
    onError: () => transition("unavailable"),
  });

  const destination = location.pathname + location.search + location.hash;
  if (status === "anonymous") {
    return <Navigate to={`/login?returnTo=${encodeURIComponent(destination)}`} replace />;
  }
  if (!identity) {
    if (status === "unavailable") return <main className="account-page"><section className="account-card"><p role="alert">暂时无法确认登录状态，原登录凭据已保留。</p><Button onClick={() => void inspect()}>重新检查登录状态</Button></section></main>;
    return <main className="account-page"><p role="status">正在检查登录状态…</p></main>;
  }

  const hidden = status !== "ready";
  return <WorkspaceIdentity value={identity}><WorkspaceRecovery value={recoveryVersion}>
    <div key={workspaceEpoch} className="protected-workspace" data-session-status={status} hidden={hidden} aria-hidden={hidden}><Outlet /></div>
    {status === "interrupted" && <div className="session-interruption" role="dialog" aria-modal="true" aria-label="登录会话已中断"><section className="session-interruption-copy"><h2>登录会话已中断</h2><p>工作区已遮住。使用同一账号重新登录后会重新读取当前规则和目标数据，并保留本页内存中的未提交内容；系统不会自动执行写入。</p></section><AccountPage mode="login" onSignedIn={(current) => void resume(current)} onSessionInvalid={(code) => { if (code === "account_disabled") { discard(); transition("anonymous"); } }} /></div>}
    {status === "checking" && <div className="session-interruption"><main className="account-page"><section className="account-card"><p role="status">正在重新确认当前账号并检查工作区数据…</p></section></main></div>}
    {status === "unavailable" && <div className="session-interruption"><main className="account-page"><section className="account-card"><p role="alert">账号服务暂时不可用，工作区已遮住，原登录凭据和同账号内存编辑意图已保留。</p><Button onClick={() => { suspend("checking"); void inspect(); }}>重新检查登录状态</Button></section></main></div>}
  </WorkspaceRecovery></WorkspaceIdentity>;
}
