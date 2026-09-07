const defaultDestination = "/platform/query-policies";

export function safeReturnDestination(value: string | null): string {
  if (!value?.startsWith("/") || value.startsWith("//") || /[\\\u0000-\u0020]/u.test(value)) return defaultDestination;
  try {
    const decoded = decodeURIComponent(value);
    if (decoded.startsWith("//") || /[\\\u0000-\u0020]/u.test(decoded)) return defaultDestination;
    const target = new URL(value, window.location.origin);
    if (target.origin !== window.location.origin || ["/login", "/register"].includes(target.pathname)) return defaultDestination;
    return target.pathname + target.search + target.hash;
  } catch { return defaultDestination; }
}
