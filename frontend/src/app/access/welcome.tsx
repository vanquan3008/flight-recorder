import { useMutation } from "@tanstack/react-query";
import { ArrowRight, Eye, EyeOff, Gauge, LoaderCircle, Lock, Mail, ShieldCheck, Zap } from "lucide-react";
import { type FormEvent, useState } from "react";
import { api, setAccessToken } from "../../shared/api";
import type { SessionView } from "../../shared/session";
import { FormAlert, FormField } from "../../shared/ui";
import { useAuth } from "../../shared/session";
import { WelcomeSignal } from "./welcome-signal";

export function Welcome() {
  const { signIn } = useAuth();
  const [showPassword, setShowPassword] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const request = useMutation({
    mutationFn: (body: { email: string; password: string }) =>
      api<SessionView>("/api/v1/auth/login", "", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: (view: SessionView) => {
      if (view.access_token) setAccessToken(view.access_token);
      signIn?.(view);
    },
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    request.mutate({
      email: (email || String(values.get("email") || "")).trim(),
      password: password || String(values.get("password") || ""),
    });
  };

  return (
    <section className="relative flex min-h-full flex-1 w-full items-center justify-center overflow-y-auto p-4 sm:p-6 lg:p-8">
      <div className="pointer-events-none absolute inset-0 -z-20 bg-[radial-gradient(color-mix(in_srgb,var(--text)_12%,transparent)_1px,transparent_1px)] [background-size:20px_20px] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_45%,#000_70%,transparent_100%)] opacity-40" aria-hidden="true" />
      <div className="pointer-events-none absolute -top-32 left-1/2 -z-10 h-80 w-96 -translate-x-1/2 rounded-full bg-linear-to-tr from-indigo-500/25 via-sky-400/20 to-purple-500/25 blur-3xl" aria-hidden="true" />
      <div className="pointer-events-none absolute -bottom-20 right-1/4 -z-10 h-64 w-64 rounded-full bg-linear-to-bl from-indigo-600/15 to-transparent blur-3xl" aria-hidden="true" />

      <WelcomeSignal />
      <div className="relative m-auto w-full max-w-[430px] rounded-3xl border border-glass-edge bg-glass-strong p-7 text-center backdrop-blur-2xl shadow-glass transition-all duration-300 motion-safe:animate-appear sm:p-8">
        <div className="pointer-events-none absolute inset-x-8 top-0 h-px bg-linear-to-r from-transparent via-white/30 dark:via-white/20 to-transparent" aria-hidden="true" />

        <div className="relative mx-auto mb-4 flex size-14 items-center justify-center motion-safe:animate-telemetry-drift">
          <div className="absolute inset-0 rounded-2xl bg-indigo-500/35 blur-md" aria-hidden="true" />
          <div className="relative flex size-14 items-center justify-center rounded-2xl bg-linear-to-b from-indigo-500 via-indigo-600 to-blue-700 text-white shadow-xl shadow-indigo-600/30 ring-1 ring-white/30" aria-hidden="true">
            <Gauge className="size-7 drop-shadow-sm" strokeWidth={2.2} />
          </div>
        </div>

        {/* Headings */}
        <h1 className="text-xl font-bold tracking-tight text-text sm:text-2xl">
          Welcome to TracePilot
        </h1>
        <p className="mx-auto mt-1.5 mb-5 min-h-[1.5em] text-xs leading-relaxed text-muted sm:text-[13px]">
          Sign in with the account your administrator created for you.
        </p>

        {/* Form */}
        <form className="grid gap-3.5" onSubmit={submit}>
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
            maxLength={1024}
            autoComplete="current-password"
            placeholder="••••••••••••"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
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
                <span>Signing in…</span>
              </>
            ) : (
              <>
                <span>Sign in</span>
                <ArrowRight className="size-4 transition-transform duration-150 group-hover:translate-x-0.5" aria-hidden="true" />
              </>
            )}
          </button>
        </form>

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
