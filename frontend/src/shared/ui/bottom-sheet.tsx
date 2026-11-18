import { X } from "lucide-react";
import type { ReactNode } from "react";
import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";

const focusable = "a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])";

type Props = { open: boolean; title: string; onClose: () => void; children: ReactNode; footer?: ReactNode };

/**
 * A modal panel that rises from the bottom edge. It traps focus, closes on Escape or the
 * backdrop, locks the page behind it and returns focus to whatever opened it.
 */
export function BottomSheet({ open, title, onClose, children, footer }: Props) {
  const panel = useRef<HTMLDivElement>(null);
  const titleID = useId();
  const close = useRef(onClose);
  close.current = onClose;

  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const first = panel.current?.querySelector<HTMLElement>(`[data-autofocus], ${focusable}`);
    (first ?? panel.current)?.focus({ preventScroll: true });
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); close.current(); return; }
      if (event.key !== "Tab" || !panel.current) return;
      const items = [...panel.current.querySelectorAll<HTMLElement>(focusable)];
      if (!items.length) return;
      const [head, tail] = [items[0], items[items.length - 1]];
      if (event.shiftKey && document.activeElement === head) { event.preventDefault(); tail.focus(); }
      else if (!event.shiftKey && document.activeElement === tail) { event.preventDefault(); head.focus(); }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      opener?.focus?.({ preventScroll: true });
    };
  }, [open]);

  if (!open) return null;
  return createPortal(
    <div className="bottom-sheet fixed inset-0 z-[60] flex flex-col justify-end">
      <div className="absolute inset-0 bg-slate-950/45 motion-safe:animate-fade-in" aria-hidden="true" onClick={onClose} />
      <div ref={panel} role="dialog" aria-modal="true" aria-labelledby={titleID} tabIndex={-1}
        className="relative flex max-h-[88dvh] w-full max-w-2xl flex-col self-center rounded-t-[22px] border border-b-0 border-glass-edge bg-surface-solid shadow-[0_-12px_40px_rgba(15,23,42,0.25)] outline-none motion-safe:animate-sheet-up">
        <div className="mx-auto mt-2 h-1 w-10 shrink-0 rounded-full bg-line" aria-hidden="true" />
        <header className="flex shrink-0 items-center justify-between gap-3 px-5 pt-2 pb-3">
          <h2 id={titleID} className="m-0 text-[15px] font-semibold text-text">{title}</h2>
          <button type="button" onClick={onClose} aria-label="Close" className="inline-flex size-9 items-center justify-center rounded-full text-muted hover:bg-hover hover:text-text">
            <X size={18} aria-hidden="true" />
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 pb-4">{children}</div>
        {footer && <footer className="shrink-0 border-t border-line-soft px-5 pt-3 pb-[max(12px,env(safe-area-inset-bottom))]">{footer}</footer>}
        {!footer && <div className="h-[env(safe-area-inset-bottom)] shrink-0" aria-hidden="true" />}
      </div>
    </div>,
    document.body,
  );
}
