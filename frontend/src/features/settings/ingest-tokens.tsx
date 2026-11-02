import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useAuth } from "../../app/auth";
import { api, pageItems, usePagedQuery } from "../../shared/api";
import type { IngestToken } from "../../shared/session";
import { Badge, CopyButton, PagedTable, Panel, formatTime } from "../../shared/ui";
import { SecretNotice } from "./secret-notice";

const tokenState = (token: IngestToken) => token.revoked_at ? "REVOKED" : Date.parse(token.expires_at) < Date.now() ? "EXPIRED" : "ACTIVE";

/** How a sender uses the token: an OTLP header, and optionally the environment to write into. */
function usage(token: string) {
  return `OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20${token}`;
}

/** The credentials an application uses to send telemetry to this project. Owners create and revoke them. */
export function IngestTokensSection() {
  const { credential, session } = useAuth();
  const queryClient = useQueryClient();
  const projectID = session?.project?.project.id ?? "";
  const [issued, setIssued] = useState<{ title: string; secret: string } | null>(null);
  const [confirming, setConfirming] = useState("");
  const base = `/api/v1/projects/${projectID}/ingest-tokens`;
  const query = usePagedQuery<IngestToken>(["settings-ingest-tokens", projectID], `${base}?limit=50`, credential);
  const tokens = pageItems(query.data?.pages);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["settings-ingest-tokens", projectID] });
  const create = useMutation({
    mutationFn: (body: { name: string; expires_at?: string }) => api<{ token: IngestToken; jwt: string }>(base, credential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: async (result) => { setIssued({ title: `Ingest token “${result.token.name}” created`, secret: result.jwt }); await refresh(); },
  });
  const revoke = useMutation({
    mutationFn: (id: string) => api<void>(`${base}/${id}`, credential, { method: "DELETE" }),
    onSuccess: async () => { setConfirming(""); await refresh(); },
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const expires = String(values.get("expires") || "");
    create.mutate({ name: String(values.get("name") || "").trim(), ...(expires ? { expires_at: new Date(`${expires}T23:59:59`).toISOString() } : {}) }, { onSuccess: () => form.reset() });
  };
  if (!projectID) return null;
  const failure = create.error || revoke.error;
  return <>
    {issued && <SecretNotice {...issued} onDismiss={() => setIssued(null)} />}
    {issued && <p className="settings-hint ingest-usage"><code>{usage(issued.secret)}</code> <CopyButton value={usage(issued.secret)} label="Copy OTLP header setting" /></p>}
    <Panel title="Ingest tokens" meta={`${tokens.length} loaded`}>
      <form className="settings-form hook-form" onSubmit={submit}>
        <h3 className="settings-form-title">New ingest token</h3>
        <label>Name<input name="name" required maxLength={64} pattern="[A-Za-z0-9][A-Za-z0-9 ._\-]*" placeholder="e.g. checkout-service" /></label>
        <label>Expires (optional, default one year)<input type="date" name="expires" min={new Date().toISOString().slice(0, 10)} /></label>
        <div className="settings-submit"><button type="submit" disabled={create.isPending}>{create.isPending ? "Creating…" : "Create token"}</button></div>
        <p className="settings-hint">A token belongs to this project and can only send telemetry into it. Give each application its own, so one can be revoked alone. Pick the environment with the <code>X-Environment</code> header.</p>
      </form>
      {failure && <p className="form-error" role="alert">{failure.message}</p>}
      <PagedTable query={query} empty="No ingest tokens yet." headers={["Name", "Status", "Created", "Expires", "Actions"]}
        rows={tokens.map((token) => {
          const state = tokenState(token);
          return [token.name, <Badge value={state} />, formatTime(token.created_at), formatTime(token.expires_at),
            state === "REVOKED" ? "—" : confirming === token.id
              ? <span className="inline-confirm"><button className="button-danger" disabled={revoke.isPending} onClick={() => revoke.mutate(token.id)}>Confirm revoke</button><button className="button-secondary" onClick={() => setConfirming("")}>Cancel</button></span>
              : <button className="button-danger" onClick={() => setConfirming(token.id)} aria-label={`Revoke ${token.name}`}>Revoke</button>];
        })} />
    </Panel>
  </>;
}
