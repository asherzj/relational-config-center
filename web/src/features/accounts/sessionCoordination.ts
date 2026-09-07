export const activityStorageKey = "rcc:last-activity-report";
export const activityLockName = "rcc:activity-report";
export const accountEntryLockName = "rcc:account-entry";
export const sessionEventStorageKey = "rcc:session-event";

type ActivityLockManager = {
  request<T>(name: string, callback: () => Promise<T>): Promise<T>;
};

export function withBrowserLock<T>(name: string, callback: () => Promise<T>): Promise<T> {
  const locks = (navigator as Navigator & { locks?: ActivityLockManager }).locks;
  if (!locks) return Promise.reject(new Error("browser coordination is unavailable"));
  return locks.request(name, callback);
}

export function publishSessionEvent(type: "changed" | "ended") {
  try {
    localStorage.removeItem(activityStorageKey);
    localStorage.setItem(sessionEventStorageKey, JSON.stringify({ type, at: Date.now(), nonce: Math.random() }));
  } catch { /* session state remains correct even when cross-tab storage is unavailable */ }
}
