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
import "./settings.css";

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
    <div className="segmented tabs">
      {available.map((value) => <button type="button" className="chip" aria-pressed={tab === value} key={value} onClick={() => setParams({ tab: value })}>{sections[value][0]}</button>)}
    </div>
    <Content />
  </>;
}
