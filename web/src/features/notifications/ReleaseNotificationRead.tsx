import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { approvalNotifications, type ApprovalNotification } from "../../api/approval-notifications";
import { businessSession } from "../../api/business-session";
import type { ReleaseHeader } from "../../api/release-orders";
import { Button } from "../../components/ui/Button";
import { ApiError } from "../../api/client";
import { presentError } from "../../api/error-messages";
import { useWorkspaceReady } from "../accounts/ProtectedWorkspace";

// This component belongs to one successfully displayed header snapshot. An ack
// response may mention a newer sequence; it must never become the next ack input.
export function ReleaseNotificationRead({ order, canAcknowledge, onRefresh }: { order: ReleaseHeader; canAcknowledge: boolean; onRefresh: () => void }) {
  const ready = useWorkspaceReady();
  const client = useQueryClient();
  const attempted = useRef(false), mounted = useRef(false), sending = useRef(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>();
  const [result, setResult] = useState<ApprovalNotification>();
  const acknowledge = async () => {
    if (!ready || document.visibilityState === "hidden" || !businessSession().credentials || sending.current) return;
    const generation = businessSession().generation;
    attempted.current = true; sending.current = true; setPending(true); setError(undefined);
    try {
      const next = await approvalNotifications.read(order.id, order.notification.sequence);
      if (businessSession().generation !== generation) return;
      if (mounted.current) setResult(next);
      // The same account may already be back on its list when this finishes.
      void client.invalidateQueries({ queryKey: ["approval-notifications"] });
      void client.invalidateQueries({ queryKey: ["release-orders"] });
    } catch (cause) { if (mounted.current) setError(cause); }
    finally { sending.current = false; if (mounted.current) setPending(false); }
  };
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    if (ready && canAcknowledge && order.notification.unread && !attempted.current) void acknowledge();
  }, [ready, canAcknowledge]);
  if (!order.notification.unread) return null;
  const failure = error !== undefined ? presentError(error) : undefined;
  return <section className="space-y-2 text-sm" aria-label="本单通知">
    {pending && <p role="status">正在标记已展示的进展为已读…</p>}
    {failure && <div className="inline-alert min-w-0 flex-wrap" role="alert"><div className="min-w-0 break-words"><strong>标记已读失败，未读提醒已保留；详情仍可继续查看。</strong>{error instanceof ApiError && <span>错误代码：{error.code}</span>}{failure.requestId && <span>请求编号：{failure.requestId}</span>}</div><Button disabled={!ready || pending} onClick={() => void acknowledge()}>重试标记已读</Button></div>}
    {result && (result.unread ? <><p>本单还有新的未读变化。</p><Button disabled={!ready} onClick={onRefresh}>读取最新进展</Button></> : <p className="text-muted-foreground">已读{result.pending ? "；本单仍待你审批。" : "。"}</p>)}
  </section>;
}
