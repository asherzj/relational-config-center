import { z } from "zod";
import { request } from "./client";

export const approvalNotificationSchema = z.object({ sequence: z.string().regex(/^(0|[1-9][0-9]*)$/), unread: z.boolean(), pending: z.boolean() });
export type ApprovalNotification = z.infer<typeof approvalNotificationSchema>;
export const approvalNotifications = {
  read: (id: string, sequence: string) => request(`/api/v1/release-orders/${encodeURIComponent(id)}/notification-read`, { method: "POST", body: JSON.stringify({ sequence }), schema: approvalNotificationSchema }),
  counts: () => request("/api/v1/approval-notifications", { schema: z.object({ unread_count: z.number().int().nonnegative(), pending_count: z.number().int().nonnegative() }) }),
};
