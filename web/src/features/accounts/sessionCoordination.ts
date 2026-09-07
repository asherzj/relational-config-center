export const activityStorageKey = "rcc:last-activity-report";
export const activityLockName = "rcc:activity-report";
export const accountEntryLockName = "rcc:account-entry";
export const sessionEventStorageKey = "rcc:session-event";
export const sessionEventChannelName = "rcc:session-events";

export type SessionEventType = "changed" | "ended";
type SessionEvent = { type: SessionEventType; at: number; nonce: number };

type ActivityLockManager = {
  request<T>(name: string, callback: () => Promise<T>): Promise<T>;
};

export function withBrowserLock<T>(name: string, callback: () => Promise<T>): Promise<T> {
  const locks = (navigator as Navigator & { locks?: ActivityLockManager }).locks;
  if (!locks) return Promise.reject(new Error("browser coordination is unavailable"));
  return locks.request(name, callback);
}

export function publishSessionEvent(type: SessionEventType) {
  const event: SessionEvent = { type, at: Date.now(), nonce: Math.random() };
  let stored = false;
  try {
    localStorage.removeItem(activityStorageKey);
    localStorage.setItem(sessionEventStorageKey, JSON.stringify(event));
    stored = true;
  } catch { /* session state remains correct even when cross-tab storage is unavailable */ }
  if (stored) return;
  try {
    const channel = new BroadcastChannel(sessionEventChannelName);
    channel.postMessage(event);
    channel.close();
  } catch { /* visibility verification remains the final browser fallback */ }
}

export function subscribeSessionEvents(listener: (type: SessionEventType) => void) {
  const seen = new Set<number>();
  const receive = (value: unknown) => {
    if (!value || typeof value !== "object") return;
    const event = value as Partial<SessionEvent>;
    if (event.type !== "changed" && event.type !== "ended") return;
    if (typeof event.nonce === "number") {
      if (seen.has(event.nonce)) return;
      seen.add(event.nonce);
      if (seen.size > 32) seen.delete(seen.values().next().value!);
    }
    listener(event.type);
  };
  const storage = (event: StorageEvent) => {
    if (event.key !== sessionEventStorageKey || !event.newValue) return;
    try { receive(JSON.parse(event.newValue)); } catch { /* ignore malformed cross-tab data */ }
  };
  const message = (event: MessageEvent) => receive(event.data);
  let channel: BroadcastChannel | undefined;
  window.addEventListener("storage", storage);
  try {
    channel = new BroadcastChannel(sessionEventChannelName);
    channel.addEventListener("message", message);
  } catch { /* storage and visibility verification remain available */ }
  return () => {
    window.removeEventListener("storage", storage);
    channel?.removeEventListener("message", message);
    channel?.close();
  };
}
