export type Role = "owner" | "user";

export type Project = { id: string; slug: string; name: string; created_at: string };
export type Membership = { project: Project; role: Role };
export type Account = {
  id: string;
  email: string;
  display_name: string;
  platform_admin: boolean;
  must_change_password: boolean;
  disabled_at?: string;
  created_at: string;
  last_login_at?: string;
};

/** What the server answers when someone signs in or refreshes. The access token is kept in memory only. */
export type SessionView = {
  user: Account;
  memberships: Membership[];
  access_token?: string;
  expires_at?: string;
};

export type Member = { user: Account; role: Role; created_at: string };
export type ProjectDetail = {
  project: Project;
  role: Role;
  environments: { id: string; name: string; created_at: string }[];
};
export type AuditEvent = {
  id: string;
  actor_user_id?: string;
  action: string;
  target_type: string;
  target_id: string;
  detail: Record<string, unknown>;
  created_at: string;
};
export type IngestToken = {
  id: string;
  name: string;
  created_at: string;
  expires_at: string;
  revoked_at?: string;
};

// ── Roles ─────────────────────────────────────────────────────────────────────

export const roles: Role[] = ["owner", "user"];

/** Each role as people know it. Super administrators are an account flag, not a project role. */
export const roleLabels: Record<Role, string> = { owner: "Owner", user: "User" };
export const roleHints: Record<Role, string> = {
  owner: "Created the project: manages members, ingest tokens, webhooks and settings",
  user: "Allowed in by an owner: reads telemetry and works incidents",
};

// ── Auth helpers ───────────────────────────────────────────────────────────────

export const signedInHint = "tracepilot-signed-in";
export const projectStorageKey = "tracepilot-project";

/** Mirrors the server rule: only owners may administer a project. */
export function canManageRole(actor: Role | undefined): boolean {
  return actor === "owner";
}
