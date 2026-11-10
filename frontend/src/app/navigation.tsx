import { Activity, BellRing, ClipboardList, Gauge, GitBranch, Network, PanelLeftClose, PanelLeftOpen, ScrollText, Settings, TrendingUp } from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import { contextURL } from "../shared/ui/navigation";

export function Navigation({ open, collapsed, onToggle }: { open: boolean; collapsed: boolean; onToggle: () => void }) {
  const location = useLocation();
  const groups = [
    ["Observe", [["/overview", "Overview", Gauge], ["/performance", "Performance", TrendingUp], ["/traces", "Trace explorer", Activity], ["/logs", "Log explorer", ScrollText], ["/topology", "Service topology", Network]]],
    ["Respond", [["/incidents", "Incidents", BellRing]]],
    ["Manage", [["/settings", "Settings", Settings], ["/flow", "How it works", GitBranch]]],
  ] as const;

  return <nav aria-label="Console sections" className={`flex h-full max-h-full shrink-0 flex-col gap-2 overflow-hidden rounded-[18px] border border-glass-edge bg-glass p-2.5 backdrop-glass shadow-glass transition-[width] max-[900px]:fixed max-[900px]:inset-x-3.5 max-[900px]:top-[74px] max-[900px]:z-40 max-[900px]:h-auto max-[900px]:max-h-[calc(100dvh-90px)] max-[900px]:w-auto max-[900px]:overflow-auto max-[900px]:bg-glass-strong max-[620px]:top-[116px] max-[620px]:max-h-[calc(100dvh-128px)] ${open ? "open max-[900px]:flex" : "max-[900px]:hidden"} ${collapsed ? "w-14 collapsed" : "w-52"}`}>
    <div className={`flex items-center min-h-[32px] pb-2 border-b border-black/5 dark:border-white/10 ${collapsed ? "justify-center" : "justify-between px-1"}`}>
      {!collapsed && <span className="text-[10.5px] font-semibold tracking-widest text-faint uppercase select-none">Console</span>}
      <button
        type="button"
        className="w-7 h-7 rounded-xl flex items-center justify-center text-slate-700 dark:text-sky-300 hover:text-slate-950 dark:hover:text-white hover:bg-white/60 dark:hover:bg-white/10 active:scale-90 transition-all cursor-pointer p-0 shadow-none border-0"
        onClick={onToggle}
        title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      >
        {collapsed ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
      </button>
    </div>
    {groups.map(([label, links]) => <div className="grid gap-1" key={label}>
      {!collapsed && <small className="px-3 py-1 text-[10px] font-semibold tracking-wider text-faint uppercase">{label}</small>}
      {links.map(([to, name, Icon]) => <NavLink
        to={contextURL(to, location.search, true)}
        key={to}
        title={name}
        end={to === "/overview"}
        className={({ isActive }) => `flex items-center gap-2.5 rounded-lg text-[13px] font-semibold no-underline transition-colors ${
          collapsed ? "w-9 h-9 justify-center p-0 mx-auto" : "px-3 py-2"
        } ${
          isActive
            ? "bg-accent-strong/26 text-text [&>svg]:text-accent"
            : "text-muted hover:bg-glass-hover hover:text-text"
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
