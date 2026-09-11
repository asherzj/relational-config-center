import { useEffect, useRef } from "react";
import { useLocation } from "react-router-dom";
import { businessSession } from "../../api/business-session";
import { useWorkspaceReady } from "../accounts/ProtectedWorkspace";

// The protected workspace remains mounted during reauthentication. Poll only
// while it can display data; recovery and route returns perform a fresh read.
export function useNotificationRefresh(refetch: () => Promise<unknown>) {
  const ready = useWorkspaceReady();
  const { key } = useLocation();
  const refresh = useRef(refetch);
  refresh.current = refetch;
  useEffect(() => {
    if (!ready) return;
    const read = () => {
      if (document.visibilityState !== "hidden" && businessSession().credentials) void refresh.current();
    };
    read();
    const interval = window.setInterval(read, 30_000);
    window.addEventListener("focus", read);
    return () => { window.clearInterval(interval); window.removeEventListener("focus", read); };
  }, [ready, key]);
}
