import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";
import { Button as PrimitiveButton } from "../shadcn/button";
import { cn } from "../../lib/utils";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  icon?: ReactNode;
};

const variants = { primary: "default", secondary: "outline", ghost: "ghost", danger: "destructive" } as const;

export const Button = forwardRef<HTMLButtonElement, Props>(function Button({ variant = "secondary", icon, className = "", children, ...props }, ref) {
  return (
    <PrimitiveButton ref={ref} variant={variants[variant]} className={cn("button", className.includes("icon-button") && "size-9 shrink-0 p-0", className)} {...props}>
      {icon}
      {children}
    </PrimitiveButton>
  );
});
