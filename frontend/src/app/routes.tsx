import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { Welcome } from "./welcome";
import { FlowPage } from "../features/flow/page";
import { IncidentPage, OperationsPage } from "../features/operations/page";
import { LogsPage } from "../features/logs/page";
import { OverviewPage } from "../features/overview/page";
import { SettingsPage } from "../features/settings/page";
import { TopologyPage } from "../features/topology/page";
import { TraceDetailPage, TracesPage } from "../features/traces/page";

export function AppRoutes({ isAuthenticated }: { isAuthenticated: boolean }) {
  if (!isAuthenticated) return <Routes><Route path="/flow" element={<FlowPage />} /><Route path="*" element={<Welcome />} /></Routes>;
  return <Routes>
    <Route path="/overview" element={<OverviewPage />} />
    <Route path="/flow" element={<FlowPage />} />
    <Route path="/traces" element={<TracesPage />} />
    <Route path="/traces/:traceID" element={<TraceDetailPage />} />
    <Route path="/logs" element={<LogsPage />} />
    <Route path="/settings" element={<SettingsPage />} />
    <Route path="/topology" element={<TopologyPage />} />
    <Route path="/incidents" element={<OperationsPage />} />
    <Route path="/incidents/:incidentID" element={<IncidentPage />} />
    <Route path="/telemetry" element={<Navigate replace to="/incidents" />} />
    <Route path="/telemetry/:incidentID" element={<LegacyIncidentRedirect />} />
    <Route path="*" element={<Navigate replace to="/overview" />} />
  </Routes>;
}

function LegacyIncidentRedirect() {
  const { incidentID = "" } = useParams();
  return <Navigate replace to={`/incidents/${incidentID}`} />;
}
