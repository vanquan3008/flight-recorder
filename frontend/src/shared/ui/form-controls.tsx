import type { InputHTMLAttributes, ReactNode } from "react";
import { AlertCircle } from "lucide-react";

interface FormFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
  icon?: React.ComponentType<{ className?: string; size?: number }>;
  rightElement?: ReactNode;
  hint?: ReactNode;
  error?: string;
}

/**
 * Reusable form field featuring accessible label binding, optional leading Lucide icon,
 * optional trailing control (e.g. password toggle), hint text, and validation message.
 */
export function FormField({
  id,
  name,
  label,
  icon: Icon,
  rightElement,
  hint,
  error,
  className = "",
  ...props
}: FormFieldProps) {
  const inputId = id || (name ? `field-${name}` : undefined);

  return (
    <div className="grid gap-1.5 text-left">
      <label htmlFor={inputId} className="block text-xs font-semibold tracking-wide text-muted">
        {label}
      </label>
      <div className="group relative flex items-center">
        {Icon && (
          <span className="pointer-events-none absolute left-3.5 flex items-center text-muted transition-colors duration-150 group-focus-within:text-accent" aria-hidden="true">
            <Icon size={16} />
          </span>
        )}
        <input
          id={inputId}
          name={name}
          className={`w-full rounded-xl border border-glass-field-edge bg-glass-field py-2.5 text-sm font-medium text-text placeholder:text-faint transition-all duration-150 hover:border-white/20 focus:border-accent focus:bg-glass-field-focus focus:outline-none focus:ring-2 focus:ring-accent/20 disabled:cursor-not-allowed disabled:opacity-60 ${
            Icon ? "pl-10" : "pl-3.5"
          } ${rightElement ? "pr-11" : "pr-3.5"} ${className}`.trim()}
          {...props}
        />
        {rightElement && (
          <div className="absolute right-3 flex items-center">
            {rightElement}
          </div>
        )}
      </div>
      {hint && !error && <p className="text-[11.5px] leading-relaxed text-faint">{hint}</p>}
      {error && (
        <p className="text-[12px] font-medium leading-relaxed text-danger-fg motion-safe:animate-appear" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

/** Accessible form alert banner for API or validation errors */
export function FormAlert({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <div
      className="flex items-start gap-2.5 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300 motion-safe:animate-appear"
      role="alert"
    >
      <AlertCircle className="mt-0.5 size-4 shrink-0 text-rose-400" aria-hidden="true" />
      <span className="font-medium leading-relaxed">{message}</span>
    </div>
  );
}
