import { settingsFormClass } from "./form-styles";
import { secondaryButtonClass } from "../../shared/ui/styles";
import { useMutation, useQuery } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { FolderPlus, Info, Layers, Pencil, Plus, Server, ShieldCheck } from "lucide-react";
import { useAuth } from "../../shared/session";
import { canManageRole, type Project, type ProjectDetail, roleLabels } from "../../shared/session";
import { api } from "../../shared/api";
import { DataTable, ErrorState, Loading, Panel, formatTime } from "../../shared/ui";

/** Slugs are lowercase words joined by hyphens; the name is a good start for one. */
const slugFrom = (name: string) => name.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 63);

export function CreateProjectForm({ onCreated }: { onCreated?: (project: Project) => void }) {
  const { credential, session } = useAuth();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const create = useMutation({
    mutationFn: (body: { name: string; slug: string; environment: string }) => api<{ project: Project }>("/api/v1/projects", credential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: async ({ project }) => {
      await session?.refresh();
      session?.switchProject(project.id);
      setName("");
      setSlug("");
      onCreated?.(project);
    },
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    create.mutate({ name: name.trim(), slug: slug || slugFrom(name), environment: String(values.get("environment") || "").trim() });
  };
  return (
    // Columns follow the form's own width: the first-project gate is a narrow panel even on a wide screen.
    <div className="@container border-b border-black/5 dark:border-white/10 bg-slate-500/[0.02] dark:bg-white/[0.015] p-4 sm:p-6">
      <form className={`settings-form ${settingsFormClass} project-form max-w-[980px] border-b-0 px-0 pt-3.5 pb-0 @min-[720px]:grid-cols-[minmax(180px,1.2fr)_minmax(160px,1fr)_minmax(160px,1fr)_auto]`} onSubmit={submit}>
        <div className="col-span-full mb-1">
          <div className="flex items-center gap-2">
            <span className="p-1.5 rounded-lg bg-indigo-500/10 text-indigo-600 dark:text-indigo-400">
              <FolderPlus size={16} />
            </span>
            <h3 className="settings-form-title m-0 text-[12px] font-bold uppercase tracking-wider text-faint">New project</h3>
          </div>
          <p className="text-[12.5px] text-slate-500 dark:text-slate-400 mt-1 m-0">Create a dedicated space to collect traces, spans, and metrics for your team.</p>
        </div>
        <label>
          Name
          <input value={name} onChange={(event) => setName(event.target.value)} required maxLength={100} placeholder="e.g. Checkout" />
        </label>
        <label>
          Slug
          <input value={slug || slugFrom(name)} onChange={(event) => setSlug(event.target.value)} required maxLength={63} pattern="[a-z0-9][a-z0-9\-]*" placeholder="checkout" />
        </label>
        <label>
          First environment
          <input name="environment" maxLength={63} placeholder="production" />
        </label>
        <div className="settings-submit flex items-end [&>button]:inline-flex [&>button]:items-center [&>button]:justify-center [&>button]:gap-1.5 [&>button]:whitespace-nowrap [&>button]:rounded-lg [&>button]:border [&>button]:border-btn-border [&>button]:bg-btn [&>button]:px-5.5 [&>button]:py-2 [&>button]:text-[13px] [&>button]:font-semibold [&>button]:text-btn-text [&>button:hover]:bg-btn-hover @max-[719px]:[&>button]:w-full">
          <button type="submit" disabled={create.isPending} className="inline-flex items-center justify-center gap-1.5 shadow-sm">
            <Plus size={15} />
            <span>{create.isPending ? "Creating…" : "Create project"}</span>
          </button>
        </div>
        {create.error && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg settings-wide col-span-full" role="alert">{create.error.message}</p>}
      </form>
    </div>
  );
}

export function ProjectsSection() {
  const { credential, session } = useAuth();
  const current = session?.project;
  const detail = useQuery({
    queryKey: ["settings-project", current?.project.id],
    enabled: Boolean(current),
    queryFn: ({ signal }) => api<ProjectDetail>(`/api/v1/projects/${current?.project.id}`, credential, { signal }),
  });
  const rename = useMutation({
    mutationFn: (name: string) => api<{ project: Project }>(`/api/v1/projects/${current?.project.id}`, credential, { method: "PATCH", body: JSON.stringify({ name }) }),
    onSuccess: async () => { await session?.refresh(); await detail.refetch(); },
  });
  const [leaving, setLeaving] = useState("");
  const leave = useMutation({
    mutationFn: (projectID: string) => api<void>(`/api/v1/projects/${projectID}/members/${session?.user.id}`, credential, { method: "DELETE" }),
    onSuccess: async () => { setLeaving(""); await session?.refresh(); },
  });
  const addEnvironment = useMutation({
    mutationFn: (name: string) => api(`/api/v1/projects/${current?.project.id}/environments`, credential, { method: "POST", body: JSON.stringify({ name }) }),
    onSuccess: async () => { await detail.refetch(); },
  });
  if (!session) return null;
  const submitEnvironment = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    addEnvironment.mutate(String(new FormData(form).get("environment") || "").trim(), { onSuccess: () => form.reset() });
  };
  const submitRename = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    rename.mutate(String(new FormData(event.currentTarget).get("name") || "").trim());
  };
  return <>
    <Panel title="Your projects" meta={`${session.memberships.length} accessible`}>
      <CreateProjectForm />
      <DataTable headers={["Project", "Slug", "Your role", "Created", ""]} empty="You are not a member of any project yet."
        rows={session.memberships.map(({ project, role }) => {
          const isCurrent = project.id === current?.project.id;
          const initial = project.name.trim().charAt(0).toUpperCase() || "P";
          return [
            <div key={project.id} className="flex items-center gap-3 py-0.5">
              <div className="w-8 h-8 rounded-xl bg-gradient-to-br from-indigo-500 via-indigo-600 to-sky-500 text-white font-bold text-xs flex items-center justify-center shadow-xs flex-shrink-0 select-none">
                {initial}
              </div>
              <div>
                <span className="font-bold text-slate-900 dark:text-white leading-tight">{project.name}</span>
              </div>
            </div>,
            <code key="slug" className="inline-flex items-center px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800/90 text-xs font-mono font-medium text-slate-700 dark:text-slate-300 border border-black/5 dark:border-white/10">
              {project.slug}
            </code>,
            <div key="role" className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-700 dark:text-slate-300">
              {session.user.platform_admin && <ShieldCheck size={14} className="text-indigo-500 dark:text-indigo-400" />}
              <span>{session.user.platform_admin ? "Super administrator" : roleLabels[role]}</span>
            </div>,
            <span key="created" className="text-xs text-slate-500 dark:text-slate-400 font-medium">
              {formatTime(project.created_at)}
            </span>,
            <span key="actions" className="table-actions inline-flex flex-wrap items-center gap-1.5">
              {isCurrent ? (
                <span className="current-tag inline-flex items-center gap-1.5 rounded-full border border-accent/35 bg-accent/12 px-2.5 py-0.75 text-[11.5px] font-semibold text-accent gap-2 px-3 py-1 text-xs bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/25 shadow-xs">
                  <span className="relative flex h-2 w-2">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                  </span>
                  Current
                </span>
              ) : (
                <button type="button" className={`button-secondary ${secondaryButtonClass} compact px-2.5 py-1 text-[11.5px] !rounded-lg !text-xs !font-semibold !px-3.5 !py-1.5 shadow-xs`} onClick={() => session.switchProject(project.id)}>
                  Open
                </button>
              )}
              {!session.user.platform_admin && (leaving === project.id
                ? <span className="inline-confirm inline-flex flex-wrap items-center gap-1.5"><button className="button-danger inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-semibold leading-snug transition-colors hover:border-accent/45 border-danger/40 bg-danger-bg text-danger-fg hover:bg-danger/20" disabled={leave.isPending} onClick={() => leave.mutate(project.id)}>Confirm leave</button><button className={`button-secondary ${secondaryButtonClass}`} onClick={() => setLeaving("")}>Cancel</button></span>
                : <button type="button" className="button-danger inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-semibold leading-snug transition-colors hover:border-accent/45 border-danger/40 bg-danger-bg text-danger-fg hover:bg-danger/20 compact px-2.5 py-1 text-[11.5px] !rounded-lg !text-xs !font-semibold" onClick={() => setLeaving(project.id)} aria-label={`Leave ${project.name}`}>Leave</button>)}
            </span>,
          ];
        })} />
      {leave.error && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg" role="alert">{leave.error.message}</p>}
    </Panel>
    {current && <Panel title={`${current.project.name} — environments`} meta={detail.data ? `${detail.data.environments.length} environments` : undefined}>
      {canManageRole(session.role) && (
        <div className="p-5 sm:p-6 border-b border-black/5 dark:border-white/10 bg-slate-500/[0.015] dark:bg-white/[0.01]">
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 items-start">
            {/* Card 1: Rename project */}
            <div className="@container min-w-0 rounded-2xl p-5 bg-white/70 dark:bg-slate-900/50 border border-black/5 dark:border-white/10 shadow-xs">
              <div className="flex items-center gap-2 mb-1">
                <span className="p-1.5 rounded-lg bg-slate-500/10 text-slate-700 dark:text-slate-300">
                  <Pencil size={15} />
                </span>
                <h3 className="settings-form-title col-span-full m-0 text-[11px] font-bold uppercase tracking-wider text-faint !m-0 !text-[12px] !font-bold">Rename project</h3>
              </div>
              <p className="text-[12px] text-slate-500 dark:text-slate-400 mb-4">Change the display name of this project across the console.</p>
              <form className={`settings-form ${settingsFormClass} single-field-form w-full @min-[22rem]:grid-cols-[minmax(0,1fr)_auto] !p-0 !border-0 !gap-3`} onSubmit={submitRename} key={current.project.name}>
                <label className="w-full">
                  Name
                  <input name="name" defaultValue={current.project.name} required maxLength={100} className="w-full" />
                </label>
                <div className="settings-submit flex items-end [&>button]:inline-flex [&>button]:items-center [&>button]:justify-center [&>button]:gap-1.5 [&>button]:whitespace-nowrap [&>button]:rounded-lg [&>button]:border [&>button]:border-btn-border [&>button]:bg-btn [&>button]:px-5.5 [&>button]:py-2 [&>button]:text-[13px] [&>button]:font-semibold [&>button]:text-btn-text [&>button:hover]:bg-btn-hover @max-[22rem]:[&>button]:w-full">
                  <button type="submit" disabled={rename.isPending} className="!bg-slate-900 hover:!bg-slate-800 dark:!bg-slate-100 dark:!text-slate-900 dark:hover:!bg-white !text-white !font-bold !text-xs !px-4 !py-2.5 !rounded-xl transition-all shadow-xs">
                    {rename.isPending ? "Saving…" : "Save name"}
                  </button>
                </div>
                {rename.error && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg settings-wide col-span-full" role="alert">{rename.error.message}</p>}
              </form>
            </div>

            {/* Card 2: Add an environment */}
            <div className="@container min-w-0 rounded-2xl p-5 bg-white/70 dark:bg-slate-900/50 border border-black/5 dark:border-white/10 shadow-xs">
              <div className="flex items-center gap-2 mb-1">
                <span className="p-1.5 rounded-lg bg-indigo-500/10 text-indigo-600 dark:text-indigo-400">
                  <Layers size={15} />
                </span>
                <h3 className="settings-form-title col-span-full m-0 text-[11px] font-bold uppercase tracking-wider text-faint !m-0 !text-[12px] !font-bold">Add an environment</h3>
              </div>
              <p className="text-[12px] text-slate-500 dark:text-slate-400 mb-4">Create partitions like staging, production, or testing.</p>
              <form className={`settings-form ${settingsFormClass} single-field-form w-full @min-[22rem]:grid-cols-[minmax(0,1fr)_auto] !p-0 !border-0 !gap-3`} onSubmit={submitEnvironment}>
                <label className="w-full">
                  Name
                  <input name="environment" required maxLength={63} placeholder="e.g. staging" className="w-full" />
                </label>
                <div className="settings-submit flex items-end [&>button]:inline-flex [&>button]:items-center [&>button]:justify-center [&>button]:gap-1.5 [&>button]:whitespace-nowrap [&>button]:rounded-lg [&>button]:border [&>button]:border-btn-border [&>button]:bg-btn [&>button]:px-5.5 [&>button]:py-2 [&>button]:text-[13px] [&>button]:font-semibold [&>button]:text-btn-text [&>button:hover]:bg-btn-hover @max-[22rem]:[&>button]:w-full">
                  <button type="submit" disabled={addEnvironment.isPending} className="inline-flex items-center gap-1.5 !text-xs !font-bold !px-4 !py-2.5 !rounded-xl shadow-xs">
                    <Plus size={15} />
                    <span>{addEnvironment.isPending ? "Adding…" : "Add environment"}</span>
                  </button>
                </div>
                {addEnvironment.error && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg settings-wide col-span-full" role="alert">{addEnvironment.error.message}</p>}
              </form>
            </div>
          </div>

          {/* OTLP Hint callout */}
          <div className="mt-4 rounded-xl p-3.5 bg-blue-500/5 dark:bg-blue-400/5 border border-blue-500/15 text-blue-900 dark:text-blue-200 text-xs flex items-start gap-2.5">
            <Info size={16} className="text-blue-600 dark:text-blue-400 shrink-0 mt-0.5" />
            <p className="settings-hint col-span-full m-0 text-xs text-faint !m-0 !text-slate-600 dark:!text-slate-300 leading-relaxed">
              Senders pick an environment with the <code className="px-1.5 py-0.5 rounded bg-blue-100 dark:bg-blue-950 font-mono text-[11px] text-blue-800 dark:text-blue-300 font-semibold">X-Environment</code> header (OTLP and API); without it, telemetry goes to the first one.
            </p>
          </div>
        </div>
      )}

      {detail.isPending ? <Loading /> : detail.isError ? <ErrorState error={detail.error} retry={() => void detail.refetch()} />
        : <DataTable headers={["Environment", "Created"]}
            rows={detail.data.environments.map((environment, index) => [
              <div key={environment.name} className="flex items-center gap-2.5 py-0.5">
                <span className="p-1 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400">
                  <Server size={14} />
                </span>
                <span className="font-mono font-bold text-slate-900 dark:text-white text-xs">{environment.name}</span>
                {index === 0 && (
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
                    Default
                  </span>
                )}
              </div>,
              <span key="created" className="text-xs text-slate-500 dark:text-slate-400 font-medium">
                {formatTime(environment.created_at)}
              </span>,
            ])} empty="No environments." />}
    </Panel>}
  </>;
}
