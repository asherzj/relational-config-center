import { X } from "lucide-react";
import { useEffect, useRef, type ReactNode } from "react";
import { Sheet, SheetContent, SheetTitle, SheetDescription } from "../shadcn/sheet";
import { Button } from "./Button";
import { useModalFocus } from "./useModalFocus";

type Props = {
  open: boolean;
  title: string;
  eyebrow: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
};

export function Drawer({ open, title, eyebrow, onClose, children, footer }: Props) {
  const dialogRef = useRef<HTMLDivElement>(null);
  useModalFocus({ open, dialogRef, onEscape: onClose });
  useEffect(() => {
    if (!open) return;
    document.body.classList.add("drawer-open");
    return () => document.body.classList.remove("drawer-open");
  }, [open]);
  if (!open) return null;

  return (
    <div className="drawer-layer">
      <Sheet open modal={false}>
        <button type="button" tabIndex={-1} className="drawer-backdrop" aria-label="关闭抽屉" onClick={onClose} />
        <SheetContent
          inline showCloseButton={false} ref={dialogRef} tabIndex={-1}
          aria-modal="true" data-modal-surface="true"
          className="drawer grid h-dvh w-full grid-rows-[auto_minmax(0,1fr)_auto] gap-0 border-0 bg-card p-0 sm:max-w-[640px]"
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => event.preventDefault()}
          onInteractOutside={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => event.preventDefault()}
        >
          <header className="drawer-header">
            <div>
              <SheetTitle className="text-xl font-semibold">{title}</SheetTitle>
              <SheetDescription className="sr-only">{eyebrow}</SheetDescription>
            </div>
            <Button variant="ghost" className="icon-button" aria-label="关闭" onClick={onClose}><X size={20} /></Button>
          </header>
          <div className="drawer-body">{children}</div>
          {footer && <footer className="drawer-footer">{footer}</footer>}
        </SheetContent>
      </Sheet>
    </div>
  );
}
