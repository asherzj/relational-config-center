import { useRef, type ReactNode, type RefObject } from "react";
import { Dialog, DialogContent } from "../shadcn/dialog";
import { cn } from "../../lib/utils";
import { useModalFocus } from "./useModalFocus";

type Props = {
  open: boolean;
  children: ReactNode;
  onClose: () => void;
  pending?: boolean;
  label?: string;
  labelledBy?: string;
  describedBy?: string;
  role?: "dialog" | "alertdialog";
  className?: string;
  initialFocusRef?: RefObject<HTMLElement | null>;
  dismissLabel: string;
};

/** Keep overlays in their workspace: auth interruption must hide them without losing drafts.
 * Existing guards own nested focus and pending dismissals; shadcn owns the surface composition.
 */
export function ModalSurface({
  open, children, onClose, pending, label, labelledBy, describedBy,
  role = "dialog", className, initialFocusRef, dismissLabel,
}: Props) {
  const ref = useRef<HTMLDivElement>(null);
  useModalFocus({ open, dialogRef: ref, initialFocusRef, onEscape: pending ? undefined : onClose });
  if (!open) return null;

  return (
    <div className="modal-layer">
      <Dialog open modal={false}>
        <button type="button" tabIndex={-1} className="modal-backdrop" aria-label={dismissLabel} onClick={onClose} disabled={pending} />
        <DialogContent
          inline ref={ref} showCloseButton={false} role={role} aria-modal="true"
          aria-label={label} aria-labelledby={labelledBy} aria-describedby={describedBy}
          data-modal-surface="true" tabIndex={-1}
          className={cn("app-dialog max-h-[calc(100dvh-2rem)] overflow-y-auto border-0 bg-card shadow-xl sm:max-w-[480px]", className)}
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => event.preventDefault()}
          onInteractOutside={(event) => event.preventDefault()}
        >
          {children}
        </DialogContent>
      </Dialog>
    </div>
  );
}
