import { AlertTriangle } from "lucide-react";
import { Button } from "./Button";

type Props = {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  destructive?: boolean;
  pending?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  destructive,
  pending,
  onConfirm,
  onCancel,
}: Props) {
  if (!open) return null;
  return (
    <div className="modal-layer">
      <button className="drawer-scrim" aria-label="取消操作" onClick={onCancel} />
      <div className="confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-description">
        <AlertTriangle className={destructive ? "danger-color" : "accent-color"} aria-hidden="true" />
        <h2 id="confirm-title">{title}</h2>
        <p id="confirm-description">{description}</p>
        <div className="confirm-actions">
          <Button onClick={onCancel} disabled={pending}>取消</Button>
          <Button variant={destructive ? "danger" : "primary"} onClick={onConfirm} disabled={pending}>
            {pending ? "正在处理…" : confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
