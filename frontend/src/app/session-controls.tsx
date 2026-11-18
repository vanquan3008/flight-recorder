import { LogOut, RefreshCw } from "lucide-react";
import { useAuth } from "../shared/session";
import { ScopeSelect } from "./scope-select";
import { initialOf, useEnvironmentOptions } from "./use-environments";

export function SessionControls({
  status,
  dot,
  environment,
  onEnvironment,
}: {
  status: string;
  dot: string;
  environment: string;
  onEnvironment: (value: string) => void;
}) {
  const { session } = useAuth();
  const { detail, options: environmentOptions, projectID } = useEnvironmentOptions(environment);

  if (!session) return null;
  const initial = initialOf(session.user.display_name);

  return (
    <div className="session-controls flex min-w-0 items-center gap-2 max-[620px]:flex-1 max-[620px]:gap-1">
      <ScopeSelect label="Project" value={projectID ?? ""} onChange={session.switchProject}
        options={session.memberships.map(({ project }) => ({ value: project.id, label: project.name }))} />
      {projectID && !session.user.must_change_password && <ScopeSelect key={projectID} label="Environment"
        value={environment} onChange={onEnvironment} options={environmentOptions} />}

      {/* Environments Loading Error Refresh Button */}
      {detail.isError && (
        <button
          className="inline-flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-full border border-amber-500/30 bg-amber-500/10 text-amber-600 transition-colors hover:bg-amber-500/20 dark:text-amber-400 cursor-pointer"
          type="button"
          onClick={() => void detail.refetch()}
          title="Retry loading environments"
          aria-label="Retry loading environments"
        >
          <RefreshCw size={13} />
          <span className="sr-only">Environments unavailable</span>
        </button>
      )}

      {/* User Capsule: Status + User Profile + Sign out */}
      <div className="user-capsule inline-flex h-[34px] shrink-0 items-center gap-2 rounded-full backdrop-blur-3xl bg-white/75 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-[inset_0_1px_1px_rgba(255,255,255,0.7)] dark:shadow-[inset_0_1px_1px_rgba(255,255,255,0.08)] px-2.5">
        {/* Status indicator */}
        <div className="flex items-center gap-1.5" role="status" title={status}>
          <i className={`size-2 shrink-0 rounded-full ${dot}`} aria-hidden="true" />
          <span className="hidden text-[11px] font-medium text-slate-600 dark:text-slate-300 sm:inline whitespace-nowrap">
            {status}
          </span>
        </div>

        <span className="h-3 w-px bg-slate-200 dark:bg-white/10 shrink-0" aria-hidden="true" />

        {/* User profile with avatar initial */}
        <div className="flex items-center gap-1.5 min-w-0" title={`Signed in as ${session.user.display_name}`}>
          <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-gradient-to-tr from-sky-600 to-indigo-500 text-[10px] font-bold text-white shadow-xs">
            {initial}
          </span>
          <span className="hidden max-w-[100px] truncate text-xs font-semibold text-slate-800 dark:text-slate-200 lg:inline">
            {session.user.display_name}
          </span>
        </div>

        <span className="h-3 w-px bg-slate-200 dark:bg-white/10 shrink-0" aria-hidden="true" />

        {/* Sign out button */}
        <button
          type="button"
          onClick={session.signOut}
          aria-label="Sign out"
          title="Sign out"
          className="group inline-flex items-center gap-1 rounded-full px-1 py-0.5 text-xs font-medium text-slate-500 hover:text-rose-600 dark:text-slate-400 dark:hover:text-rose-400 transition-colors cursor-pointer active:scale-95"
        >
          <LogOut size={13} className="shrink-0 transition-transform group-hover:translate-x-0.5" />
          <span className="max-[900px]:hidden">Sign out</span>
        </button>
      </div>
    </div>
  );
}
