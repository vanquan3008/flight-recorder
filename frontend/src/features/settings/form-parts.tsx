import type { FormEventHandler, ReactNode } from "react";
import { settingsFormClass } from "./form-styles";

// Tailwind only generates classes it finds written out, so the long class lists stay whole.
const submitSlotClass = "settings-submit flex items-end [&>button]:inline-flex [&>button]:items-center [&>button]:justify-center [&>button]:gap-1.5 [&>button]:whitespace-nowrap [&>button]:rounded-lg [&>button]:border [&>button]:border-btn-border [&>button]:bg-btn [&>button]:px-5.5 [&>button]:py-2 [&>button]:text-[13px] [&>button]:font-semibold [&>button]:text-btn-text [&>button:hover]:bg-btn-hover max-[900px]:[&>button]:w-full";
const errorClass = "form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg";

/** The cell that holds a form's submit button. */
export function SubmitSlot({ children }: { children: ReactNode }) {
  return <div className={submitSlotClass}>{children}</div>;
}

/** A failed request, shown under the form that sent it. */
export function FormError({ error, wide = false }: { error?: Error | null; wide?: boolean }) {
  if (!error) return null;
  return <p className={wide ? `${errorClass} settings-wide col-span-full` : errorClass} role="alert">{error.message}</p>;
}

type CreateFormProps = {
  title: string;
  hint: ReactNode;
  submit: { pending: boolean; label: string; pendingLabel: string };
  onSubmit: FormEventHandler<HTMLFormElement>;
  children: ReactNode;
};

/** The "new item" form every settings section opens with: a title, its fields, a submit button, and a hint. */
export function CreateForm({ title, hint, submit, onSubmit, children }: CreateFormProps) {
  return <form className={`settings-form ${settingsFormClass} hook-form min-[901px]:grid-cols-[minmax(180px,1fr)_minmax(220px,1.5fr)_auto]`} onSubmit={onSubmit}>
    <h3 className="settings-form-title col-span-full m-0 text-[11px] font-bold uppercase tracking-wider text-faint">{title}</h3>
    {children}
    <SubmitSlot><button type="submit" disabled={submit.pending}>{submit.pending ? submit.pendingLabel : submit.label}</button></SubmitSlot>
    <p className="settings-hint col-span-full m-0 text-xs text-faint">{hint}</p>
  </form>;
}
