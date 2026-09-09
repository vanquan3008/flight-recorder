import { Gauge } from "lucide-react";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import { DetectionsPage, SecurityEventPage, SecurityEventsPage, SourcesPage } from "../features/security/page";
import { IncidentPage, MachineLogsPage, OperationsPage } from "../features/operations/page";
import { OverviewPage } from "../features/overview/page";
import { RulesPage } from "../features/rules/page";
import { TraceDetailPage, TracesPage } from "../features/traces/page";

export function AppRoutes({ isAuthenticated }: { isAuthenticated: boolean }) {
  if (!isAuthenticated) return <Welcome />;
  return <Routes>
    <Route path="/overview" element={<OverviewPage />} />
    <Route path="/traces" element={<TracesPage />} />
    <Route path="/traces/:traceID" element={<TraceDetailPage />} />
    <Route path="/events" element={<SecurityEventsPage />} />
    <Route path="/events/:eventID" element={<SecurityEventPage />} />
    <Route path="/machine-logs" element={<MachineLogsPage />} />
    <Route path="/detections" element={<DetectionsPage />} />
    <Route path="/incidents" element={<OperationsPage />} />
    <Route path="/incidents/:incidentID" element={<IncidentPage />} />
    <Route path="/telemetry" element={<Navigate replace to="/incidents" />} />
    <Route path="/telemetry/:incidentID" element={<LegacyIncidentRedirect />} />
    <Route path="/rules" element={<RulesPage />} />
    <Route path="/sources" element={<SourcesPage />} />
    <Route path="*" element={<Navigate replace to="/overview" />} />
  </Routes>;
}

function LegacyIncidentRedirect() {
  const { incidentID = "" } = useParams();
  return <Navigate replace to={`/incidents/${incidentID}`} />;
}

function Welcome() {
  return <section className="welcome"><Gauge size={32} /><h1>Welcome to TracePilot</h1><p>Connect a read-scoped API key above to inspect traces, security events, and incidents.</p><span>Your key stays in this browser session only.</span></section>;
}
