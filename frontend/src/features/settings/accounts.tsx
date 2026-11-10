import { settingsFormClass } from "./form-styles";
import { secondaryButtonClass } from "../../shared/ui/styles";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useAuth } from "../../app/auth";
import { api, pageItems, usePagedQuery } from "../../shared/api";
import type { Account } from "../../shared/session";
import { Badge, PagedTable, Panel, formatTime } from "../../shared/ui";
import { SecretNotice } from "./secret-notice";

type Issued = { user: Account; temporary_password?: string };

/** Platform administrators create accounts and hand each person a temporary password once. */
export function AccountsSection() {
  const { credential, session } = useAuth();
  const queryClient = useQueryClient();
  const [issued, setIssued] = useState<{ title: string; secret: string } | null>(null);
  const query = usePagedQuery<Account>(["settings-accounts"], "/api/v1/users?limit=50", credential);
  const accounts = pageItems(query.data?.pages);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["settings-accounts"] });
  const create = useMutation({
    mutationFn: (body: { email: string; display_name: string; platform_admin: boolean }) => api<Issued>("/api/v1/users", credential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: async ({ user, temporary_password }) => {
      if (temporary_password) setIssued({ title: `Temporary password for ${user.email}`, secret: temporary_password });
      await refresh();
    },
  });
  const update = useMutation({
    mutationFn: ({ id, ...body }: { id: string; disabled?: boolean; platform_admin?: boolean }) => api<{ user: Account }>(`/api/v1/users/${id}`, credential, { method: "PATCH", body: JSON.stringify(body) }),
    onSuccess: refresh,
  });
  const reset = useMutation({
    mutationFn: (account: Account) => api<{ temporary_password: string }>(`/api/v1/users/${account.id}/password-reset`, credential, { method: "POST" }).then((result) => ({ account, result })),
    onSuccess: ({ account, result }) => setIssued({ title: `New temporary password for ${account.email}`, secret: result.temporary_password }),
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    create.mutate({ email: String(values.get("email") || "").trim(), display_name: String(values.get("name") || "").trim(), platform_admin: values.get("platform") === "on" }, { onSuccess: () => form.reset() });
  };
  const failure = create.error || update.error || reset.error;
  return <>
    {issued && <SecretNotice {...issued} onDismiss={() => setIssued(null)} />}
    <Panel title="Accounts" meta={`${accounts.length} loaded`}>
      <form className={`settings-form ${settingsFormClass} hook-form min-[901px]:grid-cols-[minmax(180px,1fr)_minmax(220px,1.5fr)_auto]`} onSubmit={submit}>
        <h3 className="settings-form-title col-span-full m-0 text-[11px] font-bold uppercase tracking-wider text-faint">New account</h3>
        <label>Email<input type="email" name="email" required maxLength={254} placeholder="person@company.com" /></label>
        <label>Display name<input name="name" required maxLength={100} placeholder="Full name" /></label>
        <div className="settings-submit flex items-end [&>button]:inline-flex [&>button]:items-center [&>button]:justify-center [&>button]:gap-1.5 [&>button]:whitespace-nowrap [&>button]:rounded-lg [&>button]:border [&>button]:border-btn-border [&>button]:bg-btn [&>button]:px-5.5 [&>button]:py-2 [&>button]:text-[13px] [&>button]:font-semibold [&>button]:text-btn-text [&>button:hover]:bg-btn-hover max-[900px]:[&>button]:w-full"><button type="submit" disabled={create.isPending}>{create.isPending ? "Creating…" : "Create account"}</button></div>
        <label className="check relative inline-flex cursor-pointer [&>input]:absolute [&>input]:inset-0 [&>input]:size-full [&>input]:cursor-pointer [&>input]:opacity-0 [&>span]:inline-flex [&>span]:items-baseline [&>span]:gap-2 [&>span]:rounded-full [&>span]:border [&>span]:border-glass-field-edge [&>span]:bg-glass-field [&>span]:px-3.5 [&>span]:py-1.5 [&>span]:text-xs [&>span]:text-muted [&>input:checked+span]:border-accent [&>input:checked+span]:bg-accent/15 [&>input:focus-visible+span]:outline-2 [&>input:focus-visible+span]:outline-accent [&_b]:font-semibold [&_b]:text-text [&_small]:text-[11px] [&_small]:text-faint settings-wide col-span-full"><input type="checkbox" name="platform" /><span><b>Platform administrator</b> <small>Can create projects and accounts</small></span></label>
        <p className="settings-hint col-span-full m-0 text-xs text-faint">The person signs in with the temporary password shown once, then chooses their own.</p>
      </form>
      {failure && <p className="form-error mt-1.5 text-[12.5px] font-medium leading-relaxed text-danger-fg" role="alert">{failure.message}</p>}
      <PagedTable query={query} empty="No accounts." headers={["Person", "Status", "Last sign-in", "Created", "Actions"]}
        rows={accounts.map((account) => {
          const self = account.id === session?.user.id;
          return [
            <span className="person grid gap-px [&_b]:font-semibold [&_small]:text-[11.5px] [&_small]:text-faint"><span><b>{account.display_name}</b>{self && <small> (you)</small>}</span><small>{account.email}</small></span>,
            <span className="scope-list flex flex-wrap gap-1">{account.disabled_at ? <Badge value="DISABLED" /> : <Badge value="ACTIVE" />}{account.platform_admin && <Badge value="ADMIN" />}{account.must_change_password && <Badge value="TEMP PASSWORD" />}</span>,
            account.last_login_at ? formatTime(account.last_login_at) : "Never", formatTime(account.created_at),
            self ? "—" : <span className="table-actions inline-flex flex-wrap items-center gap-1.5">
              <button className={`button-secondary ${secondaryButtonClass}`} disabled={update.isPending} onClick={() => update.mutate({ id: account.id, platform_admin: !account.platform_admin })} aria-label={`${account.platform_admin ? "Demote" : "Grant admin"} ${account.email}`}>{account.platform_admin ? "Demote" : "Grant admin"}</button>
              <button className={`button-secondary ${secondaryButtonClass}`} disabled={reset.isPending} onClick={() => reset.mutate(account)} aria-label={`Reset password of ${account.email}`}>Reset password</button>
              <button className={account.disabled_at ? `button-secondary ${secondaryButtonClass}` : "button-danger inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-semibold leading-snug transition-colors hover:border-accent/45 border-danger/40 bg-danger-bg text-danger-fg hover:bg-danger/20"} disabled={update.isPending} onClick={() => update.mutate({ id: account.id, disabled: !account.disabled_at })} aria-label={`${account.disabled_at ? "Enable" : "Disable"} ${account.email}`}>{account.disabled_at ? "Enable" : "Disable"}</button>
            </span>,
          ];
        })} />
    </Panel>
  </>;
}
