import { createContext, useCallback, useContext, useEffect, useMemo, type ReactNode } from "react";
import { toast } from "sonner";
import { Toaster } from "../shadcn/sonner";

type ToastContextValue = { showToast: (message: string) => void };
const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const showToast = useCallback((message: string) => { toast.success(message, { id: "operation-result", duration: 3200 }); }, []);
  const value = useMemo(() => ({ showToast }), [showToast]);
  useEffect(() => () => { toast.dismiss("operation-result"); }, []);
  return <ToastContext.Provider value={value}>{children}<Toaster position="bottom-right" containerAriaLabel="操作通知" closeButton toastOptions={{ className: "font-sans" }} /></ToastContext.Provider>;
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used inside ToastProvider");
  return context;
}
