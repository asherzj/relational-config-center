import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { approvalNotifications } from "../../api/approval-notifications";
import { ApiError } from "../../api/client";
import { presentError } from "../../api/error-messages";
import { Button } from "../../components/ui/Button";
import { useWorkspaceIdentity, useWorkspaceReady } from "../accounts/ProtectedWorkspace";

import { useNotificationRefresh } from "./useNotificationRefresh";

export function PersonalNotifications({ onNavigate }: { onNavigate: () => void }) {
  const identity = useWorkspaceIdentity();
  const ready = useWorkspaceReady();
  const counts = useQuery({ queryKey: ["approval-notifications", identity?.account.id], queryFn: approvalNotifications.counts, enabled: ready, retry: false });
  useNotificationRefresh(counts.refetch);
  const failure = counts.isError ? presentError(counts.error) : undefined;
  return <section className="mx-3 mb-4 space-y-2 rounded-lg border p-3 text-xs" aria-label="个人审批通知" aria-live="polite">
    <Link className="block font-medium underline underline-offset-4" to="/configuration/notifications?view=all&unread=true" onClick={onNavigate}>
      个人未读 {counts.data ? counts.data.unread_count : counts.isPending ? "读取中…" : "暂不可用"}
    </Link>
    <p aria-label="个人待审批数">待审批 {counts.data ? counts.data.pending_count : counts.isPending ? "读取中…" : "暂不可用"}</p>
    {failure && <div className="space-y-1 break-words text-destructive"><p>{counts.data ? "未读与待审批数刷新失败，保留上次读取的计数。" : "未读与待审批数读取失败。"}</p><p>{failure.message}</p>{counts.error instanceof ApiError && <p>错误代码：{counts.error.code}</p>}{failure.requestId && <p>请求编号：{failure.requestId}</p>}</div>}
    <Button variant="ghost" className="h-8 px-0 text-xs" disabled={!ready || counts.isFetching} onClick={() => void counts.refetch()}>{counts.isFetching ? "正在刷新通知…" : "刷新通知"}</Button>
  </section>;
}
