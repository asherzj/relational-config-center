import { X } from "lucide-react";
import { useRef, type ReactNode } from "react";
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

  if (!open) return null;
  return (
    <div className="drawer-layer">
      <button className="drawer-scrim" aria-label="关闭抽屉" onClick={onClose} />
      <div className="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title" tabIndex={-1} ref={dialogRef} data-modal-surface="true">
        <header className="drawer-header">
          <div>
            <span className="drawer-eyebrow">{eyebrow}</span>
            <h2 id="drawer-title">{title}</h2>
          </div>
          <button className="icon-button" aria-label="关闭" onClick={onClose}>
            <X size={20} />
          </button>
        </header>
        <div className="drawer-body">{children}</div>
        {footer && <footer className="drawer-footer">{footer}</footer>}
      </div>
    </div>
  );
}
