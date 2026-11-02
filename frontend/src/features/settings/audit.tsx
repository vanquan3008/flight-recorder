import { useAuth } from "../../app/auth";
import type { AuditEvent } from "../../shared/session";
import { pageItems, usePagedQuery } from "../../shared/api";
import { PagedTable, Panel, formatTime } from "../../shared/ui";

const actions: Record<string, string> = {
  "project.created": "Project created", "project.renamed": "Project renamed", "member.added": "Member added",
  "member.role_changed": "Role changed", "member.removed": "Member removed", "user.setup": "First administrator set up",
  "user.created": "Account created", "user.updated": "Account updated", "user.password_set": "Password set",
  "user.registered": "Account registered", "environment.created": "Environment added",
  "ingest_token.created": "Ingest token created", "ingest_token.revoked": "Ingest token revoked",
};

const short = (id?: string) => (id ? id.slice(0, 8) : "system");

/** The control-plane history of this project: who changed access, and when. */
export function AuditSection() {
  const { credential, session } = useAuth();
  const query = usePagedQuery<AuditEvent>(["settings-audit", session?.project?.project.id ?? ""], "/api/v1/audit-events?limit=50", credential);
  const events = pageItems(query.data?.pages);
  return <Panel title="Audit log" meta={`${events.length} loaded`}>
    <PagedTable query={query} empty="No access changes recorded yet." headers={["When", "Action", "By", "Target", "Detail"]}
      rows={events.map((event) => [
        formatTime(event.created_at), actions[event.action] ?? event.action,
        <code title={event.actor_user_id}>{event.actor_user_id ? `user ${short(event.actor_user_id)}` : "system"}</code>,
        <code title={event.target_id}>{event.target_type} {short(event.target_id)}</code>,
        <code>{Object.entries(event.detail).map(([key, value]) => `${key}=${String(value)}`).join(" ")}</code>,
      ])} />
  </Panel>;
}
