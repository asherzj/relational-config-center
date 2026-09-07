import { Skeleton } from "../shadcn/skeleton";
import { AlertCircle, RefreshCw, Search } from "lucide-react";
import { presentError } from "../../api/error-messages";
import { Button } from "./Button";

export function LoadingState({ label = "正在加载…" }: { label?: string }) {
  return (
    <div className="feedback-state" role="status">
      <div className="grid w-full max-w-sm gap-3" aria-hidden="true"><Skeleton className="h-4 w-3/5" /><Skeleton className="h-4 w-full" /><Skeleton className="h-4 w-4/5" /></div>
      <strong>{label}</strong>
    </div>
  );
}

export function EmptyState({ entity = "查询规则" }: { entity?: string }) {
  return (
    <div className="feedback-state">
      <Search aria-hidden="true" />
      <strong>还没有{entity}</strong>
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
