import { useQuery, useQueryClient } from "@tanstack/react-query";
import { GitBranch, Menu, X } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { api, APIError, onSessionRefreshed, refreshAccess, sessionCredential, setAccessToken, setProjectContext } from "../shared/api";
import { ChangePasswordGate, NoProject } from "./access-gates";
import { AppRoutes } from "./routes";
import { AuthContext, projectStorageKey, type ProjectDetail, type SessionView, signedInHint } from "../shared/session";
import { ThemeToggle } from "./theme";
import { Navigation } from "./navigation";
import { SessionControls } from "./session-controls";
import { ErrorState } from "../shared/ui";

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
  const [params, setParams] = useSearchParams();
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

  const requestedProject = params.get("project");
  const current = session ? session.memberships.find(({ project }) => project.id === (requestedProject || projectID) || project.slug === requestedProject)
    ?? (requestedProject ? undefined : session.memberships[0]) : undefined;
  const unknownProject = Boolean(session && requestedProject && !current);
  const environmentStorageKey = `tracepilot-environment:${current?.project.id ?? ""}`;
  const environment = params.get("environment") ?? localStorage.getItem(environmentStorageKey) ?? "";
  const scopeKey = `${current?.project.id ?? ""}:${environment}`;
  const previousScope = useRef(scopeKey);

  // Keep module-level request context in sync with the active project. A layout effect, not a passive one:
  // pages below start their queries in passive effects, which run before this component's own, so they
  // would send their first requests without the project.
  useLayoutEffect(() => {
    setProjectContext({ project: current?.project.id ?? "", environment });
    if (previousScope.current !== scopeKey) queryClient.clear();
    previousScope.current = scopeKey;
  }, [current?.project.id, environment, scopeKey, queryClient]);

  useEffect(() => {
    if (!current) return;
    localStorage.setItem(projectStorageKey, current.project.id);
    localStorage.setItem(environmentStorageKey, environment);
    // Router navigations commit as transitions, so right after `navigate()` this render can still hold the
    // previous location. `setParams` would then rewrite the URL from that stale path and query (sign-in would
    // end on "/?project=…" instead of "/overview"; a just-picked time range would be dropped). Wait for the
    // router to catch up with the browser URL; `location.pathname` and `location.search` re-run this effect.
    if (location.pathname !== window.location.pathname || location.search !== window.location.search) return;
    const next = new URLSearchParams(params);
    next.set("project", current.project.id);
    if (environment) next.set("environment", environment); else next.delete("environment");
    if (next.toString() !== params.toString()) setParams(next, { replace: true });
  }, [current, environment, environmentStorageKey, location.pathname, location.search, params, setParams]);

  const credential = session ? sessionCredential : "";
  const mustChangePassword = Boolean(session?.user.must_change_password);
  const needsProject = Boolean(session) && !current && !unknownProject;
  const connection = useQuery({
    queryKey: ["connection", credential, scopeKey],
    enabled: Boolean(credential) && !mustChangePassword && !needsProject && !unknownProject,
    retry: false,
    // The project read checks the token, project access and environment like any scoped call, but reads
    // only cached identity rows, never telemetry: listing traces here scanned spans on every page load.
    queryFn: ({ signal }) => api<ProjectDetail>(`/api/v1/projects/${encodeURIComponent(current?.project.id ?? "")}`, credential, { signal }),
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
    const next = new URLSearchParams();
    for (const key of ["range", "from", "to"]) if (params.has(key)) next.set(key, params.get(key)!);
    next.set("project", id);
    const savedEnvironment = localStorage.getItem(`tracepilot-environment:${id}`);
    if (savedEnvironment) next.set("environment", savedEnvironment);
    navigate(`/overview?${next}`);
  }, [queryClient, params, navigate]);

  const switchEnvironment = (value: string) => {
    localStorage.setItem(environmentStorageKey, value);
    const next = new URLSearchParams(params);
    next.delete("cursor");
    if (value) next.set("environment", value); else next.delete("environment");
    setParams(next);
  };

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

  const sessionState = session ? { user: session.user, memberships: session.memberships, project: current, environment, role: session.user.platform_admin ? "owner" as const : current?.role, switchProject, refresh: reloadSession, adopt, signOut } : undefined;
  if (booting) return <div className="app-shell relative flex h-screen items-center justify-center" role="status"><span className="loading-label inline-flex items-center gap-2 text-xs font-semibold text-muted [&_svg]:text-accent">Restoring your session…</span></div>;

  return <AuthContext.Provider value={{ credential, isAuthenticated, signIn, session: sessionState }}>
    <div className="app-shell relative flex flex-col h-screen max-h-screen overflow-hidden">
      <a className="fixed -top-24 left-4 z-100 rounded-lg border-2 border-accent bg-surface-solid px-3 py-2 text-text! focus:top-2" href="#content">Skip to content</a>

      {/* Floating Dynamic Island Topbar */}
      <header className="topbar relative z-30 mx-3 mt-2.5 flex h-[58px] shrink-0 items-center justify-between gap-4 rounded-[18px] border border-glass-edge bg-glass px-4 backdrop-glass shadow-glass md:mx-4 md:px-6 max-[620px]:h-auto max-[620px]:flex-wrap max-[620px]:gap-2 max-[620px]:px-3 max-[620px]:py-2">
        <div className="topbar-title flex min-w-0 items-center gap-3.5 max-[620px]:w-full">
          {showSidebar && <button className="mobile-menu flex size-8 shrink-0 items-center justify-center rounded-full text-muted hover:bg-glass-hover min-[901px]:hidden" onClick={() => setMenuOpen((open) => !open)} aria-label="Toggle navigation" aria-expanded={menuOpen}>{menuOpen ? <X size={16} /> : <Menu size={16} />}</button>}
          <NavLink className="brand group flex items-center gap-2.5 text-slate-950 dark:text-white font-semibold text-[15px] tracking-tight no-underline whitespace-nowrap" to="/overview">
            <BrandIcon />
            <span className="text-slate-950 dark:text-white font-semibold tracking-tight">TracePilot</span>
            <span className="rounded-full border border-glass-edge bg-glass-nested px-2 py-0.5 text-[10px] font-semibold tracking-wider text-muted uppercase">Console</span>
          </NavLink>
        </div>
        <div className="topbar-actions flex min-w-0 items-center gap-3 max-[620px]:w-full max-[620px]:flex-nowrap max-[620px]:gap-1.5 [&>.theme-toggle]:shrink-0">
          <ThemeToggle />
          {session ? <SessionControls status={status} dot={statusDotBg} environment={environment} onEnvironment={switchEnvironment} />
            : <NavLink to="/flow" className="inline-flex items-center gap-1.5 text-xs font-bold text-slate-700 dark:text-sky-300 hover:text-slate-950 dark:hover:text-white px-3.5 py-1.5 rounded-full backdrop-blur-2xl bg-white/70 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-xs active:scale-95 transition-all no-underline"><GitBranch size={13} /> Architecture</NavLink>}
        </div>
      </header>
      <main className={`flex-1 min-h-0 overflow-hidden w-full relative z-10 ${showSidebar ? `grid gap-3.5 p-3 md:p-4 min-[901px]:pr-0 max-[900px]:grid-cols-1 transition-[grid-template-columns] duration-200 ease-in-out ${collapsed ? "grid-cols-[56px_minmax(0,1fr)] sidebar-collapsed" : "grid-cols-[208px_minmax(0,1fr)]"}` : "flex flex-col"}`}>
        {showSidebar && <Navigation open={menuOpen} collapsed={collapsed} onToggle={toggleCollapsed} />}
        <section id="content" tabIndex={-1} className={`w-full h-full min-w-0 flex flex-col ${showSidebar ? "overflow-y-auto overflow-x-hidden min-[901px]:pr-4" : "overflow-hidden items-center justify-center"}`}>
          <div key={scopeKey} className="page-flow flex-1 flex flex-col min-h-0 w-full [&>*]:shrink-0">
            {mustChangePassword ? <ChangePasswordGate onChanged={adopt} /> : unknownProject ? <ErrorState error={new APIError(403, "Choose a project you have access to.")} /> : needsProject ? <NoProject /> : <AppRoutes isAuthenticated={isAuthenticated} />}
          </div>
        </section>
      </main>
    </div>
  </AuthContext.Provider>;
}
