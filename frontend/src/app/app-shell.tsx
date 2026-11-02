import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, BellRing, ClipboardList, Gauge, GitBranch, LogOut, Menu, Network, PanelLeftClose, PanelLeftOpen, ScrollText, Settings, TrendingUp, X } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { api, APIError, onSessionRefreshed, refreshAccess, sessionCredential, setAccessToken, setProjectContext } from "../shared/api";
import type { Page, TraceSummary } from "../shared/types";
import { AuthContext } from "./auth";
import { ChangePasswordGate, NoProject } from "./access-gates";
import { AppRoutes } from "./routes";
import { projectStorageKey, type SessionView, signedInHint } from "../shared/session";
import { ThemeToggle } from "./theme";

function BrandIcon() {
  return <div className="brand-icon relative flex items-center justify-center w-[34px] h-[34px] rounded-[11px] bg-gradient-to-tr from-sky-400 via-sky-500 to-blue-600 ring-1 ring-white/60 ring-inset shadow-[0_4px_18px_rgba(14,165,233,0.5)] overflow-hidden shrink-0 group-hover:scale-105 active:scale-95 transition-transform duration-200">
    <div className="absolute inset-x-0 top-0 h-1/2 bg-gradient-to-b from-white/40 to-transparent pointer-events-none" />
    <svg className="w-[18px] h-[18px] text-white relative z-10 drop-shadow-sm" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M17.8 19.2 16 11l3.5-3.5C21 6 21.5 4 21 3c-1-.5-3 0-4.5 1.5L13 8 4.8 6.2c-.5-.1-.9.1-1.1.5l-.3.5c-.2.5-.1 1 .3 1.3L9 12l-2 3H4l-1 1 3 2 2 3 1-1v-3l3-2 3.5 5.3c.3.4.8.5 1.3.3l.5-.3c.4-.2.6-.6.5-1.1z" />
    </svg>
  </div>;
}

// What the console keeps of a session: the access token is held by `shared/api` in memory and is never in React state.
const withoutToken = (view: SessionView): SessionView => ({ ...view, access_token: undefined });

