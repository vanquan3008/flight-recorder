import { ShieldAlert } from "lucide-react";
import { CopyButton } from "../../shared/ui";

/** The server returns a secret once; this keeps it visible until the operator dismisses it. */
export function SecretNotice({ title, secret, onDismiss }: { title: string; secret: string; onDismiss: () => void }) {
  return <div className="secret-notice" role="status">
    <ShieldAlert aria-hidden="true" />
    <div><strong>{title}</strong><p>This value is shown only once. Store it now; it cannot be retrieved later.</p><code>{secret}</code></div>
    <div className="secret-actions"><CopyButton value={secret} label="Copy secret" /><button type="button" className="button-secondary compact" onClick={onDismiss}>I have stored it</button></div>
  </div>;
}
