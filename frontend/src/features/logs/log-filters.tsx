import { SlidersHorizontal, Search, X } from "lucide-react";
import type { FormEvent } from "react";
import { useId, useState } from "react";
import { BottomSheet, useIsMobile } from "../../shared/ui";
import { primaryButtonClass, secondaryButtonClass } from "../../shared/ui/styles";
import { severityGroups } from "./log-format";

type Props = {
  params: URLSearchParams;
  formKey: string;
  active: string[];
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onPick: (severity: string) => void;
  onClear: () => void;
};

const field = "h-11 w-full min-w-0 rounded-xl border border-glass-field-edge bg-glass-field px-3 text-[14px] text-text focus:border-accent focus:ring-2 focus:ring-accent/15 focus:outline-none";
const tones: Record<string, string> = { ERROR: "text-rose-500", FATAL: "text-rose-500", WARN: "text-amber-500", INFO: "text-sky-500" };
const desktopField = "h-9 min-w-0 rounded-lg border border-glass-field-edge bg-glass-field px-3 text-xs text-text focus:border-accent focus:ring-2 focus:ring-accent/15";

/** Severity chips plus the text filters. Phones keep search inline and move the identifiers into a sheet. */
export function LogFilters({ params, formKey, active, onSubmit, onPick, onClear }: Props) {
  const mobile = useIsMobile();
  const formID = useId();
  const [sheet, setSheet] = useState(false);
  const value = (name: string) => params.get(name) || "";
  const extra = ["service", "traceId", "spanId"].filter((name) => params.get(name)).length;
  const submit = (event: FormEvent<HTMLFormElement>) => { setSheet(false); onSubmit(event); };

  const chips = <div className="severity-chips inline-flex h-9 shrink-0 items-center gap-0.5 self-start rounded-xl border border-glass-field-edge bg-glass-field p-0.75 max-md:h-10 max-md:max-w-full max-md:self-stretch max-md:overflow-x-auto max-md:[scrollbar-width:none]" role="group" aria-label="Quick severity filter">
    {["", ...severityGroups.slice(2)].map((severity) => {
      const selected = value("severity") === severity;
      return <button type="button" key={severity || "all"} aria-pressed={selected} onClick={() => onPick(severity)}
        className={`severity-chip inline-flex h-full shrink-0 cursor-pointer items-center justify-center rounded-lg border px-3 text-[11px] leading-none font-bold tracking-wide uppercase transition-colors hover:bg-glass-hover hover:text-text max-md:flex-1 max-md:px-3.5 ${selected ? "border-btn-border bg-btn text-btn-text" : `border-transparent ${tones[severity] ?? "text-muted"}`} ${severity}`}>
        {severity || "All"}
      </button>;
    })}
  </div>;

  const identifierFields = [["service", "Service", "e.g. checkout", ""], ["traceId", "Trace ID", "32 hex characters", "font-mono"], ["spanId", "Span ID", "16 hex characters", "font-mono"]] as const;
  // Closed sheet: the current values still travel with a search. Open sheet: editable, attached to the form by id.
  const identifiers = (attach?: string) => identifierFields.map(([name, label, hint, font]) => attach
    ? <label key={name}>{label}<input name={name} placeholder={hint} defaultValue={value(name)} form={attach} className={`${field} ${font}`} /></label>
    : <input key={name} type="hidden" name={name} value={value(name)} />);

  if (!mobile) return <div className="log-toolbar-left flex min-w-0 flex-[1_1_36rem] flex-wrap items-start gap-2.5">
    {chips}
    <form className="toolbar log-filters m-0 flex min-w-0 max-w-full flex-[1_1_28rem] flex-wrap items-center gap-2" key={formKey} onSubmit={submit}>
      <input name="service" placeholder="Service" aria-label="Filter logs by service" defaultValue={value("service")} className={`${desktopField} max-w-44 flex-[1_1_7rem]`} />
      <input type="hidden" name="severity" value={value("severity")} />
      <input name="search" placeholder="Search body text" aria-label="Search log body" defaultValue={value("search")} className={`${desktopField} max-w-80 flex-[2_1_10rem]`} />
      <input name="traceId" placeholder="Trace ID" aria-label="Filter logs by trace ID" defaultValue={value("traceId")} className={`${desktopField} max-w-48 flex-[1_1_7rem] font-mono`} />
      <input name="spanId" placeholder="Span ID" aria-label="Filter logs by span ID" defaultValue={value("spanId")} className={`${desktopField} max-w-40 flex-[1_1_6rem] font-mono`} />
      <button type="submit" className={`${primaryButtonClass} h-9 whitespace-nowrap`}><Search size={15} /> Search logs</button>
      {active.length > 0 && <button type="button" className={`button-secondary ${secondaryButtonClass} h-9`} onClick={onClear}><X size={15} /> Clear filters</button>}
    </form>
  </div>;

  return <div className="log-toolbar-left grid w-full gap-2.5" key={formKey}>
    {chips}
    <form id={formID} className="log-filters m-0 grid grid-cols-[minmax(0,1fr)_auto_auto] gap-2" role="search" onSubmit={submit}>
      <input type="hidden" name="severity" value={value("severity")} />
      <input name="search" type="search" placeholder="Search body text" aria-label="Search log body" defaultValue={value("search")} className={field} />
      <button type="submit" aria-label="Search logs" className="inline-flex size-11 items-center justify-center rounded-xl bg-btn text-btn-text"><Search size={17} aria-hidden="true" /></button>
      <button type="button" onClick={() => setSheet(true)} aria-haspopup="dialog" aria-expanded={sheet}
        className={`relative inline-flex h-11 items-center gap-1.5 rounded-xl border px-3 text-[13px] font-semibold ${extra ? "border-accent/50 bg-accent/10 text-accent" : "border-glass-field-edge bg-glass-field text-muted"}`}>
        <SlidersHorizontal size={16} aria-hidden="true" />Filters{extra > 0 && <span className="rounded-full bg-accent px-1.5 text-[10.5px] text-white">{extra}</span>}
      </button>
      {!sheet && identifiers()}
    </form>
    <BottomSheet open={sheet} title="Filter logs" onClose={() => setSheet(false)} footer={<div className="grid grid-cols-2 gap-2">
      <button type="button" disabled={!active.length} onClick={() => { setSheet(false); onClear(); }} className={`${secondaryButtonClass} h-11 justify-center rounded-xl`}><X size={15} />Clear all</button>
      <button type="submit" form={formID} className="inline-flex h-11 items-center justify-center gap-1.5 rounded-xl bg-btn text-[13.5px] font-semibold text-btn-text"><Search size={15} />Apply</button>
    </div>}>
      <div className="grid gap-3 [&_label]:grid [&_label]:gap-1.5 [&_label]:text-[12px] [&_label]:font-semibold [&_label]:text-muted">
        <p className="m-0 text-[12.5px] text-muted">Narrow the logs to one service, or follow a single trace or span.</p>
        {identifiers(formID)}
      </div>
    </BottomSheet>
  </div>;
}
