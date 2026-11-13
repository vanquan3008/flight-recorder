import { secondaryButtonClass } from "../../shared/ui/styles";
import { ShieldAlert } from "lucide-react";
import { CopyButton } from "../../shared/ui";

/** The server returns a secret once; this keeps it visible until the operator dismisses it. */
export function SecretNotice({ title, secret, onDismiss }: { title: string; secret: string; onDismiss: () => void }) {
  return <div className="secret-notice mb-3.5 grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3.5 rounded-2xl border border-warning/55 bg-warning/10 px-5 py-4 text-warn-fg max-[620px]:grid-cols-1 [&>svg]:mt-0.5 [&_strong]:text-[13.5px] [&_strong]:text-text [&_p]:mt-0.5 [&_p]:mb-2.5 [&_p]:text-[12.5px] [&_p]:text-muted [&_code]:block [&_code]:select-all [&_code]:rounded-lg [&_code]:border [&_code]:border-glass-divider [&_code]:bg-glass-sunken [&_code]:px-3.5 [&_code]:py-2.5 [&_code]:text-[12.5px] [&_code]:text-text [&_code]:wrap-anywhere" role="status">
    <ShieldAlert aria-hidden="true" />
    <div><strong>{title}</strong><p>This value is shown only once. Store it now; it cannot be retrieved later.</p><code>{secret}</code></div>
    <div className="secret-actions grid justify-items-end gap-2 max-[620px]:justify-items-start"><CopyButton value={secret} label="Copy secret" /><button type="button" className={`button-secondary ${secondaryButtonClass} compact px-2.5 py-1 text-[11.5px]`} onClick={onDismiss}>I have stored it</button></div>
  </div>;
}
