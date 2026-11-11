import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { KeyboardEvent } from "react";
import { useLocation, useParams, useSearchParams } from "react-router-dom";
import { useAuth } from "../../shared/session";
import { api } from "../../shared/api";
import type { Incident } from "../../shared/types";
import { ErrorState, Loading } from "../../shared/ui";
import { IncidentAnalysisPanels } from "./incident-analysis";
import { IncidentHeader } from "./incident-header";
import { IncidentRecords } from "./incident-records";

const tabs = [
  { id: "analysis", label: "Investigation" }, { id: "occurrences", label: "Occurrences" },
  { id: "telemetry", label: "Linked telemetry" }, { id: "history", label: "Status history" },
] as const;
type Tab = typeof tabs[number]["id"];

export function IncidentPage() {
  const { incidentID = "" } = useParams();
  return <IncidentWorkspace key={incidentID} incidentID={incidentID} />;
}

function IncidentWorkspace({ incidentID }: { incidentID: string }) {
  const { credential } = useAuth();
  const client = useQueryClient();
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const tab = tabs.find((item) => item.id === params.get("tab"))?.id ?? "analysis";
  const path = `/api/v1/incidents/${incidentID}`;
  const incident = useQuery({ queryKey: ["incident", incidentID], queryFn: ({ signal }) => api<Incident>(path, credential, { signal }) });
  const mutation = useMutation({
    mutationFn: (status: string) => api<Incident>(path, credential, { method: "PATCH", body: JSON.stringify({ status }) }),
    onSuccess: async (updated) => {
      client.setQueryData(["incident", updated.id], updated);
      await Promise.all([["incident", updated.id], ["incident-changes", updated.id], ["incidents"], ["overview"]].map((queryKey) => client.invalidateQueries({ queryKey })));
    },
  });
  const selectTab = (next: Tab) => {
    const search = new URLSearchParams(params);
    if (next === "analysis") search.delete("tab"); else search.set("tab", next);
    setParams(search, { replace: true, state: location.state });
  };
  const onTabKey = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1
      : event.key === "ArrowRight" ? (index + 1) % tabs.length : event.key === "ArrowLeft" ? (index + tabs.length - 1) % tabs.length : -1;
    if (next < 0) return;
    event.preventDefault(); selectTab(tabs[next].id);
    event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
  };
  if (incident.isPending) return <Loading page />;
  if (incident.isError) return <ErrorState error={incident.error} retry={() => void incident.refetch()} />;
  return <div className="flex min-w-0 flex-col gap-4 pb-4">
    <IncidentHeader incident={incident.data} pending={mutation.isPending} error={mutation.error} onStatus={(status) => mutation.mutate(status)} />
    <div className="flex gap-1 overflow-x-auto border-b border-line px-1" role="tablist" aria-label="Incident investigation views">
      {tabs.map((item, index) => <button key={item.id} type="button" role="tab" id={`incident-tab-${item.id}`} aria-controls="incident-panel" aria-selected={tab === item.id}
        tabIndex={tab === item.id ? 0 : -1} onClick={() => selectTab(item.id)} onKeyDown={(event) => onTabKey(event, index)}
        className={`inline-flex shrink-0 items-center gap-2 border-b-2 px-3 py-3 text-xs font-medium transition-colors ${tab === item.id ? "border-accent text-accent" : "border-transparent text-muted hover:text-text"}`}>
        {item.label}{item.id === "occurrences" && <span className="rounded bg-neutral-bg px-1.5 py-px text-[10px] tabular-nums">{incident.data.occurrence_count.toLocaleString("en")}</span>}
      </button>)}
    </div>
    <div id="incident-panel" role="tabpanel" aria-labelledby={`incident-tab-${tab}`} tabIndex={0} className="min-w-0">
      {tab === "analysis" ? <IncidentAnalysisPanels key={incidentID} incidentID={incidentID} />
        : <IncidentRecords key={`${incidentID}:${tab}`} incidentID={incidentID} kind={tab} totalOccurrences={incident.data.occurrence_count} />}
    </div>
  </div>;
}
