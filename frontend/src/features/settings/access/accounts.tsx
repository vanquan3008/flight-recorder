import { CreateForm, FormError } from "../form-parts";
import { secondaryButtonClass } from "../../../shared/ui/styles";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useAuth } from "../../../shared/session";
import { api, pageItems, usePagedQuery } from "../../../shared/api";
import type { Account } from "../../../shared/session";
import { Badge, PagedTable, Panel, formatTime } from "../../../shared/ui";
import { SecretNotice } from "../secret-notice";

type Issued = { user: Account; temporary_password?: string };

/** The super administrator creates every account and hands each person a temporary password once. */
export function AccountsSection() {
  const { credential, session } = useAuth();
  const queryClient = useQueryClient();
  const [issued, setIssued] = useState<{ title: string; secret: string } | null>(null);
  const query = usePagedQuery<Account>(["settings-accounts"], "/api/v1/users?limit=50", credential);
  const accounts = pageItems(query.data?.pages);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["settings-accounts"] });
  const create = useMutation({
    mutationFn: (body: { email: string; display_name: string }) => api<Issued>("/api/v1/users", credential, { method: "POST", body: JSON.stringify(body) }),
    onSuccess: async ({ user, temporary_password }) => {
      if (temporary_password) setIssued({ title: `Temporary password for ${user.email}`, secret: temporary_password });
      await refresh();
    },
  });
  const update = useMutation({
    mutationFn: ({ id, ...body }: { id: string; disabled: boolean }) => api<{ user: Account }>(`/api/v1/users/${id}`, credential, { method: "PATCH", body: JSON.stringify(body) }),
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
    create.mutate({ email: String(values.get("email") || "").trim(), display_name: String(values.get("name") || "").trim() }, { onSuccess: () => form.reset() });
  };
  const failure = create.error || update.error || reset.error;
  return <>
    {issued && <SecretNotice {...issued} onDismiss={() => setIssued(null)} />}
    <Panel title="Accounts" meta={`${accounts.length} loaded`}>
      <CreateForm title="New account" hint="The person signs in with the temporary password shown once, then chooses their own." onSubmit={submit}
        submit={{ pending: create.isPending, label: "Create account", pendingLabel: "Creating…" }}>
        <label>Email<input type="email" name="email" required maxLength={254} placeholder="person@company.com" /></label>
        <label>Display name<input name="name" required maxLength={100} placeholder="Full name" /></label>
      </CreateForm>
      <FormError error={failure} />
      <PagedTable query={query} empty="No accounts." headers={["Person", "Status", "Last sign-in", "Created", "Actions"]}
        rows={accounts.map((account) => {
          const self = account.id === session?.user.id;
          return [
            <span className="person grid gap-px [&_b]:font-semibold [&_small]:text-[11.5px] [&_small]:text-faint"><span><b>{account.display_name}</b>{self && <small> (you)</small>}</span><small>{account.email}</small></span>,
            <span className="scope-list flex flex-wrap gap-1">{account.disabled_at ? <Badge value="DISABLED" /> : <Badge value="ACTIVE" />}{account.platform_admin && <Badge value="ADMIN" />}{account.must_change_password && <Badge value="TEMP PASSWORD" />}</span>,
            account.last_login_at ? formatTime(account.last_login_at) : "Never", formatTime(account.created_at),
            self ? "—" : <span className="table-actions inline-flex flex-wrap items-center gap-1.5">
              <button className={`button-secondary ${secondaryButtonClass}`} disabled={reset.isPending} onClick={() => reset.mutate(account)} aria-label={`Reset password of ${account.email}`}>Reset password</button>
              <button className={account.disabled_at ? `button-secondary ${secondaryButtonClass}` : "button-danger inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg border px-3 py-1.5 text-xs font-semibold leading-snug transition-colors hover:border-accent/45 border-danger/40 bg-danger-bg text-danger-fg hover:bg-danger/20"} disabled={update.isPending} onClick={() => update.mutate({ id: account.id, disabled: !account.disabled_at })} aria-label={`${account.disabled_at ? "Enable" : "Disable"} ${account.email}`}>{account.disabled_at ? "Enable" : "Disable"}</button>
            </span>,
          ];
        })} />
    </Panel>
  </>;
}
