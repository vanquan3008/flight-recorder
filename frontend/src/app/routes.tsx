import { lazy, Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Welcome } from "./welcome";

const FlowPage = lazy(() => import("../features/flow/page").then((m) => ({ default: m.FlowPage })));
const IncidentPage = lazy(() => import("../features/operations/page").then((m) => ({ default: m.IncidentPage })));
const OperationsPage = lazy(() => import("../features/operations/page").then((m) => ({ default: m.OperationsPage })));
const LogsPage = lazy(() => import("../features/logs/page").then((m) => ({ default: m.LogsPage })));
const OverviewPage = lazy(() => import("../features/overview/page").then((m) => ({ default: m.OverviewPage })));
const PerformancePage = lazy(() => import("../features/performance/page").then((m) => ({ default: m.PerformancePage })));
const SettingsPage = lazy(() => import("../features/settings/page").then((m) => ({ default: m.SettingsPage })));
const TopologyPage = lazy(() => import("../features/topology/page").then((m) => ({ default: m.TopologyPage })));
const TraceDetailPage = lazy(() => import("../features/traces/page").then((m) => ({ default: m.TraceDetailPage })));
const TracesPage = lazy(() => import("../features/traces/page").then((m) => ({ default: m.TracesPage })));

function RouteLoading() {
  return (
    <div className="flex items-center justify-center min-h-[40vh] p-8 text-sky-500/70 dark:text-sky-400/70" role="status" aria-label="Loading route">
      <div className="w-7 h-7 rounded-full border-2 border-current border-t-transparent animate-spin" />
    </div>
  );
}

export function AppRoutes({ isAuthenticated }: { isAuthenticated: boolean }) {
  if (!isAuthenticated) {
    return (
      <Suspense fallback={<RouteLoading />}>
        <Routes>
          <Route path="/flow" element={<FlowPage />} />
          <Route path="*" element={<Welcome />} />
        </Routes>
      </Suspense>
    );
  }
  return (
    <Suspense fallback={<RouteLoading />}>
      <Routes>
        <Route path="/overview" element={<OverviewPage />} />
        <Route path="/flow" element={<FlowPage />} />
        <Route path="/performance" element={<PerformancePage />} />
        <Route path="/traces" element={<TracesPage />} />
        <Route path="/traces/:traceID" element={<TraceDetailPage />} />
        <Route path="/logs" element={<LogsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/topology" element={<TopologyPage />} />
        <Route path="/incidents" element={<OperationsPage />} />
        <Route path="/incidents/:incidentID" element={<IncidentPage />} />
        <Route path="*" element={<Navigate replace to="/overview" />} />
      </Routes>
    </Suspense>
  );
}
