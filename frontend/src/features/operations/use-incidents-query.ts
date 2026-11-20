import { queryString, pageItems, usePagedQuery } from "../../shared/api";
import { useAuth } from "../../shared/session";
import type { Incident } from "../../shared/types";

export type IncidentFilters = {
  service?: string;
  status?: string;
  from: string;
  to: string;
};

export function useIncidentsQuery(filters: IncidentFilters) {
  const { credential } = useAuth();
  const path = `/api/v1/incidents?${queryString({
    service: filters.service || undefined,
    status: filters.status || undefined,
    from: filters.from,
    to: filters.to,
  })}`;

  const query = usePagedQuery<Incident>(["incidents"], path, credential);
  const incidents = pageItems(query.data?.pages);

  const openCount = incidents.filter((i) => i.status.toUpperCase() === "OPEN").length;
  const investigatingCount = incidents.filter((i) => i.status.toUpperCase() === "INVESTIGATING").length;
  const resolvedCount = incidents.filter((i) => i.status.toUpperCase() === "RESOLVED").length;
  const ignoredCount = incidents.filter((i) => i.status.toUpperCase() === "IGNORED").length;
  const totalOccurrences = incidents.reduce((sum, i) => sum + (i.occurrence_count || 0), 0);

  return {
    query,
    incidents,
    counts: {
      open: openCount,
      investigating: investigatingCount,
      resolved: resolvedCount,
      ignored: ignoredCount,
      totalOccurrences,
    },
  };
}

