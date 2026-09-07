import { useLayoutEffect, useRef, type RefObject } from "react";

const focusableSelector = [
  "a[href]",
  "button:not(:disabled)",
  "input:not(:disabled)",
  "select:not(:disabled)",
  "textarea:not(:disabled)",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

function focusableElements(container: HTMLElement) {
  return Array.from(container.querySelectorAll<HTMLElement>(focusableSelector))
    .filter((element) => {
      if (element.matches(":disabled") || element.tabIndex < 0 || element.closest("[inert]")) return false;
      for (let current: HTMLElement | null = element; current && container.contains(current); current = current.parentElement) {
        if (current.hidden || current.getAttribute("aria-hidden") === "true") return false;
        const style = window.getComputedStyle(current);
        if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse") return false;
      }
      return true;
    });
}

const modalStack: HTMLElement[] = [];
const managedInert = new Map<HTMLElement, boolean>();

function restoreManagedInert() {
  managedInert.forEach((wasInert, element) => {
    if (wasInert) element.setAttribute("inert", "");
    else element.removeAttribute("inert");
  });
  managedInert.clear();
}

function refreshModalEnvironment() {
  restoreManagedInert();
  for (let index = modalStack.length - 1; index >= 0; index -= 1) {
    if (!modalStack[index]!.isConnected) modalStack.splice(index, 1);
  }
  document.documentElement.classList.toggle("modal-open", modalStack.length > 0);
  document.body.classList.toggle("modal-open", modalStack.length > 0);
  const top = modalStack.at(-1);
  if (!top) return;

  // Keep the top layer (including its pointer scrim) available and make every
  // sibling branch behind that layer natively inert. This also inerts a lower
  // drawer or dialog when a confirmation is mounted above it.
  let branch: HTMLElement = top.parentElement ?? top;
  while (branch.parentElement) {
    const parent = branch.parentElement;
    for (const sibling of parent.children) {
      if (sibling === branch || !(sibling instanceof HTMLElement)) continue;
      managedInert.set(sibling, sibling.hasAttribute("inert"));
      sibling.setAttribute("inert", "");
    }
    if (parent === document.body) break;
    branch = parent;
  }
}

function isTopModal(container: HTMLElement) {
  return modalStack.at(-1) === container;
}

function canRestoreFocus(element: HTMLElement | null): element is HTMLElement {
  if (!element?.isConnected || element.matches(":disabled") || element.closest("[inert]")) return false;
  if (element.tabIndex < 0 && element.getAttribute("data-modal-surface") !== "true") return false;
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    if (current.hidden || current.getAttribute("aria-hidden") === "true") return false;
    const style = window.getComputedStyle(current);
    if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse") return false;
  }
  return true;
}

function focusModal(dialog: HTMLElement, preferred?: HTMLElement | null) {
  const focusable = focusableElements(dialog);
  const target = preferred && focusable.includes(preferred) ? preferred : dialog;
  target.focus();
}

export function useModalFocus({
  open,
  dialogRef,
  initialFocusRef,
  onEscape,
}: {
  open: boolean;
  dialogRef: RefObject<HTMLElement | null>;
  initialFocusRef?: RefObject<HTMLElement | null>;
  onEscape?: () => void;
}) {
  const escapeRef = useRef(onEscape);
  escapeRef.current = onEscape;

  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!open || !dialog) return;
    const previous = document.activeElement as HTMLElement | null;
    modalStack.push(dialog);
    refreshModalEnvironment();
    const onKeyDown = (event: KeyboardEvent) => {
      if (!isTopModal(dialog)) return;
      if (event.key === "Escape" && escapeRef.current) {
        event.preventDefault();
        event.stopPropagation();
        escapeRef.current();
        return;
      }
      if (event.key !== "Tab") return;

      const focusable = focusableElements(dialog);
      if (!focusable.length) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || active === dialog || !dialog.contains(active))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && (active === last || !dialog.contains(active))) {
        event.preventDefault();
        first.focus();
      }
    };
    const onFocusIn = (event: FocusEvent) => {
      if (!isTopModal(dialog) || dialog.contains(event.target as Node)) return;
      focusModal(dialog, initialFocusRef?.current);
    };

    document.addEventListener("keydown", onKeyDown, true);
    document.addEventListener("focusin", onFocusIn, true);
    focusModal(dialog, initialFocusRef?.current);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      document.removeEventListener("focusin", onFocusIn, true);
      const index = modalStack.lastIndexOf(dialog);
      if (index >= 0) modalStack.splice(index, 1);
      refreshModalEnvironment();
      if (canRestoreFocus(previous)) previous.focus();
      else modalStack.at(-1)?.focus();
    };
  }, [dialogRef, initialFocusRef, open]);
}
