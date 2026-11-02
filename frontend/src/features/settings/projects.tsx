import { useMutation, useQuery } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useAuth } from "../../app/auth";
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
      onCreated?.(project);
    },
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    create.mutate({ name: name.trim(), slug: slug || slugFrom(name), environment: String(values.get("environment") || "").trim() });
  };
  return <form className="settings-form project-form" onSubmit={submit}>
    <h3 className="settings-form-title">New project</h3>
    <label>Name<input value={name} onChange={(event) => setName(event.target.value)} required maxLength={100} placeholder="e.g. Checkout" /></label>
    <label>Slug<input value={slug || slugFrom(name)} onChange={(event) => setSlug(event.target.value)} required maxLength={63} pattern="[a-z0-9][a-z0-9\-]*" placeholder="checkout" /></label>
    <label>First environment<input name="environment" maxLength={63} placeholder="production" /></label>
    <div className="settings-submit"><button type="submit" disabled={create.isPending}>{create.isPending ? "Creating…" : "Create project"}</button></div>
    {create.error && <p className="form-error settings-wide" role="alert">{create.error.message}</p>}
  </form>;
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
        rows={session.memberships.map(({ project, role }) => [
          project.name, <code>{project.slug}</code>, session.user.platform_admin ? "Super administrator" : roleLabels[role], formatTime(project.created_at),
          <span className="table-actions">
            {project.id === current?.project.id ? <span className="muted-note">Current</span> : <button type="button" className="button-secondary compact" onClick={() => session.switchProject(project.id)}>Open</button>}
            {!session.user.platform_admin && (leaving === project.id
              ? <span className="inline-confirm"><button className="button-danger" disabled={leave.isPending} onClick={() => leave.mutate(project.id)}>Confirm leave</button><button className="button-secondary" onClick={() => setLeaving("")}>Cancel</button></span>
              : <button type="button" className="button-danger compact" onClick={() => setLeaving(project.id)} aria-label={`Leave ${project.name}`}>Leave</button>)}
          </span>,
        ])} />
      {leave.error && <p className="form-error" role="alert">{leave.error.message}</p>}
    </Panel>
    {current && <Panel title={`${current.project.name} — environments`} meta={detail.data ? `${detail.data.environments.length} environments` : undefined}>
      {canManageRole(session.role) && <form className="settings-form hook-form" onSubmit={submitRename} key={current.project.name}>
        <h3 className="settings-form-title">Rename project</h3>
        <label>Name<input name="name" defaultValue={current.project.name} required maxLength={100} /></label>
        <div className="settings-submit"><button type="submit" disabled={rename.isPending}>{rename.isPending ? "Saving…" : "Save name"}</button></div>
        {rename.error && <p className="form-error settings-wide" role="alert">{rename.error.message}</p>}
      </form>}
      {canManageRole(session.role) && <form className="settings-form hook-form" onSubmit={submitEnvironment}>
        <h3 className="settings-form-title">Add an environment</h3>
        <label>Name<input name="environment" required maxLength={63} placeholder="e.g. staging" /></label>
        <div className="settings-submit"><button type="submit" disabled={addEnvironment.isPending}>{addEnvironment.isPending ? "Adding…" : "Add environment"}</button></div>
        <p className="settings-hint">Senders pick an environment with the <code>X-Environment</code> header (OTLP and API); without it, telemetry goes to the first one.</p>
        {addEnvironment.error && <p className="form-error settings-wide" role="alert">{addEnvironment.error.message}</p>}
      </form>}
      {detail.isPending ? <Loading /> : detail.isError ? <ErrorState error={detail.error} retry={() => void detail.refetch()} />
        : <DataTable headers={["Environment", "Created"]} rows={detail.data.environments.map((environment) => [environment.name, formatTime(environment.created_at)])} empty="No environments." />}
    </Panel>}
  </>;
}
