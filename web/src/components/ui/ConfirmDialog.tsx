import { AlertTriangle } from "lucide-react";
import { useRef } from "react";
import { Button } from "./Button";
import { useModalFocus } from "./useModalFocus";

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
  const dialogRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  useModalFocus({ open, dialogRef, initialFocusRef: cancelRef, onEscape: pending ? undefined : onCancel });
  if (!open) return null;
  return (
    <div className="modal-layer">
      <button className="drawer-scrim" aria-label="取消操作" disabled={pending} onClick={onCancel} />
      <div ref={dialogRef} tabIndex={-1} className="confirm-dialog" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-description" data-modal-surface="true">
        <AlertTriangle className={destructive ? "danger-color" : "accent-color"} aria-hidden="true" />
        <h2 id="confirm-title">{title}</h2>
        <p id="confirm-description">{description}</p>
        <div className="confirm-actions">
          <Button ref={cancelRef} onClick={onCancel} disabled={pending}>取消</Button>
          <Button variant={destructive ? "danger" : "primary"} onClick={onConfirm} disabled={pending}>
            {pending ? "正在处理…" : confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
