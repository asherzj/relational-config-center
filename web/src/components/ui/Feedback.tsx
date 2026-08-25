import { AlertCircle, RefreshCw, Search } from "lucide-react";
import { presentError } from "../../api/error-messages";
import { Button } from "./Button";

export function LoadingState({ label = "正在加载…" }: { label?: string }) {
  return (
    <div className="feedback-state" role="status">
      <span className="spinner" aria-hidden="true" />
      <strong>{label}</strong>
    </div>
  );
}

export function EmptyState() {
  return (
    <div className="feedback-state">
      <Search aria-hidden="true" />
      <strong>还没有查询策略</strong>
      <span>创建第一份草稿后，它会出现在这里。</span>
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const presented = presentError(error);
  return (
    <div className="feedback-state feedback-error" role="alert">
      <AlertCircle aria-hidden="true" />
      <strong>{presented.message}</strong>
      {presented.requestId && <span>请求编号：{presented.requestId}</span>}
      {onRetry && (
        <Button variant="secondary" icon={<RefreshCw size={16} />} onClick={onRetry}>
          重试
        </Button>
      )}
    </div>
  );
}
