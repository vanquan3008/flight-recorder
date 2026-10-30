export type APIKey = { id: string; name: string; scopes: string[]; expires_at?: string; created_at: string; revoked_at?: string };
export type IssuedKey = { api_key: APIKey; key: string };
export type Webhook = { id: string; name: string; url: string; events: string[]; enabled: boolean; created_at: string; updated_at: string };
export type WebhookDelivery = {
  id: string; webhook_id: string; event_type: string; status: string; attempt: number; max_attempts: number;
  http_status?: number; last_error?: string; next_attempt_at: string; created_at: string;
};
export const keyScopes = [
  ["ingest", "Send telemetry"], ["read", "Read telemetry and incidents"], ["write", "Change incident status"],
  ["replay", "Replay incident requests"], ["admin", "Manage keys and webhooks"],
] as const;
