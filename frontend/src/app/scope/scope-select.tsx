import { Check, ChevronDown, FolderGit2, Layers } from "lucide-react";
import type { CSSProperties, KeyboardEvent } from "react";
import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

type Option = { value: string; label: string };
type Props = { label: "Project" | "Environment"; value: string; options: Option[]; onChange: (value: string) => void };

export function ScopeSelect({ label, value, options, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [position, setPosition] = useState<CSSProperties>({});
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const typeahead = useRef({ text: "", at: 0 });
  const id = useId();
  const selected = options.findIndex((option) => option.value === value);
  const selectedLabel = options[selected]?.label ?? "Choose project";
  const Icon = label === "Project" ? FolderGit2 : Layers;
  const iconTone = label === "Project" ? "text-sky-600 dark:text-sky-400" : "text-emerald-600 dark:text-emerald-400";
  const show = (index = Math.max(0, selected)) => {
    typeahead.current = { text: "", at: 0 };
    setActive(index);
    setOpen(true);
  };
  const choose = (index: number) => {
    const option = options[index];
    if (!option) return;
    setOpen(false);
    trigger.current?.focus({ preventScroll: true });
    if (option.value !== value) onChange(option.value);
  };

  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const rect = trigger.current?.getBoundingClientRect();
      if (!rect) return;
      const width = Math.min(Math.max(rect.width, 280), window.innerWidth - 24);
      const below = window.innerHeight - rect.bottom - 20;
      const above = rect.top - 20;
      const flip = below < 240 && above > below;
      setPosition({ width, left: Math.max(12, Math.min(rect.right - width, window.innerWidth - width - 12)),
        maxHeight: Math.max(80, flip ? above : below), ...(flip ? { bottom: window.innerHeight - rect.top + 8 } : { top: rect.bottom + 8 }) });
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => { window.removeEventListener("resize", place); window.removeEventListener("scroll", place, true); };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const dismiss = (event: PointerEvent) => {
      if (!trigger.current?.contains(event.target as Node) && !menu.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [open]);

  useEffect(() => {
    if (open) menu.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView?.({ block: "nearest" });
  }, [open, active]);

  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "Tab") { setOpen(false); return; }
    if (event.key === "Escape") {
      if (open) { event.preventDefault(); event.stopPropagation(); setOpen(false); }
      return;
    }
    if (["Enter", " "].includes(event.key)) {
      event.preventDefault();
      if (open) choose(active); else show();
      return;
    }
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      if (!open) show(event.key === "Home" ? 0 : event.key === "End" ? options.length - 1 : Math.max(0, selected));
      else setActive(event.key === "Home" ? 0 : event.key === "End" ? options.length - 1 : Math.max(0, Math.min(options.length - 1, active + (event.key === "ArrowDown" ? 1 : -1))));
      return;
    }
    if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      const now = Date.now();
      const text = (now - typeahead.current.at < 700 ? typeahead.current.text : "") + event.key.toLocaleLowerCase();
      typeahead.current = { text, at: now };
      const prefix = [...text].every((letter) => letter === text[0]) ? text[0] : text;
      const start = open ? active : Math.max(0, selected);
      const match = options.map((_, index) => (start + index + 1) % options.length).find((index) => options[index].label.toLocaleLowerCase().startsWith(prefix));
      if (match !== undefined) { setActive(match); setOpen(true); }
    }
  };

  return <>
    <button ref={trigger} type="button" role="combobox" aria-label={label} aria-haspopup="listbox" aria-expanded={open}
      aria-controls={open ? id : undefined} aria-activedescendant={open && options[active] ? `${id}-${active}` : undefined}
      title={`${label}: ${selectedLabel}`} onKeyDown={onKeyDown} onClick={() => open ? setOpen(false) : show()}
      onBlur={(event) => { if (!menu.current?.contains(event.relatedTarget as Node)) setOpen(false); }}
      className={`inline-flex h-[34px] min-w-0 items-center gap-1.5 rounded-full border border-solid px-2.5 text-xs text-text transition-colors max-[620px]:flex-1 max-[620px]:basis-0 max-[620px]:px-2 ${open ? "border-accent bg-hover ring-2 ring-accent/20" : "border-line bg-glass-field hover:bg-glass-field-focus"}`}>
      <Icon size={13} className={`shrink-0 ${iconTone}`} aria-hidden="true" />
      <span className="hidden select-none text-[10px] font-semibold uppercase tracking-wider text-faint xl:inline">{label === "Project" ? "Project" : "Env"}</span>
      <span className="hidden h-3 w-px shrink-0 bg-line xl:inline" aria-hidden="true" />
      <span className="min-w-0 max-w-48 flex-1 truncate text-left font-semibold max-[900px]:max-w-32 max-[620px]:max-w-none">{selectedLabel}</span>
      <ChevronDown size={11} className={`shrink-0 text-faint transition-transform ${open ? "rotate-180" : ""}`} aria-hidden="true" />
    </button>
    {open && createPortal(<div ref={menu} style={position} className="fixed z-100 flex flex-col overflow-hidden rounded-xl border border-line bg-surface-solid text-text shadow-[0_12px_40px_rgba(0,0,0,0.2)]">
      <div className="flex shrink-0 items-center gap-2 border-b border-line-soft px-3.5 py-3 text-[11px] font-semibold text-muted"><Icon size={14} className={iconTone} />Switch {label.toLowerCase()}</div>
      <div id={id} role="listbox" aria-label={`${label} options`} className="min-h-0 max-h-64 overflow-y-auto p-1.5">
        {options.length ? options.map((option, index) => <div key={option.value} id={`${id}-${index}`} role="option" aria-selected={option.value === value} data-index={index}
          onPointerDown={(event) => event.preventDefault()} onPointerMove={() => setActive(index)} onClick={() => choose(index)}
          className={`flex cursor-pointer items-center justify-between gap-3 rounded-lg px-3 py-2.5 text-xs leading-relaxed ${active === index ? "bg-hover" : ""} ${option.value === value ? "font-semibold text-accent" : "text-text"}`}>
          <span className="min-w-0 break-words">{option.label}</span>{option.value === value ? <Check size={14} className="shrink-0" aria-hidden="true" /> : <span className="w-3.5 shrink-0" />}
        </div>) : <p className="m-0 px-3 py-4 text-xs text-muted">No projects available</p>}
      </div>
    </div>, document.body)}
  </>;
}
