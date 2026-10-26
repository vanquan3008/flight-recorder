import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, BellRing, ChevronRight, ClipboardList, Gauge, GitBranch, KeyRound, Menu, Network, ScrollText, Settings, X } from "lucide-react";
import { useEffect, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { api, APIError } from "../shared/api";
import type { Page, TraceSummary } from "../shared/types";
import { errorGuidance } from "../shared/ui";
import { AuthContext, apiKeyStorageKey } from "./auth";
import { AppRoutes } from "./routes";
import { ThemeToggle } from "./theme";

export function AppShell() {
  const queryClient = useQueryClient();
  const [apiKey, setAPIKey] = useState(() => sessionStorage.getItem(apiKeyStorageKey) || "");
  const [draftKey, setDraftKey] = useState(apiKey);
  const [menuOpen, setMenuOpen] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();
  const connection = useQuery({
    queryKey: ["connection", apiKey],
    enabled: Boolean(apiKey),
    retry: false,
    queryFn: ({ signal }) => api<Page<TraceSummary>>("/api/v1/traces?limit=1", apiKey, { signal }),
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
    sessionStorage.setItem(apiKeyStorageKey, key);
    setAPIKey(key);
    setDraftKey(key);
    navigate("/overview");
  };
  const disconnect = () => {
    queryClient.clear();
    sessionStorage.removeItem(apiKeyStorageKey);
    setAPIKey("");
    setDraftKey("");
  };
  const isAuthenticated = Boolean(apiKey);
  const rejected = connection.error instanceof APIError && (connection.error.status === 401 || connection.error.status === 403);
  const status = !isAuthenticated ? "API key required" : connection.isPending ? "Checking API" : connection.isSuccess ? "Connected" : rejected ? "Key not accepted" : "Connection failed";
  const statusClass = !isAuthenticated ? "" : connection.isSuccess ? "connected" : connection.isError ? "failed" : "checking";
  return <AuthContext.Provider value={{ apiKey, isAuthenticated, connect, disconnect }}>
    <header className="topbar">
      <div className="topbar-title"><button className="icon-button mobile-menu" onClick={() => setMenuOpen((open) => !open)} aria-label="Toggle navigation" aria-expanded={menuOpen}>{menuOpen ? <X /> : <Menu />}</button><NavLink className="brand" to="/overview">TracePilot <span>Console</span></NavLink></div>
      <div className="topbar-actions">
        <ThemeToggle />
        <form className="key-form" onSubmit={(event) => { event.preventDefault(); connect(draftKey); }}>
          <span className={`connection-dot ${statusClass}`} aria-hidden="true" /><span className="connection-label" role="status">{status}</span>
          <label className="sr-only" htmlFor="api-key">API key</label>
          <input id="api-key" type="password" autoComplete="off" placeholder="Paste API key" value={draftKey} onChange={(event) => setDraftKey(event.target.value)} />
          {isAuthenticated ? <button type="button" className="button-secondary" onClick={disconnect}>Disconnect</button> : <button type="submit"><KeyRound size={15} /> Connect</button>}
        </form>
      </div>
    </header>
    <main>
      <Navigation open={menuOpen} />
      <section id="content">
        {isAuthenticated && connection.isError && <ConnectionBanner error={connection.error} onRetry={() => void connection.refetch()} onDisconnect={disconnect} />}
        <AppRoutes isAuthenticated={isAuthenticated} />
      </section>
    </main>
  </AuthContext.Provider>;
}

function Navigation({ open }: { open: boolean }) {
  const groups = [
    ["Observe", [["/overview", "Overview", Gauge], ["/traces", "Trace explorer", Activity], ["/logs", "Log explorer", ScrollText], ["/topology", "Service topology", Network]]],
    ["Respond", [["/incidents", "Incidents", BellRing]]],
    ["Manage", [["/settings", "Settings", Settings], ["/flow", "How it works", GitBranch]]],
  ] as const;
  return <nav aria-label="Console sections" className={open ? "open" : ""}>{groups.map(([label, links]) => <div className="nav-section" key={label}><small>{label}</small>{links.map(([to, name, Icon]) => <NavLink to={to} key={to} end={to === "/overview"}><Icon size={17} /><span>{name}</span><ChevronRight className="nav-chevron" size={15} /></NavLink>)}</div>)}<div className="nav-footer"><ClipboardList size={15} /> <span>Operator console</span></div></nav>;
}

function ConnectionBanner({ error, onRetry, onDisconnect }: { error: unknown; onRetry: () => void; onDisconnect: () => void }) {
  const { title, message } = errorGuidance(error);
  const rejected = error instanceof APIError && (error.status === 401 || error.status === 403);
  return <div className="message error banner" role="alert"><div><strong>{title}</strong><p>{message}</p></div>
    {rejected ? <button type="button" onClick={onDisconnect}>Disconnect</button> : <button type="button" onClick={onRetry}>Retry</button>}</div>;
}
