import { LogOut, MoreHorizontal } from "lucide-react";
import { useEffect, useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { useAuth } from "../../shared/session";
import { BottomSheet } from "../../shared/ui";
import { contextURL } from "../../shared/ui/links";
import { isActivePath, moreItems, primaryTabs } from "./nav-items";
import { ThemeToggle } from "./theme";
import { initialOf } from "../scope/use-environments";

const tab = "flex h-14 flex-col items-center justify-center gap-0.5 rounded-xl text-[10.5px] font-semibold no-underline transition-colors";
const tone = (active: boolean) => active ? "text-accent [&>svg]:scale-110" : "text-muted hover:text-text";

/** Phone navigation: the investigation path in a bottom bar, everything else (and the account) under More. */
export function MobileTabBar({ status, dot }: { status: string; dot: string }) {
  const { pathname, search } = useLocation();
  const { session } = useAuth();
  const [more, setMore] = useState(false);
  useEffect(() => setMore(false), [pathname]);
  const moreActive = moreItems.some((item) => isActivePath(pathname, item.to));

  return <>
    <nav aria-label="Console sections" className="mobile-tabbar fixed inset-x-0 bottom-0 z-40 border-t border-glass-edge bg-glass-strong px-2 pb-[env(safe-area-inset-bottom)] backdrop-glass shadow-[0_-6px_24px_rgba(15,23,42,0.08)]">
      <ul className="m-0 grid list-none grid-cols-5 gap-1 p-0">
        {primaryTabs.map(({ to, short, Icon }) => <li key={to}>
          <NavLink to={contextURL(to, search, true)} end={to === "/overview"} className={({ isActive }) => `${tab} ${tone(isActive)}`}>
            <Icon size={20} strokeWidth={1.9} aria-hidden="true" className="transition-transform" />
            <span>{short}</span>
          </NavLink>
        </li>)}
        <li>
          <button type="button" className={`${tab} w-full ${tone(moreActive || more)}`} aria-haspopup="dialog" aria-expanded={more}
            aria-current={moreActive ? "page" : undefined} onClick={() => setMore(true)}>
            <MoreHorizontal size={20} strokeWidth={1.9} aria-hidden="true" />
            <span>More</span>
          </button>
        </li>
      </ul>
    </nav>
    <BottomSheet open={more} title="More" onClose={() => setMore(false)}>
      <nav aria-label="More sections">
        <ul className="m-0 grid list-none grid-cols-2 gap-2 p-0">
          {moreItems.map(({ to, label, Icon }) => <li key={to}>
            <NavLink to={contextURL(to, search, true)} className={({ isActive }) => `flex min-h-14 items-center gap-3 rounded-2xl border px-3.5 py-3 text-[13px] font-semibold no-underline transition-colors ${isActive ? "border-accent/40 bg-accent/10 text-text" : "border-line-soft bg-surface-raised text-muted hover:text-text"}`}>
              <Icon size={18} aria-hidden="true" className="shrink-0 text-accent" />
              <span className="min-w-0">{label}</span>
            </NavLink>
          </li>)}
        </ul>
      </nav>
      <div className="mt-4 flex items-center justify-between gap-3 rounded-2xl border border-line-soft bg-surface-raised px-3.5 py-2.5">
        <span className="text-[13px] font-semibold text-text">Theme</span>
        <ThemeToggle />
      </div>
      {session && <section aria-label="Account" className="mt-3 flex items-center gap-3 rounded-2xl border border-line-soft bg-surface-raised px-3.5 py-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-tr from-sky-600 to-indigo-500 text-sm font-bold text-white" aria-hidden="true">{initialOf(session.user.display_name)}</span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-semibold text-text">{session.user.display_name}</span>
          <span className="flex items-center gap-1.5 text-[11.5px] text-muted" role="status"><i className={`size-2 shrink-0 rounded-full ${dot}`} aria-hidden="true" />{status}</span>
        </span>
        <button type="button" onClick={session.signOut} className="inline-flex h-10 items-center gap-1.5 rounded-xl border border-line px-3 text-[12.5px] font-semibold text-muted hover:border-rose-500/40 hover:text-rose-600 dark:hover:text-rose-400">
          <LogOut size={15} aria-hidden="true" />Sign out
        </button>
      </section>}
    </BottomSheet>
  </>;
}
