import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useAuth } from "../../app/auth";
import { api, pageItems, usePagedQuery } from "../../shared/api";
import { PagedTable } from "../../shared/paged-table";
import { Badge, Panel, formatTime } from "../../shared/ui";
import { SecretNotice } from "./secret-notice";
import { type APIKey, type IssuedKey, keyScopes } from "./types";

const keyState = (key: APIKey) => key.revoked_at ? "REVOKED" : key.expires_at && Date.parse(key.expires_at) < Date.now() ? "EXPIRED" : "ACTIVE";

export function APIKeysSection() {
  const { apiKey } = useAuth();
  const queryClient = useQueryClient();
  const [issued, setIssued] = useState<{ title: string; secret: string } | null>(null);
  const [confirming, setConfirming] = useState("");
  const query = usePagedQuery<APIKey>(["settings-keys"], "/api/v1/api-keys?limit=50", apiKey);
  const keys = pageItems(query.data?.pages);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["settings-keys"] });
  const create = useMutation({
    mutationFn: (body: { name: string; scopes: string[]; expires_at?: string }) => api<IssuedKey>("/api/v1/api-keys", apiKey, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: async (result) => { setIssued({ title: `Key “${result.api_key.name}” created`, secret: result.key }); await refresh(); },
  });
  const rotate = useMutation({
    mutationFn: (id: string) => api<IssuedKey>(`/api/v1/api-keys/${id}/rotate`, apiKey, { method: "POST" }),
    onSuccess: async (result) => { setIssued({ title: `Key “${result.api_key.name}” rotated`, secret: result.key }); await refresh(); },
  });
  const revoke = useMutation({
    mutationFn: (id: string) => api<void>(`/api/v1/api-keys/${id}`, apiKey, { method: "DELETE" }),
    onSuccess: async () => { setConfirming(""); await refresh(); },
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    const expires = String(values.get("expires") || "");
    create.mutate({
      name: String(values.get("name") || "").trim(),
      scopes: values.getAll("scope").map(String),
      ...(expires ? { expires_at: new Date(`${expires}T23:59:59`).toISOString() } : {}),
    }, { onSuccess: () => form.reset() });
  };
  const failure = create.error || rotate.error || revoke.error;
  return <>
    {issued && <SecretNotice {...issued} onDismiss={() => setIssued(null)} />}
    <form className="settings-form" onSubmit={submit}>
      <label>Name<input name="name" required maxLength={64} pattern="[A-Za-z0-9][A-Za-z0-9 ._\-]*" placeholder="e.g. checkout-ingest" /></label>
      <fieldset><legend>Scopes</legend>{keyScopes.map(([scope, hint]) => <label className="check" key={scope}><input type="checkbox" name="scope" value={scope} defaultChecked={scope === "ingest"} /><span><b>{scope}</b> <small>{hint}</small></span></label>)}</fieldset>
      <label>Expires (optional)<input type="date" name="expires" min={new Date().toISOString().slice(0, 10)} /></label>
      <div className="settings-submit"><button type="submit" disabled={create.isPending}>{create.isPending ? "Creating…" : "Create key"}</button></div>
    </form>
    {failure && <p className="form-error" role="alert">{failure.message}</p>}
    <Panel title="API keys" meta={`${keys.length} loaded`}>
      <PagedTable query={query} empty="No API keys yet." headers={["Name", "Scopes", "Status", "Created", "Expires", "Actions"]}
        rows={keys.map((key) => {
          const state = keyState(key);
          return [key.name, <span className="scope-list">{key.scopes.map((scope) => <Badge key={scope} value={scope} />)}</span>, <Badge value={state} />,
            formatTime(key.created_at), key.expires_at ? formatTime(key.expires_at) : "Never",
            state === "REVOKED" ? "—" : confirming === key.id
              ? <span className="inline-confirm"><button className="button-danger" disabled={revoke.isPending} onClick={() => revoke.mutate(key.id)}>Confirm revoke</button><button className="button-secondary" onClick={() => setConfirming("")}>Cancel</button></span>
              : <span className="table-actions"><button className="button-secondary" disabled={rotate.isPending} onClick={() => rotate.mutate(key.id)} aria-label={`Rotate ${key.name}`}>Rotate</button><button className="button-danger" onClick={() => setConfirming(key.id)} aria-label={`Revoke ${key.name}`}>Revoke</button></span>];
        })} />
    </Panel>
  </>;
}
