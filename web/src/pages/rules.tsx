import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Plus, Power } from "lucide-react";
import { useState, type ReactNode } from "react";
import { api } from "../api";
import { useAuth } from "../auth";
import type { Page, Rule, RuleAudit } from "../types";
import { Badge, DataTable, Empty, ErrorState, Loading, PageHeader, Panel, formatTime } from "../ui";

const fields = ["source_ip", "destination_ip", "outcome", "action", "severity", "kind", "category", "observer_vendor", "observer_product", "host_name", "user_name", "rule_identifier"];
const operators = ["equals", "contains", "cidr", "exists"];
const severities = ["LOW", "MEDIUM", "HIGH", "CRITICAL"];

export function RulesPage() {
  const { apiKey } = useAuth();
  const queryClient = useQueryClient();
  const [history, setHistory] = useState<Rule | null>(null);
  const rules = useQuery({ queryKey: ["rules"], queryFn: ({ signal }) => api<Page<Rule>>("/api/v1/security/rules", apiKey, { signal }) });
  const create = useMutation({ mutationFn: (body: Record<string, unknown>) => api<Rule>("/api/v1/security/rules", apiKey, { method: "POST", body: JSON.stringify(body) }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["rules"] }) });
  const disable = useMutation({ mutationFn: (id: string) => api<void>(`/api/v1/security/rules/${id}`, apiKey, { method: "DELETE" }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["rules"] }) });
  if (rules.isPending) return <Loading />;
  if (rules.isError) return <ErrorState error={rules.error} retry={() => void rules.refetch()} />;
  return <>
    <PageHeader title="Detection rules">Small, explicit predicates keep detections reviewable and predictable.</PageHeader>
    <RuleForm pending={create.isPending} error={create.error} onSubmit={(body) => create.mutateAsync(body)} />
    <Panel title="Configured rules" meta={`${rules.data.data.length} rules`}><DataTable headers={["Name", "Predicate", "Threshold / window", "Severity", "Version", "State", ""]} empty="Create your first detection rule above." rows={rules.data.data.map((rule) => [
      rule.name, `${rule.field} ${rule.operator} ${rule.value || ""}`, `${rule.threshold} / ${rule.window_seconds}s`, <Badge value={rule.severity} />, rule.version, rule.enabled ? "Enabled" : "Disabled", <RuleActions rule={rule} pending={disable.isPending} onHistory={() => setHistory(rule)} onDisable={() => disable.mutate(rule.id)} />,
    ])} /></Panel>
    {history && <RuleHistory rule={history} onClose={() => setHistory(null)} />}
  </>;
}

function RuleForm({ pending, error, onSubmit }: { pending: boolean; error: Error | null; onSubmit: (body: Record<string, unknown>) => Promise<unknown> }) {
  const [operator, setOperator] = useState("equals");
  return <form className="rule-form" onSubmit={(event) => {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    void onSubmit({ name: form.get("name"), field: form.get("field"), operator: form.get("operator"), value: form.get("value"), severity: form.get("severity"), threshold: Number(form.get("threshold")), window_seconds: Number(form.get("window_seconds")), group_by: form.get("group_by"), enabled: true }).then(() => formElement.reset()).catch(() => undefined);
  }}>
    <Field label="Rule name"><input required name="name" /></Field>
    <Field label="Field"><select name="field">{fields.map((field) => <option key={field}>{field}</option>)}</select></Field>
    <Field label="Operator"><select name="operator" value={operator} onChange={(event) => setOperator(event.target.value)}>{operators.map((option) => <option key={option}>{option}</option>)}</select></Field>
    <Field label="Value"><input required={operator !== "exists"} name="value" /></Field>
    <Field label="Severity"><select name="severity">{severities.map((severity) => <option key={severity}>{severity}</option>)}</select></Field>
    <Field label="Threshold"><input required min="1" defaultValue="1" name="threshold" type="number" /></Field>
    <Field label="Window (seconds)"><input required min="1" defaultValue="300" name="window_seconds" type="number" /></Field>
    <Field label="Group by (optional)"><select name="group_by"><option value="">No grouping</option>{fields.map((field) => <option key={field}>{field}</option>)}</select></Field>
    <div className="rule-submit"><button disabled={pending}><Plus size={16} /> {pending ? "Creating…" : "Create rule"}</button>{error && <span className="form-error">{error.message}</span>}</div>
  </form>;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label>{label}{children}</label>;
}

function RuleActions({ rule, pending, onHistory, onDisable }: { rule: Rule; pending: boolean; onHistory: () => void; onDisable: () => void }) {
  const [confirming, setConfirming] = useState(false);
  if (confirming) return <div className="inline-confirm"><span>Disable?</span><button className="button-danger" disabled={pending} onClick={onDisable}>Yes</button><button className="button-secondary" onClick={() => setConfirming(false)}>No</button></div>;
  return <div className="table-actions"><button className="button-secondary" onClick={onHistory}><History size={14} /> History</button>{rule.enabled && <button className="button-danger" onClick={() => setConfirming(true)}><Power size={14} /> Disable</button>}</div>;
}

function RuleHistory({ rule, onClose }: { rule: Rule; onClose: () => void }) {
  const { apiKey } = useAuth();
  const history = useQuery({ queryKey: ["rule-history", rule.id], queryFn: ({ signal }) => api<Page<RuleAudit>>(`/api/v1/security/rules/${rule.id}/history?limit=100`, apiKey, { signal }) });
  return <Panel title={`Rule history: ${rule.name}`} meta={<button className="button-secondary" onClick={onClose}>Close</button>}>{history.isPending ? <Loading /> : history.isError ? <ErrorState error={history.error} retry={() => void history.refetch()} /> : history.data.data.length ? <DataTable headers={["Time", "Action", "Version", "Actor", "Predicate"]} rows={history.data.data.map((audit) => [formatTime(audit.created_at), <Badge value={audit.action} />, audit.version, audit.actor_key_id || "system", `${audit.rule.field} ${audit.rule.operator} ${audit.rule.value || ""}`])} /> : <Empty>No recorded mutations for this rule.</Empty>}</Panel>;
}