export function AppShell() {
  const queryClient = useQueryClient();
  const [menuOpen, setMenuOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("tracepilot-sidebar-collapsed") === "true");
  const location = useLocation();
  const navigate = useNavigate();
  // The access token lives in memory, so a reload starts signed out. A hint left at sign-in says a refresh
  // cookie may exist, so the console asks for a new token before deciding what to show.
  const [session, setSession] = useState<SessionView | null>(null);
  const [expiresAt, setExpiresAt] = useState<number>(0);
  const [booting, setBooting] = useState(() => localStorage.getItem(signedInHint) === "1");
  const [projectID, setProjectID] = useState(() => localStorage.getItem(projectStorageKey) || "");
  // Set once the person signed out: a refresh that was already in flight must not sign them back in.
  const ended = useRef(false);

  const clearSession = useCallback(() => {
    ended.current = true;
    setAccessToken("");
    queryClient.clear();
    localStorage.removeItem(signedInHint);
    setSession(null);
  }, [queryClient]);

  const adopt = useCallback((view: SessionView) => {
    if (ended.current) return;
    if (view.access_token) setAccessToken(view.access_token);
    if (view.expires_at) setExpiresAt(Date.parse(view.expires_at));
    localStorage.setItem(signedInHint, "1");
    setSession(withoutToken(view));
  }, []);

  // Register the refresh listener once; adopt and clearSession are stable callbacks.
  useEffect(() => {
    onSessionRefreshed(adopt);
    return () => onSessionRefreshed(undefined);
  }, [adopt]);

  useEffect(() => {
    if (!booting) return;
    let cancelled = false;
    void refreshAccess().then((view) => {
      if (cancelled) return;
      if (!view) localStorage.removeItem(signedInHint);
      setBooting(false);
    });
    return () => { cancelled = true; };
  }, [booting]);

  // Renew the access token a minute before it runs out, so a person at the keyboard never sees it expire.
  useEffect(() => {
    if (!session || !expiresAt) return;
    // Never longer than the access token's own life: a timer cannot hold more than ~24 days anyway.
    const wait = Math.min(Math.max(5_000, expiresAt - Date.now() - 60_000), 20 * 60_000);
    const timer = window.setTimeout(() => void refreshAccess().then((view) => { if (!view) clearSession(); }), wait);
    return () => window.clearTimeout(timer);
  }, [session, expiresAt, clearSession]);

  const current = session ? session.memberships.find((membership) => membership.project.id === projectID) ?? session.memberships[0] : undefined;

  // Keep module-level request context in sync with the active project. A layout effect, not a passive one:
  // pages below start their queries in passive effects, which run before this component's own, so they
  // would send their first requests without the project.
  useLayoutEffect(() => {
    setProjectContext({ project: current?.project.id ?? "" });
  }, [current?.project.id]);

  const credential = session ? sessionCredential : "";
  const mustChangePassword = Boolean(session?.user.must_change_password);
  const needsProject = Boolean(session) && !current;
  const connection = useQuery({
    queryKey: ["connection", credential, current?.project.id],
    enabled: Boolean(credential) && !mustChangePassword && !needsProject,
    retry: false,
    queryFn: ({ signal }) => api<Page<TraceSummary>>("/api/v1/traces?limit=1", credential, { signal }),
  });

  useEffect(() => setMenuOpen(false), [location.pathname]);
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => event.key === "Escape" && setMenuOpen(false);
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const signIn = useCallback((view: SessionView) => {
    queryClient.clear();
    ended.current = false;
    adopt(view);
    navigate("/overview");
  }, [queryClient, adopt, navigate]);

  const signOut = useCallback(() => {
    // The server ends the refresh session and clears its cookie; a failed request still ends this browser's view of it.
    void api<void>("/api/v1/auth/logout", "", { method: "POST" }).catch(() => undefined).finally(clearSession);
  }, [clearSession]);

  const reloadSession = useCallback(async () => {
    const view = await api<SessionView>("/api/v1/auth/me", credential);
    setSession(withoutToken(view));
  }, [credential]);

  const switchProject = useCallback((id: string) => {
    queryClient.clear();
    localStorage.setItem(projectStorageKey, id);
    setProjectID(id);
  }, [queryClient]);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((c) => {
      const next = !c;
      localStorage.setItem("tracepilot-sidebar-collapsed", String(next));
      return next;
    });
  }, []);

  const isAuthenticated = Boolean(session);
  const showSidebar = isAuthenticated || location.pathname === "/flow";

  // A session the server no longer accepts (refresh failed, signed out elsewhere, account disabled) ends here.
  useEffect(() => {
    if (session && connection.error instanceof APIError && connection.error.status === 401) clearSession();
  }, [session, connection.error, clearSession]);

  const status = !isAuthenticated ? "Signed out" : (mustChangePassword || needsProject) ? "Signed in" : connection.isPending ? "Checking API" : connection.isSuccess ? "Connected" : "Connection failed";
  const statusDotBg = !isAuthenticated ? "bg-slate-400" : connection.isSuccess ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.7)]" : connection.isError ? "bg-rose-500" : "bg-amber-500 animate-pulse";

  const sessionState = session ? { user: session.user, memberships: session.memberships, project: current, role: session.user.platform_admin ? "owner" as const : current?.role, switchProject, refresh: reloadSession, adopt, signOut } : undefined;
  if (booting) return <div className="app-shell relative flex h-screen items-center justify-center" role="status"><span className="loading-label">Restoring your session…</span></div>;

  return <AuthContext.Provider value={{ credential, isAuthenticated, signIn, session: sessionState }}>
    <div className="app-shell relative flex flex-col h-screen max-h-screen overflow-hidden">

      {/* Floating Dynamic Island Topbar */}
      <header className="topbar relative z-30 flex h-[58px] shrink-0 items-center justify-between gap-4 mx-3 md:mx-4 mt-2.5 px-4 md:px-6 rounded-[24px] backdrop-blur-3xl bg-white/75 dark:bg-[#08182f]/65 border border-white/80 dark:border-white/12 shadow-[inset_0_1px_1px_rgba(255,255,255,0.7),0_10px_30px_rgba(0,0,0,0.06)] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.18),0_16px_40px_rgba(0,0,0,0.5)]">
        <div className="topbar-title flex min-w-0 items-center gap-3.5">
          {showSidebar && <button className="icon-button mobile-menu md:hidden flex items-center justify-center w-8 h-8 rounded-full bg-white dark:bg-sky-950/60 border border-slate-200 dark:border-white/10 text-sky-800 dark:text-sky-300 p-0 shadow-none cursor-pointer active:scale-90 transition-transform" onClick={() => setMenuOpen((open) => !open)} aria-label="Toggle navigation" aria-expanded={menuOpen}>{menuOpen ? <X size={16} /> : <Menu size={16} />}</button>}
          <NavLink className="brand group flex items-center gap-2.5 text-slate-950 dark:text-white font-semibold text-[15px] tracking-tight no-underline whitespace-nowrap" to="/overview">
            <BrandIcon />
            <span className="text-slate-950 dark:text-white font-semibold tracking-tight">TracePilot</span>
            <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold tracking-wider uppercase bg-sky-500/15 dark:bg-sky-400/20 border border-sky-500/35 dark:border-sky-400/30 text-sky-800 dark:text-sky-300 shadow-xs">Console</span>
          </NavLink>
        </div>
        <div className="topbar-actions flex min-w-0 items-center gap-3">
          <ThemeToggle />
          {session ? <SessionControls name={session.user.display_name} memberships={session.memberships} current={current?.project.id} onSwitch={switchProject} onSignOut={signOut} status={status} dot={statusDotBg} />
            : <NavLink to="/flow" className="inline-flex items-center gap-1.5 text-xs font-bold text-slate-700 dark:text-sky-300 hover:text-slate-950 dark:hover:text-white px-3.5 py-1.5 rounded-full backdrop-blur-2xl bg-white/70 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-xs active:scale-95 transition-all no-underline"><GitBranch size={13} /> Architecture</NavLink>}
        </div>
      </header>
      <main className={`flex-1 min-h-0 overflow-hidden w-full relative z-10 ${showSidebar ? `grid gap-3.5 p-3 md:p-4 transition-[grid-template-columns] duration-200 ease-in-out ${collapsed ? "grid-cols-[56px_minmax(0,1fr)] sidebar-collapsed" : "grid-cols-[208px_minmax(0,1fr)]"}` : "flex flex-col"}`}>
        {showSidebar && <Navigation open={menuOpen} collapsed={collapsed} onToggle={toggleCollapsed} />}
        <section id="content" className={`w-full h-full min-w-0 flex flex-col ${showSidebar ? "overflow-y-auto overflow-x-hidden" : "overflow-hidden items-center justify-center"}`}>
          <div className="page-flow flex-1 flex flex-col min-h-0 w-full">
            {mustChangePassword ? <ChangePasswordGate onChanged={adopt} /> : needsProject ? <NoProject /> : <AppRoutes isAuthenticated={isAuthenticated} />}
          </div>
        </section>
      </main>
    </div>
  </AuthContext.Provider>;
}

