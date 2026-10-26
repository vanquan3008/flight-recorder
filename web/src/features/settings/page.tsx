import { useSearchParams } from "react-router-dom";
import { PageHeader } from "../../shared/ui";
import { APIKeysSection } from "./api-keys";
import { WebhooksSection } from "./webhooks";
import "./settings.css";

const tabs = [["keys", "API keys"], ["webhooks", "Webhooks"]] as const;

export function SettingsPage() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "webhooks" ? "webhooks" : "keys";
  return <>
    <PageHeader title="Settings">Manage access keys and incident notifications. These actions need an admin-scoped key.</PageHeader>
    <div className="segmented tabs">
      {tabs.map(([value, label]) => <button type="button" className="chip" aria-pressed={tab === value} key={value} onClick={() => setParams({ tab: value })}>{label}</button>)}
    </div>
    {tab === "keys" ? <APIKeysSection /> : <WebhooksSection />}
  </>;
}
