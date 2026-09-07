import { useForegroundActivity } from "./useForegroundActivity";
import { sessionEventStorageKey } from "./sessionCoordination";
import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { accounts, type CurrentIdentity } from "../../api/accounts";
import { ApiError } from "../../api/client";
import { businessSessionInvalid, setBusinessSession } from "../../api/business-session";
import { Button } from "../../components/ui/Button";

const WorkspaceIdentity = createContext<CurrentIdentity | null>(null);
export function useWorkspaceIdentity() { return useContext(WorkspaceIdentity); }

export function ProtectedWorkspace() {
  const location = useLocation();
  const queryClient = useQueryClient();
  const [identity, setIdentity] = useState<CurrentIdentity | null>(null);
  const [status, setStatus] = useState<"loading" | "ready" | "anonymous" | "unavailable">("loading");
  const generation = useRef(0);
  const identityRef = useRef<CurrentIdentity | null>(null);
  const [activityError, setActivityError] = useState(false);

  function clear() {
    generation.current++;
    setBusinessSession(null);
    queryClient.clear();
    identityRef.current = null;
    setIdentity(null);
  }
  async function inspect() {
    const currentGeneration = ++generation.current;
    setStatus("loading");
    try {
      const current = await accounts.current();
      if (generation.current !== currentGeneration) return;
      setBusinessSession({ accountID: current.account.id, csrf: current.csrf_token });
      identityRef.current = current;
      setIdentity(current);
      setStatus("ready");
    } catch (cause) {
      if (generation.current !== currentGeneration) return;
      setStatus(cause instanceof ApiError && cause.status === 401 ? "anonymous" : "unavailable");
    }
  }
  useEffect(() => {
    clear();
    void inspect();
    const invalid = () => { clear(); setStatus("anonymous"); };
    const synchronize = (event: StorageEvent) => {
      if (event.key !== sessionEventStorageKey || !event.newValue) return;
      let type: unknown;
      try { type = JSON.parse(event.newValue).type; } catch { return; }
      if (type === "ended") invalid();
      if (type === "changed") { clear(); void inspect(); }
    };
    window.addEventListener(businessSessionInvalid, invalid);
    window.addEventListener("storage", synchronize);
    return () => {
      generation.current++;
      setBusinessSession(null);
      window.removeEventListener(businessSessionInvalid, invalid);
      window.removeEventListener("storage", synchronize);
    };
  }, [queryClient]);

  useForegroundActivity(identity, {
    generation: () => generation.current,
    isCurrent: (value, csrf) => generation.current === value && identityRef.current?.csrf_token === csrf,
    onCurrent: (current) => { identityRef.current = current; setIdentity(current); setActivityError(false); },
    onInvalid: () => { clear(); setStatus("anonymous"); },
    onError: () => setActivityError(true),
  });

  if (status === "anonymous") {
    const destination = location.pathname + location.search + location.hash;
    return <Navigate to={`/login?returnTo=${encodeURIComponent(destination)}`} replace />;
  }
  if (status === "unavailable") return <main className="account-page"><section className="account-card"><p role="alert">暂时无法确认登录状态，原登录凭据已保留。</p><Button onClick={() => void inspect()}>重新检查登录状态</Button></section></main>;
  if (status !== "ready" || !identity) return <main className="account-page"><p role="status">正在检查登录状态…</p></main>;
  return <WorkspaceIdentity value={identity}>{activityError && <p role="alert">活动续期尚未确认，请检查服务后重新验证登录状态。<Button onClick={() => void inspect()}>重新检查登录状态</Button></p>}<Outlet /></WorkspaceIdentity>;
}
