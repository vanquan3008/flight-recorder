import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowRight, Eye, EyeOff, Gauge, LoaderCircle, Lock, Mail, ShieldCheck, Sparkles, User, Zap } from "lucide-react";
import { type FormEvent, useState } from "react";
import { api, setAccessToken } from "../shared/api";
import type { SessionView } from "../shared/session";
import { FormAlert, FormField } from "../shared/ui";
import { useAuth } from "./auth";
import { WelcomeSignal } from "./welcome-signal";

type Mode = "signin" | "register";

const post = (path: string, body: object) =>
  api<SessionView>(path, "", { method: "POST", body: JSON.stringify(body) });

/**
 * Modern SaaS Bento/Glassmorphism Welcome & Authentication screen.
 * Beautifully crafted with Tailwind CSS utility classes, glowing telemetry accents,
 * accessible input controls, and instant dev autofill.
 */
export function Welcome() {
  const { signIn } = useAuth();
  const [mode, setMode] = useState<Mode>("signin");
  const [showPassword, setShowPassword] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");

  // Check whether registration is open on the server.
  const status = useQuery({
    queryKey: ["auth-status"],
    retry: false,
    queryFn: ({ signal }) => api<{ registration_open: boolean }>("/api/v1/auth/status", "", { signal }),
  });
  const registrationOpen = status.data?.registration_open !== false;
  const registering = mode === "register" && registrationOpen;

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
    const formEmail = (email || String(values.get("email") || "")).trim();
    const formPassword = password || String(values.get("password") || "");
    const formName = (displayName || String(values.get("name") || "")).trim();

    if (registering) {
      register.mutate({ email: formEmail, display_name: formName, password: formPassword });
    } else {
      signInRequest.mutate({ email: formEmail, password: formPassword });
    }
  };

  const fillDevCredentials = () => {
    setEmail("admin@tracepilot.local");
    setPassword("admin-password-123");
  };

  const subtitle = registering
    ? "Create an account. An administrator can grant you project access."
    : "Sign in to access your telemetry, traces, and system diagnostics.";

  return (
    <section className="relative flex min-h-full flex-1 w-full items-center justify-center overflow-y-auto p-4 sm:p-6 lg:p-8">
      {/* Subtle Dot-Matrix Ambient Grid Background */}
      <div
        className="pointer-events-none absolute inset-0 -z-20 bg-[radial-gradient(color-mix(in_srgb,var(--text)_12%,transparent)_1px,transparent_1px)] [background-size:20px_20px] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_45%,#000_70%,transparent_100%)] opacity-40"
        aria-hidden="true"
      />

      {/* Atmospheric Luminous Mesh Glows */}
      <div
        className="pointer-events-none absolute -top-32 left-1/2 -z-10 h-80 w-96 -translate-x-1/2 rounded-full bg-linear-to-tr from-indigo-500/25 via-sky-400/20 to-purple-500/25 blur-3xl"
        aria-hidden="true"
      />
      <div
        className="pointer-events-none absolute -bottom-20 right-1/4 -z-10 h-64 w-64 rounded-full bg-linear-to-bl from-indigo-600/15 to-transparent blur-3xl"
        aria-hidden="true"
      />

      <WelcomeSignal />
      {/* Modern Bento Glassmorphic Container */}
      <div className="relative m-auto w-full max-w-[430px] rounded-3xl border border-glass-edge bg-glass-strong p-7 text-center backdrop-blur-2xl shadow-glass transition-all duration-300 motion-safe:animate-appear sm:p-8">
        {/* Subtle Top Glass Reflection Highlight */}
        <div
          className="pointer-events-none absolute inset-x-8 top-0 h-px bg-linear-to-r from-transparent via-white/30 dark:via-white/20 to-transparent"
          aria-hidden="true"
        />

        {/* Brand Icon with multi-layer glowing depth */}
        <div className="relative mx-auto mb-4 flex size-14 items-center justify-center motion-safe:animate-telemetry-drift">
          <div
            className="absolute inset-0 rounded-2xl bg-indigo-500/35 blur-md"
            aria-hidden="true"
          />
          <div
            className="relative flex size-14 items-center justify-center rounded-2xl bg-linear-to-b from-indigo-500 via-indigo-600 to-blue-700 text-white shadow-xl shadow-indigo-600/30 ring-1 ring-white/30"
            aria-hidden="true"
          >
            <Gauge className="size-7 drop-shadow-sm" strokeWidth={2.2} />
          </div>
        </div>

        {/* Headings */}
        <h1 className="text-xl font-bold tracking-tight text-text sm:text-2xl">
          Welcome to TracePilot
        </h1>
        <p className="mx-auto mt-1.5 mb-5 min-h-[1.5em] text-xs leading-relaxed text-muted sm:text-[13px]">
          {subtitle}
        </p>

        {/* Mode Switcher Tabs */}
        {registrationOpen && (
          <div
            className="relative mb-5 grid grid-cols-2 rounded-xl border border-glass-field-edge bg-glass-field p-1"
            role="group"
            aria-label="Sign in or register"
          >
            <button
              type="button"
              aria-pressed={mode === "signin"}
              onClick={() => switchMode("signin")}
              className={`rounded-lg py-2 text-xs font-semibold transition-all duration-200 ${
                mode === "signin"
                  ? "bg-btn text-white shadow-md"
                  : "text-muted hover:text-text"
              }`}
            >
              Sign in
            </button>
            <button
              type="button"
              aria-pressed={mode === "register"}
              onClick={() => switchMode("register")}
              className={`rounded-lg py-2 text-xs font-semibold transition-all duration-200 ${
                mode === "register"
                  ? "bg-btn text-white shadow-md"
                  : "text-muted hover:text-text"
              }`}
            >
              Register
            </button>
          </div>
        )}

        {/* Form */}
        <form className="grid gap-3.5" onSubmit={submit}>
          {registering && (
            <FormField
              id="name"
              name="name"
              label="Display name"
              icon={User}
              required={registering}
              maxLength={100}
              placeholder="Alice Developer"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className="motion-safe:animate-appear"
            />
          )}

          <FormField
            id="email"
            name="email"
            type="email"
            label="Email"
            icon={Mail}
            required
            autoComplete="username"
            maxLength={254}
            placeholder="you@company.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />

          <FormField
            id="password"
            name="password"
            type={showPassword ? "text" : "password"}
            label="Password"
            icon={Lock}
            required
            minLength={registering ? 12 : undefined}
            maxLength={registering ? 256 : 1024}
            autoComplete={registering ? "new-password" : "current-password"}
            placeholder="••••••••••••"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            hint={
              registering
                ? "At least 12 characters. A passphrase of a few words works well."
                : undefined
            }
            rightElement={
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                aria-pressed={showPassword}
                className="text-muted transition-colors hover:text-text focus:outline-none"
                aria-label={showPassword ? "Hide password" : "Show password"}
              >
                {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            }
          />

          <FormAlert message={request.error?.message} />

          {/* Submit CTA Button */}
          <button
            type="submit"
            disabled={request.isPending}
            className="group relative mt-2 flex w-full items-center justify-center gap-2 overflow-hidden rounded-xl border border-btn-border bg-btn py-2.75 px-4 text-xs sm:text-sm font-semibold text-white shadow-lg shadow-indigo-500/20 transition-all duration-150 hover:bg-btn-hover hover:shadow-indigo-500/35 hover:-translate-y-0.5 active:translate-y-0 active:scale-[0.99] disabled:cursor-progress disabled:opacity-80"
          >
            <span aria-hidden="true" className="pointer-events-none absolute inset-0 -translate-x-[120%] -skew-x-12 bg-linear-to-r from-transparent via-white/20 to-transparent transition-transform duration-700 group-hover:translate-x-[120%]" />
            {request.isPending ? (
              <>
                <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                <span>{registering ? "Creating account…" : "Signing in…"}</span>
              </>
            ) : (
              <>
                <span>{registering ? "Create account" : "Sign in"}</span>
                <ArrowRight className="size-4 transition-transform duration-150 group-hover:translate-x-0.5" aria-hidden="true" />
              </>
            )}
          </button>
        </form>

        {/* Quick Dev Login Helper */}
        {!registering && (
          <div className="mt-5 border-t border-line-soft pt-4">
            <div className="flex flex-col items-center gap-2">
              <div className="flex items-center gap-1.5 text-[11px] font-medium text-faint">
                <Sparkles className="size-3 text-amber-400" aria-hidden="true" />
                <span>Quick fill for local development</span>
              </div>
              <button
                type="button"
                onClick={fillDevCredentials}
                className="group inline-flex items-center gap-2 rounded-xl border border-glass-field-edge bg-glass-field px-3 py-1.5 text-xs text-muted transition-all duration-150 hover:border-accent/40 hover:bg-glass-field-focus hover:text-text active:scale-[0.98]"
                title="Autofill default admin credentials"
              >
                <span className="font-mono text-[11.5px] text-accent">admin@tracepilot.local</span>
                <span className="rounded-md bg-accent/15 px-1.5 py-0.5 text-[10px] font-semibold tracking-wider text-accent uppercase transition-colors group-hover:bg-accent/25">
                  Autofill
                </span>
              </button>
            </div>
          </div>
        )}

        {/* Trust & Architecture Micro-Badges */}
        <div className="mt-5 flex items-center justify-center gap-4 border-t border-line-soft/60 pt-4 text-[11px] text-faint">
          <div className="flex items-center gap-1.5">
            <ShieldCheck className="size-3.5 text-emerald-400/80" aria-hidden="true" />
            <span>Role-Based RBAC</span>
          </div>
          <span className="text-line" aria-hidden="true">•</span>
          <div className="flex items-center gap-1.5">
            <Zap className="size-3.5 text-sky-400/80" aria-hidden="true" />
            <span>ClickHouse Engine</span>
          </div>
        </div>
      </div>
    </section>
  );
}
