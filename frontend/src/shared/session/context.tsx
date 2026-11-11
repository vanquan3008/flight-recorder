import { createContext, useContext } from "react";
import type { Account, Membership, Role, SessionView } from "./model";

export type AuthState = {
  /** Non-empty while someone is signed in; pass it to `api`, which attaches the access token. */
  credential: string;
  isAuthenticated: boolean;
  /** Called by the sign-in and registration forms once the server accepted them. */
  signIn?: (view: SessionView) => void;
  session?: {
    user: Account;
    memberships: Membership[];
    project?: Membership;
    environment?: string;
    /** The role in the current project; a super administrator is an owner everywhere. */
    role?: Role;
    switchProject: (projectID: string) => void;
    refresh: () => Promise<void>;
    adopt: (view: SessionView) => void;
    signOut: () => void;
  };
};

export const AuthContext = createContext<AuthState | null>(null);

export function useAuth(): AuthState {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthContext");
  return value;
}
