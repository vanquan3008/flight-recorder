import { useMutation } from "@tanstack/react-query";
import { Eye, EyeOff, KeyRound } from "lucide-react";
import { type FormEvent, useState } from "react";
import { CreateProjectForm } from "../../features/settings/projects/projects";
import { api, sessionCredential } from "../../shared/api";
import type { SessionView } from "../../shared/session";
import { FormAlert, FormField, Panel } from "../../shared/ui";

/** An account made by an administrator starts with a temporary password; nothing else works until it is replaced. */
export function ChangePasswordGate({ onChanged }: { onChanged: (view: SessionView) => void }) {
  const [mismatch, setMismatch] = useState(false);
  const [showCurrent, setShowCurrent] = useState(false);
  const [showNew, setShowNew] = useState(false);
  const change = useMutation({
    mutationFn: (body: { current_password: string; new_password: string }) =>
      api<SessionView>("/api/v1/auth/password", sessionCredential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: onChanged,
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const next = String(values.get("new") || "");
    const match = next === String(values.get("confirm") || "");
    setMismatch(!match);
    if (match) {
      change.mutate({ current_password: String(values.get("current") || ""), new_password: next });
    }
  };

  return (
    <div className="mx-auto my-6 w-full max-w-[480px]">
      <Panel title="Choose your own password">
        <form className="grid gap-3.5 p-5" onSubmit={submit}>
          <p className="m-0 flex items-start gap-2.5 text-[13px] leading-relaxed text-muted">
            <KeyRound size={16} className="mt-0.5 shrink-0 text-accent" aria-hidden="true" />
            You signed in with a temporary password. Choose a new one to continue.
          </p>

          <FormField
            id="current"
            name="current"
            type={showCurrent ? "text" : "password"}
            label="Temporary password"
            autoComplete="current-password"
            required
            rightElement={
              <button
                type="button"
                onClick={() => setShowCurrent(!showCurrent)}
                tabIndex={-1}
                className="text-muted hover:text-text focus:outline-none"
                aria-label={showCurrent ? "Hide password" : "Show password"}
              >
                {showCurrent ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            }
          />

          <FormField
            id="new"
            name="new"
            type={showNew ? "text" : "password"}
            label="New password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={256}
            hint="At least 12 characters. A passphrase of a few words works well."
            rightElement={
              <button
                type="button"
                onClick={() => setShowNew(!showNew)}
                tabIndex={-1}
                className="text-muted hover:text-text focus:outline-none"
                aria-label={showNew ? "Hide password" : "Show password"}
              >
                {showNew ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            }
          />

          <FormField
            id="confirm"
            name="confirm"
            type={showNew ? "text" : "password"}
            label="Repeat new password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={256}
          />

          {mismatch && <FormAlert message="The two passwords do not match." />}
          {change.error && <FormAlert message={change.error.message} />}

          <button
            type="submit"
            disabled={change.isPending}
            className="mt-1 inline-flex items-center justify-center gap-2 rounded-xl border border-btn-border bg-btn px-5 py-2.5 text-xs font-semibold text-white shadow-md transition-all hover:bg-btn-hover disabled:cursor-progress disabled:opacity-80 justify-self-start"
          >
            {change.isPending ? "Saving…" : "Save password"}
          </button>
        </form>
      </Panel>
    </div>
  );
}

/** A signed-in person with no project yet: they can create one for their product, or wait for an owner to add them to one. */
export function NoProject() {
  return <div className="access-gate mx-auto my-6 w-full max-w-[480px]">
    <Panel title="Create your first project">
      <p className="access-lead m-0 flex items-start gap-2 px-4 pt-4 pb-1 text-[13px] leading-relaxed text-muted">A project receives the telemetry of one product. Create one to start collecting, or ask the owner of an existing project to add you.</p>
      <CreateProjectForm />
    </Panel>
  </div>;
}