function SessionControls({ name, memberships, current, onSwitch, onSignOut, status, dot }: { name: string; memberships: SessionView["memberships"]; current?: string; onSwitch: (id: string) => void; onSignOut: () => void; status: string; dot: string }) {
  return <div className="session-controls flex items-center gap-2.5">
    <div className="connection-pill hidden sm:inline-flex items-center gap-2 px-3 py-1.5 rounded-full backdrop-blur-2xl bg-white/70 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-xs">
      <span className={`w-2 h-2 rounded-full ${dot}`} aria-hidden="true" />
      <span className="connection-label text-slate-900 dark:text-sky-300 text-xs font-bold whitespace-nowrap" role="status">{status}</span>
    </div>
    {memberships.length > 0 && <>
      <label className="sr-only" htmlFor="project-switcher">Project</label>
      <select id="project-switcher" className="project-switcher" value={current ?? ""} onChange={(event) => onSwitch(event.target.value)}>
        {memberships.map(({ project }) => <option key={project.id} value={project.id}>{project.name}</option>)}
      </select>
    </>}
    <span className="session-user" title="Signed in">{name}</span>
    <button type="button" className="disconnect-btn inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-xs font-bold text-rose-700 dark:text-rose-400 backdrop-blur-2xl bg-rose-50/80 dark:bg-rose-500/10 border border-rose-300/80 dark:border-rose-500/20 hover:bg-rose-100 dark:hover:bg-rose-500/20 active:scale-95 transition-all cursor-pointer shadow-none" onClick={onSignOut}><LogOut size={13} aria-hidden="true" /> Sign out</button>
  </div>;
}

