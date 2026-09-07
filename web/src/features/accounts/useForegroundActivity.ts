import { useEffect } from "react";
import { accounts, type CurrentIdentity } from "../../api/accounts";
import { ApiError } from "../../api/client";
import { activityStorageKey, activityLockName, withBrowserLock } from "./sessionCoordination";

export function useForegroundActivity(identity: CurrentIdentity | null, callbacks: {
 generation: () => number;
 isCurrent: (generation: number, csrf: string) => boolean;
 onCurrent: (current: CurrentIdentity) => void;
 onInvalid: () => void;
 onError: (cause: unknown) => void;
}) {
  useEffect(() => {
    if (!identity) return;
    const report = () => {
      if (document.visibilityState === "hidden") return;
      const generation = callbacks.generation();
      const sessionCSRF = identity.csrf_token;
      void withBrowserLock(activityLockName, async () => {
        if (document.visibilityState === "hidden" || !callbacks.isCurrent(generation, sessionCSRF)) return;
        const now = Date.now();
        let previous = 0;
        try { previous = Number(localStorage.getItem(activityStorageKey) ?? 0); } catch { /* storage may be unavailable */ }
        if (Number.isFinite(previous) && now - previous < 60_000) return;
        const reservation = String(now);
        try { localStorage.setItem(activityStorageKey, reservation); } catch { /* this tab still reports */ }
        try {
          const current = await accounts.activity(sessionCSRF);
          if (callbacks.isCurrent(generation, sessionCSRF)) callbacks.onCurrent(current);
        } catch (cause) {
          try { if (localStorage.getItem(activityStorageKey) === reservation) localStorage.removeItem(activityStorageKey); } catch { /* retry on the next interaction */ }
          if (!callbacks.isCurrent(generation, sessionCSRF)) return;
          if (cause instanceof ApiError && cause.status === 401) {
            callbacks.onInvalid();
            return;
          }
          callbacks.onError(cause);
        }
      }).catch((cause) => {
        if (callbacks.isCurrent(generation, sessionCSRF)) callbacks.onError(cause);
      });
    };
    document.addEventListener("pointerdown", report, { passive: true });
    document.addEventListener("keydown", report);
    document.addEventListener("touchstart", report, { passive: true });
    return () => {
      document.removeEventListener("pointerdown", report);
      document.removeEventListener("keydown", report);
      document.removeEventListener("touchstart", report);
    };
  }, [identity]);

}
