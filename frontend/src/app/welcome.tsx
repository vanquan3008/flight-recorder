import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowRight, Gauge, LoaderCircle } from "lucide-react";
import { type FormEvent, useState } from "react";
import { api, setAccessToken } from "../shared/api";
import { useAuth } from "./auth";
import type { SessionView } from "../shared/session";
import "./welcome.css";

type Mode = "signin" | "register";

const post = (path: string, body: object) => api<SessionView>(path, "", { method: "POST", body: JSON.stringify(body) });

/** An account is needed to enter. One form serves both: registering only adds a name field, which unfolds in place. */
export function Welcome() {
  const { signIn } = useAuth();
  const [mode, setMode] = useState<Mode>("signin");
  // Whether new accounts are accepted. If the server cannot say, offer registration and let the server decide.
  const status = useQuery({
    queryKey: ["auth-status"], retry: false,
    queryFn: ({ signal }) => api<{ registration_open: boolean; first_account: boolean }>("/api/v1/auth/status", "", { signal }),
  });
  const registrationOpen = status.data?.registration_open !== false;
  const firstAccount = status.data?.first_account === true;
  const registering = mode === "register" && registrationOpen;

  // The access token is kept in memory the moment it arrives, before the console renders as signed in.
  const accept = (view: SessionView) => {
    if (view.access_token) setAccessToken(view.access_token);
    signIn?.(view);
  };
  const signInRequest = useMutation({
    mutationFn: (body: { email: string; password: string }) => post("/api/v1/auth/login", body),
    onSuccess: accept,
  });
  const register = useMutation({
    mutationFn: (body: { email: string; display_name: string; password: string }) => post("/api/v1/auth/register", body),
    onSuccess: accept,
  });
  const request = registering ? register : signInRequest;
  const switchMode = (next: Mode) => {
    setMode(next);
    signInRequest.reset();
    register.reset();
  };
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const email = String(values.get("email") || "").trim();
    const password = String(values.get("password") || "");
    if (registering) register.mutate({ email, display_name: String(values.get("name") || "").trim(), password });
    else signInRequest.mutate({ email, password });
  };
  const subtitle = registering
    ? firstAccount ? "You are the first here, so this account becomes the super administrator." : "Create an account, then make a project for your product."
    : "Sign in to open your projects.";

  return <section className="welcome">
    <div className="welcome-hero">
      <div className="welcome-icon-wrap" style={{ "--i": 0 } as React.CSSProperties} aria-hidden="true"><Gauge size={26} /></div>
      <h1 style={{ "--i": 1 } as React.CSSProperties}>Welcome to TracePilot</h1>
      <p key={mode} className="welcome-subtitle" style={{ "--i": 2 } as React.CSSProperties}>{subtitle}</p>

      {registrationOpen && <div className="welcome-modes" data-mode={mode} role="group" aria-label="Sign in or register" style={{ "--i": 3 } as React.CSSProperties}>
        <button type="button" aria-pressed={mode === "signin"} onClick={() => switchMode("signin")}>Sign in</button>
        <button type="button" aria-pressed={mode === "register"} onClick={() => switchMode("register")}>Register</button>
      </div>}

      <form className="welcome-form" onSubmit={submit} style={{ "--i": 4 } as React.CSSProperties}>
        <div className="welcome-fold" data-open={registering} inert={!registering}>
          <div><label>Display name<input name="name" autoComplete="name" required={registering} disabled={!registering} maxLength={100} /></label></div>
        </div>
        <label>Email<input type="email" name="email" autoComplete="username" required maxLength={254} placeholder="you@company.com" /></label>
        <label>Password<input type="password" name="password" autoComplete={registering ? "new-password" : "current-password"} required minLength={registering ? 12 : undefined} maxLength={registering ? 256 : 1024} /></label>
        <div className="welcome-fold" data-open={registering} inert={!registering}>
          <div><small>At least 12 characters. A passphrase of a few words works well.</small></div>
        </div>
        <div className="welcome-fold" data-open={Boolean(request.error)}>
          <div><p className="form-error" role={request.error ? "alert" : undefined}>{request.error?.message}</p></div>
        </div>
        <button type="submit" className="welcome-submit" disabled={request.isPending}>
          {request.isPending ? <><LoaderCircle className="animate-spin" size={15} aria-hidden="true" /> {registering ? "Creating…" : "Signing in…"}</> : <>{registering ? "Create account" : "Sign in"} <ArrowRight size={15} aria-hidden="true" /></>}
        </button>
      </form>
    </div>
  </section>;
}
