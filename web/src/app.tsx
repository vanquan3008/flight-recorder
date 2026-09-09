import { QueryClient, QueryClientProvider, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type ReactNode } from "react";
import { Activity, BellRing, ChevronRight, ClipboardList, Gauge, KeyRound, Menu, Network, Radar, ShieldAlert, SlidersHorizontal, TerminalSquare, X } from "lucide-react";
import { HashRouter, Navigate, NavLink, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import { api } from "./api";
import { AuthContext } from "./auth";
import { OverviewPage } from "./pages/overview";
import { OperationsPage, IncidentPage, MachineLogsPage } from "./pages/operations";
import { RulesPage } from "./pages/rules";
import { DetectionsPage, SecurityEventPage, SecurityEventsPage, SourcesPage } from "./pages/security";
import { TraceDetailPage, TracesPage } from "./pages/traces";
import type { Page, SecurityEvent } from "./types";

const storageKey = "tracepilot-api-key";

function Shell() {
  const queryClient = useQueryClient();
  const [apiKey, setAPIKey] = useState(() => sessionStorage.getItem(storageKey) || "");
  const [draftKey, setDraftKey] = useState(apiKey);
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const connection = useQuery({
    queryKey: ["connection"],
    enabled: Boolean(apiKey),
    retry: false,
    queryFn: ({ signal }) => api<Page<SecurityEvent>>("/api/v1/security/events?limit=1", apiKey, { signal }),
  });
  useEffect(() => setMenuOpen(false), [location.pathname]);
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => event.key === "Escape" && setMenuOpen(false);
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
  const connect = (value: string) => {
    const key = value.trim();
    if (!key) return;
    queryClient.clear();
    sessionStorage.setItem(storageKey, key);
    setAPIKey(key);
    setDraftKey(key);
    navigate("/overview");
  };
  const disconnect = () => {
    queryClient.clear();
    sessionStorage.removeItem(storageKey);
    setAPIKey("");
    setDraftKey("");
  };
  const status = !apiKey ? "API key required" : connection.isPending ? "Checking API" : connection.isSuccess ? "Connected" : "Connection failed";
  const statusClass = !apiKey ? "" : connection.isSuccess ? "connected" : connection.isError ? "failed" : "checking";
  return <AuthContext.Provider value={{ apiKey, connect, disconnect }}>
    <header className="topbar">
      <div className="topbar-title"><button className="icon-button mobile-menu" onClick={() => setMenuOpen((open) => !open)} aria-label="Toggle navigation" aria-expanded={menuOpen}>{menuOpen ? <X /> : <Menu />}</button><NavLink className="brand" to="/overview">TracePilot <span>Console</span></NavLink></div>
      <form className="key-form" onSubmit={(event) => { event.preventDefault(); connect(draftKey); }}>
        <span className={`connection-dot ${statusClass}`} title={status} />
        <label className="sr-only" htmlFor="api-key">API key</label>
        <input id="api-key" type="password" autoComplete="off" placeholder="Paste API key" value={draftKey} onChange={(event) => setDraftKey(event.target.value)} />
        {apiKey ? <button type="button" className="button-secondary" onClick={disconnect}>Disconnect</button> : <button type="submit"><KeyRound size={15} /> Connect</button>}
      </form>
    </header>
    <main>
      <Navigation open={menuOpen} />
      <section id="content"><RequireKey><Routes>
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
      </Routes></RequireKey></section>
    </main>
  </AuthContext.Provider>;
}

function LegacyIncidentRedirect() {
  const { incidentID = "" } = useParams();
  return <Navigate replace to={`/incidents/${incidentID}`} />;
}

function RequireKey({ children }: { children: ReactNode }) {
  const key = sessionStorage.getItem(storageKey);
  if (key) return <>{children}</>;
  return <section className="welcome"><Gauge size={32} /><h1>Welcome to TracePilot</h1><p>Connect a read-scoped API key above to inspect traces, security events, and incidents.</p><span>Your key stays in this browser session only.</span></section>;
}

function Navigation({ open }: { open: boolean }) {
  const groups = [
    ["Monitor", [["/overview", "Overview", Gauge], ["/traces", "Trace explorer", Activity], ["/events", "Security events", ShieldAlert], ["/machine-logs", "Machine logs", TerminalSquare]]],
    ["Respond", [["/detections", "Detections", Radar], ["/incidents", "Incidents", BellRing]]],
    ["Configure", [["/rules", "Detection rules", SlidersHorizontal], ["/sources", "Sources", Network]]],
  ] as const;
  return <nav aria-label="Console sections" className={open ? "open" : ""}>{groups.map(([label, links]) => <div className="nav-section" key={label}><small>{label}</small>{links.map(([to, name, Icon]) => <NavLink to={to} key={to} end={to === "/overview"}><Icon size={17} /><span>{name}</span><ChevronRight className="nav-chevron" size={15} /></NavLink>)}</div>)}<div className="nav-footer"><ClipboardList size={15} /> <span>Operator console</span></div></nav>;
}

export function App() {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: (count, error) => !(error instanceof Error && "status" in error && (error as { status: number }).status < 500) && count < 1, staleTime: 15_000 } } }));
  return <QueryClientProvider client={queryClient}><HashRouter><Shell /></HashRouter></QueryClientProvider>;
}
