import { useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { canManageRole } from "../../shared/session";
import { PageHeader } from "../../shared/ui";
import { AccountsSection } from "./accounts";
import { AuditSection } from "./audit";
import { IngestTokensSection } from "./ingest-tokens";
import { MembersSection } from "./members";
import { ProjectsSection } from "./projects";
import { WebhooksSection } from "./webhooks";

const sections = {
  projects: ["Projects", ProjectsSection],
  tokens: ["Ingest tokens", IngestTokensSection],
  webhooks: ["Webhooks", WebhooksSection],
  members: ["Members", MembersSection],
  audit: ["Audit", AuditSection],
  accounts: ["Accounts", AccountsSection],
} as const;

type Section = keyof typeof sections;

/** What a person can use: everyone sees projects; owners also run their project; super administrators manage accounts. */
function availableSections(session: ReturnType<typeof useAuth>["session"]): Section[] {
  if (!session) return ["projects"];
  return [
    "projects",
    ...(canManageRole(session.role) ? ["tokens", "webhooks", "members", "audit"] as const : []),
    ...(session.user.platform_admin ? ["accounts"] as const : []),
  ];
}

export function SettingsPage() {
  const { session } = useAuth();
  const [params, setParams] = useSearchParams();
  const available = availableSections(session);
  const requested = params.get("tab");
  const tab = available.find((value) => value === requested) ?? available[0];
  const Content = sections[tab][1];
  return <>
    <PageHeader title="Settings">Projects, people and access. What you can change depends on your role in the project.</PageHeader>
    <div className="segmented inline-flex min-w-0 gap-0.5 rounded-lg border border-glass-field-edge bg-glass-field p-0.75 tabs mb-4 flex max-w-full flex-wrap self-start gap-1 rounded-xl border-glass-divider bg-surface/35 p-1">
      {available.map((value) => <button type="button" className="chip rounded-lg px-4.5 py-1.5 text-[12.5px] font-semibold text-muted transition-colors hover:bg-glass-hover hover:text-text aria-pressed:bg-btn aria-pressed:text-btn-text" aria-pressed={tab === value} key={value} onClick={() => setParams({ tab: value })}>{sections[value][0]}</button>)}
    </div>
    <Content />
  </>;
}
