import { AlertTriangle } from "lucide-react";
import { useId, useRef } from "react";
import { DialogDescription, DialogTitle } from "../shadcn/dialog";
import { Button } from "./Button";
import { ModalSurface } from "./ModalSurface";

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

export function ConfirmDialog({ open, title, description, confirmLabel, destructive, pending, onConfirm, onCancel }: Props) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  const descriptionId = useId();
  return (
    <ModalSurface
      open={open} onClose={onCancel} pending={pending} role="alertdialog"
      labelledBy={titleId} describedBy={descriptionId} initialFocusRef={cancelRef}
      dismissLabel="取消操作" className="confirm-dialog"
    >
      <AlertTriangle className={destructive ? "text-destructive" : "text-muted-foreground"} size={24} aria-hidden="true" />
      <DialogTitle id={titleId}>{title}</DialogTitle>
      <DialogDescription id={descriptionId} className="leading-7">{description}</DialogDescription>
      <div className="confirm-actions">
        <Button ref={cancelRef} onClick={onCancel} disabled={pending}>取消</Button>
        <Button variant={destructive ? "danger" : "primary"} onClick={onConfirm} disabled={pending}>
          {pending ? "正在处理…" : confirmLabel}
        </Button>
      </div>
    </ModalSurface>
  );
}
