import { Activity, BellRing, Gauge, GitBranch, Network, ScrollText, Settings, TrendingUp, type LucideIcon } from "lucide-react";

export type NavItem = { to: string; label: string; short: string; Icon: LucideIcon };

const item = (to: string, label: string, short: string, Icon: LucideIcon): NavItem => ({ to, label, short, Icon });

export const overview = item("/overview", "Overview", "Overview", Gauge);
export const performance = item("/performance", "Performance", "Performance", TrendingUp);
export const traces = item("/traces", "Trace explorer", "Traces", Activity);
export const logs = item("/logs", "Log explorer", "Logs", ScrollText);
export const topology = item("/topology", "Service topology", "Topology", Network);
export const incidents = item("/incidents", "Incidents", "Incidents", BellRing);
export const settings = item("/settings", "Settings", "Settings", Settings);
export const flow = item("/flow", "How it works", "How it works", GitBranch);

/** Sidebar sections on wider screens. */
export const navGroups: [string, NavItem[]][] = [
  ["Observe", [overview, performance, traces, logs, topology]],
  ["Respond", [incidents]],
  ["Manage", [settings, flow]],
];

/** The phone bottom bar keeps the investigation path one tap away; the rest lives under More. */
export const primaryTabs = [overview, traces, logs, incidents];
export const moreItems = [performance, topology, settings, flow];

export const isActivePath = (pathname: string, to: string) => pathname === to || pathname.startsWith(`${to}/`);
