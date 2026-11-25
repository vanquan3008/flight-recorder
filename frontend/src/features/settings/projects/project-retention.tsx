import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { api } from "../../../shared/api";
import { ErrorState, Loading, Panel, formatCount } from "../../../shared/ui";
import { primaryButtonClass, secondaryButtonClass } from "../../../shared/ui/styles";
import { settingsFormClass } from "../form-styles";

type Retention = { retention_days: number | null; default_age_seconds: number; max_days: number };
const daySeconds = 86_400;

function systemAge(seconds: number): string {
  if (!seconds) return "keep indefinitely";
  if (seconds % daySeconds === 0) return formatCount(seconds / daySeconds, "day");
  if (seconds % 3_600 === 0) return formatCount(seconds / 3_600, "hour");
  return formatCount(seconds, "second");
}

export function ProjectRetention({ projectID, projectName, credential, editable }: { projectID: string; projectName: string; credential: string; editable: boolean }) {
  const queryClient = useQueryClient();
  const queryKey = ["project-retention", projectID];
  const path = `/api/v1/projects/${projectID}/retention`;
  const policy = useQuery({ queryKey, queryFn: ({ signal }) => api<Retention>(path, credential, { signal }) });
  const update = useMutation({
    mutationFn: (days: number | null) => api<Retention>(path, credential, { method: "PUT", body: JSON.stringify({ retention_days: days }) }),
    onSuccess: (saved) => queryClient.setQueryData(queryKey, saved),
  });

  return <Panel title="Data retention" meta={projectName}>
    {policy.isPending ? <Loading /> : policy.isError ? <ErrorState error={policy.error} retry={() => void policy.refetch()} /> : <>
      <p className="m-0 px-5 pt-4 text-sm text-muted">Applies to logs, traces, and metrics in every environment of this project.</p>
      {editable ? <RetentionForm key={String(policy.data.retention_days)} policy={policy.data} pending={update.isPending} onSave={update.mutate} />
        : <p className="m-0 px-5 py-4 text-sm font-medium">{policy.data.retention_days === null ? `System default (${systemAge(policy.data.default_age_seconds)})` : formatCount(policy.data.retention_days, "day")}</p>}
      {update.error && <p role="alert" className="m-0 px-5 pb-4 text-sm text-danger-fg">{update.error.message}</p>}
      {update.isSuccess && <p role="status" className="m-0 px-5 pb-4 text-sm text-ok-fg">Retention saved.</p>}
    </>}
  </Panel>;
}

function RetentionForm({ policy, pending, onSave }: { policy: Retention; pending: boolean; onSave: (days: number | null) => void }) {
  const policyLabel = useId();
  const [custom, setCustom] = useState(policy.retention_days !== null);
  const [confirm, setConfirm] = useState<number | null>();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const days = custom ? Number(new FormData(event.currentTarget).get("days")) : null;
    if (days === policy.retention_days) return;
    const effectiveAge = (days: number | null) => days === null ? policy.default_age_seconds || Infinity : days * daySeconds;
    if (effectiveAge(days) < effectiveAge(policy.retention_days)) setConfirm(days);
    else onSave(days);
  };

  return <form className={`${settingsFormClass} sm:grid-cols-[minmax(0,18rem)_minmax(0,10rem)_auto] border-b-0`} onSubmit={submit}>
    <label><span id={policyLabel}>Retention policy</span>
      <select aria-labelledby={policyLabel} value={custom ? "custom" : "system"} disabled={pending} onChange={(event) => { setCustom(event.target.value === "custom"); setConfirm(undefined); }}>
        <option value="system">System default ({systemAge(policy.default_age_seconds)})</option>
        <option value="custom">Custom retention</option>
      </select>
    </label>
    {custom && <label>Retention days
      <input name="days" type="number" required min={1} max={policy.max_days} step={1} disabled={pending} defaultValue={policy.retention_days ?? Math.max(1, Math.ceil(policy.default_age_seconds / daySeconds))} onChange={() => setConfirm(undefined)} />
    </label>}
    <div><button type="submit" className={primaryButtonClass} disabled={pending || confirm !== undefined}>{pending ? "Saving…" : "Save retention"}</button></div>
    {confirm !== undefined && <div className="col-span-full rounded-lg border border-warning/40 bg-warn-bg p-3 text-sm text-warn-fg">
      <p className="m-0">Data older than the new retention period will be permanently deleted during cleanup.</p>
      <div className="mt-2 flex flex-wrap gap-2">
        <button type="button" className={primaryButtonClass} onClick={() => { onSave(confirm); setConfirm(undefined); }}>Confirm shorter retention</button>
        <button type="button" className={secondaryButtonClass} onClick={() => setConfirm(undefined)}>Cancel</button>
      </div>
    </div>}
  </form>;
}
