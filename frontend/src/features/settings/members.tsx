import { settingsFormClass } from "./form-styles";
import { secondaryButtonClass } from "../../shared/ui/styles";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useAuth } from "../../shared/session";
import { canManageRole, type Member, type Role, roleHints, roleLabels, roles } from "../../shared/session";
import { api } from "../../shared/api";
import { Badge, DataTable, ErrorState, Loading, Panel, formatTime } from "../../shared/ui";

export function MembersSection() {
  const { credential, session } = useAuth();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState("");
  const projectID = session?.project?.project.id;
  const actorRole = session?.role;
  const base = `/api/v1/projects/${projectID}/members`;
  const members = useQuery({
    queryKey: ["settings-members", projectID],
    enabled: Boolean(projectID),
    queryFn: ({ signal }) => api<{ data: Member[] }>(base, credential, { signal }),
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["settings-members", projectID] });
  const add = useMutation({
    mutationFn: (body: { email: string; role: Role }) => api<Member>(base, credential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: refresh,
  });
  const change = useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) => api(`${base}/${id}`, credential, { method: "PATCH", body: JSON.stringify({ role }) }),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api<void>(`${base}/${id}`, credential, { method: "DELETE" }),
    onSuccess: async (_, id) => {
      setConfirming("");
      if (id === session?.user.id) await session.refresh();
      await refresh();
    },
  });
  if (!session || !projectID) return null;
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    add.mutate({ email: String(values.get("email") || "").trim(), role: String(values.get("role")) as Role }, { onSuccess: () => form.reset() });
  };
  const manager = canManageRole(actorRole);
  const failure = add.error || change.error || remove.error;
  return <Panel title="Members" meta={members.data ? `${members.data.data.length} in ${session.project?.project.name}` : undefined}>
    <form className={`settings-form ${settingsFormClass} hook-form min-[901px]:grid-cols-[minmax(180px,1fr)_minmax(220px,1.5fr)_auto]`} onSubmit={submit}>
      <h3 className="settings-form-title col-span-full m-0 text-[11px] font-bold uppercase tracking-wider text-faint">Add a member</h3>
      <label>Account email<input type="email" name="email" required maxLength={254} placeholder="teammate@company.com" /></label>
      <label>Role<select name="role" defaultValue="user">{roles.map((role) => <option key={role} value={role}>{roleLabels[role]} — {roleHints[role]}</option>)}</select></label>
      <div className="settings-submit flex items-end [&>button]:inline-flex [&>button]:items-center [&>button]:justify-center [&>button]:gap-1.5 [&>button]:whitespace-nowrap [&>button]:rounded-lg [&>button]:border [&>button]:border-btn-border [&>button]:bg-btn [&>button]:px-5.5 [&>button]:py-2 [&>button]:text-[13px] [&>button]:font-semibold [&>button]:text-btn-text [&>button:hover]:bg-btn-hover max-[900px]:[&>button]:w-full"><button type="submit" disabled={add.isPending}>{add.isPending ? "Adding…" : "Add member"}</button></div>
      <p className="settings-hint col-span-full m-0 text-xs text-faint">The person needs an account first. Anyone can register; share the sign-up page with them first.</p>
    </form>
    {failure && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg" role="alert">{failure.message}</p>}
    {members.isPending ? <Loading /> : members.isError ? <ErrorState error={members.error} retry={() => void members.refetch()} /> :
      <DataTable headers={["Person", "Role", "Last sign-in", "Added", "Actions"]} empty="No members."
        rows={members.data.data.map((member) => {
          const self = member.user.id === session.user.id;
          const manageable = manager;
          return [
            <span className="person grid gap-px [&_b]:font-semibold [&_small]:text-[11.5px] [&_small]:text-faint"><span><b>{member.user.display_name}</b>{self && <small> (you)</small>}</span><small>{member.user.email}</small></span>,
            manageable
              ? <select aria-label={`Role of ${member.user.display_name}`} value={member.role} disabled={change.isPending} onChange={(event) => change.mutate({ id: member.user.id, role: event.target.value as Role })}>
                {roles.map((role) => <option key={role} value={role}>{roleLabels[role]}</option>)}
              </select>
              : <Badge value={roleLabels[member.role]} />,
            member.user.last_login_at ? formatTime(member.user.last_login_at) : "Never", formatTime(member.created_at),
            !manageable && !self ? "—" : confirming === member.user.id
              ? <span className="inline-confirm inline-flex flex-wrap items-center gap-1.5"><button className="button-danger inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-semibold leading-snug transition-colors hover:border-accent/45 border-danger/40 bg-danger-bg text-danger-fg hover:bg-danger/20" disabled={remove.isPending} onClick={() => remove.mutate(member.user.id)}>{self ? "Confirm leave" : "Confirm remove"}</button><button className={`button-secondary ${secondaryButtonClass}`} onClick={() => setConfirming("")}>Cancel</button></span>
              : <button className="button-danger inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-semibold leading-snug transition-colors hover:border-accent/45 border-danger/40 bg-danger-bg text-danger-fg hover:bg-danger/20" onClick={() => setConfirming(member.user.id)} aria-label={`${self ? "Leave project" : "Remove"} ${member.user.display_name}`}>{self ? "Leave" : "Remove"}</button>,
          ];
        })} />}
  </Panel>;
}
