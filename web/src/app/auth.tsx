import { createContext, useContext } from "react";

export const apiKeyStorageKey = "tracepilot-api-key";

export type AuthState = {
  apiKey: string;
  isAuthenticated: boolean;
  connect: (key: string) => void;
  disconnect: () => void;
};

export const AuthContext = createContext<AuthState | null>(null);

export function useAuth(): AuthState {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthContext");
  return value;
}
