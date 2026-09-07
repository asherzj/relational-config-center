import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  icon?: ReactNode;
};

export const Button = forwardRef<HTMLButtonElement, Props>(function Button({ variant = "secondary", icon, className = "", children, ...props }, ref) {
  return (
    <button ref={ref} className={`button button-${variant} ${className}`.trim()} {...props}>
      {icon}
      <span>{children}</span>
    </button>
  );
});
