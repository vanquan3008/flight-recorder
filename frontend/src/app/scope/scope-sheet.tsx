import { Check, ChevronDown, FolderGit2, Layers, RefreshCw } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useState } from "react";
import { useAuth } from "../../shared/session";
import { BottomSheet } from "../../shared/ui";
import { type ScopeOption, useEnvironmentOptions } from "./use-environments";

function Choices({ label, Icon, options, value, onPick }: { label: string; Icon: LucideIcon; options: ScopeOption[]; value: string; onPick: (value: string) => void }) {
  return <section className="mb-4" aria-label={label}>
    <h3 className="m-0 mb-2 flex items-center gap-1.5 text-[11px] font-bold tracking-wider text-faint uppercase"><Icon size={13} aria-hidden="true" />{label}</h3>
    <ul className="m-0 grid list-none gap-1.5 p-0">
      {options.map((option) => {
        const selected = option.value === value;
        return <li key={option.value || "default"}>
          <button type="button" aria-pressed={selected} onClick={() => onPick(option.value)}
            className={`flex min-h-12 w-full items-center justify-between gap-3 rounded-xl border px-3.5 text-left text-[13.5px] font-medium transition-colors ${selected ? "border-accent/50 bg-accent/10 text-text" : "border-line-soft bg-surface-raised text-muted hover:text-text"}`}>
            <span className="min-w-0 truncate">{option.label}</span>
            {selected && <Check size={16} className="shrink-0 text-accent" aria-hidden="true" />}
          </button>
        </li>;
      })}
    </ul>
  </section>;
}

/** The phone header shows the scope as one compact chip; tapping it opens the project and environment choices. */
export function ScopeChip({ environment, onEnvironment, dot }: { environment: string; onEnvironment: (value: string) => void; dot: string }) {
  const { session } = useAuth();
  const { detail, options, projectID } = useEnvironmentOptions(environment);
  const [open, setOpen] = useState(false);
  if (!session) return null;
  const project = session.project?.project.name ?? "Choose project";
  const environmentLabel = options.find((option) => option.value === environment)?.label ?? environment;
  const canPickEnvironment = Boolean(projectID) && !session.user.must_change_password;
  const projects = session.memberships.map(({ project: item }) => ({ value: item.id, label: item.name }));

  return <>
    <button type="button" onClick={() => setOpen(true)} aria-haspopup="dialog" aria-expanded={open}
      aria-label={`Scope: ${project}${canPickEnvironment ? `, ${environmentLabel}` : ""}. Change project or environment`}
      className="scope-chip flex h-10 min-w-0 flex-1 items-center gap-2 rounded-full border border-glass-field-edge bg-glass-field pr-2.5 pl-3 text-left">
      <i className={`size-2 shrink-0 rounded-full ${dot}`} aria-hidden="true" />
      <span className="min-w-0 flex-1 leading-tight">
        <span className="block truncate text-[12.5px] font-semibold text-text">{project}</span>
        {canPickEnvironment && <span className="block truncate text-[10.5px] text-muted">{environmentLabel}</span>}
      </span>
      <ChevronDown size={15} className="shrink-0 text-muted" aria-hidden="true" />
    </button>
    <BottomSheet open={open} title="Project and environment" onClose={() => setOpen(false)}>
      <Choices label="Project" Icon={FolderGit2} options={projects} value={projectID ?? ""}
        onPick={(value) => { setOpen(false); if (value !== projectID) session.switchProject(value); }} />
      {canPickEnvironment && <Choices label="Environment" Icon={Layers} options={options} value={environment}
        onPick={(value) => { setOpen(false); if (value !== environment) onEnvironment(value); }} />}
      {detail.isError && <button type="button" onClick={() => void detail.refetch()} className="inline-flex h-10 items-center gap-2 rounded-xl border border-amber-500/30 bg-amber-500/10 px-3 text-[12.5px] font-semibold text-amber-700 dark:text-amber-300">
        <RefreshCw size={14} aria-hidden="true" />Retry loading environments
      </button>}
    </BottomSheet>
  </>;
}