function Navigation({ open, collapsed, onToggle }: { open: boolean; collapsed: boolean; onToggle: () => void }) {
  const groups = [
    ["Observe", [["/overview", "Overview", Gauge], ["/performance", "Performance", TrendingUp], ["/traces", "Trace explorer", Activity], ["/logs", "Log explorer", ScrollText], ["/topology", "Service topology", Network]]],
    ["Respond", [["/incidents", "Incidents", BellRing]]],
    ["Manage", [["/settings", "Settings", Settings], ["/flow", "How it works", GitBranch]]],
  ] as const;

  return <nav aria-label="Console sections" className={`h-full max-h-full flex flex-col gap-2 p-2.5 rounded-[22px] backdrop-blur-3xl bg-white/75 dark:bg-[#091b34]/70 border border-white/80 dark:border-white/12 shadow-[inset_0_1px_1px_rgba(255,255,255,0.7),0_16px_48px_rgba(0,0,0,0.08)] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.18),0_20px_50px_rgba(0,0,0,0.55)] shrink-0 overflow-hidden transition-all duration-200 ease-in-out ${open ? "open" : ""} ${collapsed ? "w-[56px] collapsed" : "w-[208px]"}`}>
    <div className={`flex items-center min-h-[32px] pb-2 border-b border-black/5 dark:border-white/10 ${collapsed ? "justify-center" : "justify-between px-1"}`}>
      {!collapsed && <span className="text-[10.5px] font-black uppercase tracking-widest text-slate-800 dark:text-sky-300 select-none">Console</span>}
      <button
        type="button"
        className="w-7 h-7 rounded-xl flex items-center justify-center text-slate-700 dark:text-sky-300 hover:text-slate-950 dark:hover:text-white hover:bg-white/60 dark:hover:bg-white/10 active:scale-90 transition-all cursor-pointer p-0 shadow-none border-0"
        style={{ background: "transparent", boxShadow: "none" }}
        onClick={onToggle}
        title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      >
        {collapsed ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
      </button>
    </div>
    {groups.map(([label, links]) => <div className="grid gap-1" key={label}>
      {!collapsed && <small className="px-3 py-1 text-[10px] font-black uppercase tracking-wider text-slate-700 dark:text-sky-400">{label}</small>}
      {links.map(([to, name, Icon]) => <NavLink
        to={to}
        key={to}
        title={name}
        end={to === "/overview"}
        className={({ isActive }) => `flex items-center gap-2.5 rounded-xl text-[13px] font-semibold transition-all duration-150 no-underline ${
          collapsed ? "w-9 h-9 justify-center p-0 mx-auto" : "px-3 py-2"
        } ${
          isActive
            ? "bg-gradient-to-r from-sky-400 via-sky-500 to-blue-600 text-white font-bold shadow-[0_6px_20px_rgba(14,165,233,0.4),inset_0_1px_1px_rgba(255,255,255,0.4)] scale-[1.01] active:scale-95"
            : "text-slate-700 dark:text-sky-200/90 hover:text-sky-700 dark:hover:text-white hover:bg-white/60 dark:hover:bg-white/10 active:scale-95"
        }`}
      >
        <Icon size={16} className="shrink-0" />
        {!collapsed && <span>{name}</span>}
      </NavLink>)}
    </div>)}
    <div className={`flex items-center gap-2 mt-auto pt-3 border-t border-black/5 dark:border-white/10 text-slate-700 dark:text-sky-400 text-[11px] font-bold ${collapsed ? "justify-center" : "px-2"}`} title="Operator console">
      <ClipboardList size={14} className="shrink-0" />
      {!collapsed && <span>Operator console</span>}
    </div>
  </nav>;
}
