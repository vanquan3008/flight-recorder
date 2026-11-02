import { useMutation } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { type FormEvent, useState } from "react";
import { CreateProjectForm } from "../features/settings/projects";
import { api, sessionCredential } from "../shared/api";
import type { SessionView } from "../shared/session";
import { Panel } from "../shared/ui";
import "./access.css";

/** An account made by an administrator starts with a temporary password; nothing else works until it is replaced. */
export function ChangePasswordGate({ onChanged }: { onChanged: (view: SessionView) => void }) {
  const [mismatch, setMismatch] = useState(false);
  const change = useMutation({
    mutationFn: (body: { current_password: string; new_password: string }) => api<SessionView>("/api/v1/auth/password", sessionCredential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: onChanged,
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const next = String(values.get("new") || "");
    setMismatch(next !== String(values.get("confirm") || ""));
    if (next === String(values.get("confirm") || "")) change.mutate({ current_password: String(values.get("current") || ""), new_password: next });
  };
  return <div className="access-gate">
    <Panel title="Choose your own password">
      <form className="access-form" onSubmit={submit}>
        <p className="access-lead"><KeyRound size={16} aria-hidden="true" /> You signed in with a temporary password. Choose a new one to continue.</p>
        <label>Temporary password<input type="password" name="current" autoComplete="current-password" required /></label>
        <label>New password<input type="password" name="new" autoComplete="new-password" required minLength={12} maxLength={256} /><small>At least 12 characters. A passphrase of a few words works well.</small></label>
        <label>Repeat new password<input type="password" name="confirm" autoComplete="new-password" required minLength={12} maxLength={256} /></label>
        {mismatch && <p className="form-error" role="alert">The two passwords do not match.</p>}
        {change.error && <p className="form-error" role="alert">{change.error.message}</p>}
        <button type="submit" disabled={change.isPending}>{change.isPending ? "Saving…" : "Save password"}</button>
      </form>
    </Panel>
  </div>;
}

/** A signed-in person with no project yet: they can create one for their product, or wait for an owner to add them to one. */
export function NoProject() {
  return <div className="access-gate">
    <Panel title="Create your first project">
      <p className="access-lead">A project receives the telemetry of one product. Create one to start collecting, or ask the owner of an existing project to add you.</p>
      <CreateProjectForm />
    </Panel>
  </div>;
}
