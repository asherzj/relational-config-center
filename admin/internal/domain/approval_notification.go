package domain

type ApprovalNotification struct {
	Sequence string `json:"sequence"`
	Unread   bool   `json:"unread"`
	Pending  bool   `json:"pending"`
}
type ApprovalNotificationCounts struct {
	UnreadCount  int `json:"unread_count"`
	PendingCount int `json:"pending_count"`
}
