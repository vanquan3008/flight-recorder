import { Monitor, Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";

export const themeStorageKey = "tracepilot-theme";
export type ThemeChoice = "system" | "light" | "dark";

const choices: [ThemeChoice, string, typeof Sun][] = [["system", "System theme", Monitor], ["light", "Light theme", Sun], ["dark", "Dark theme", Moon]];

function readChoice(): ThemeChoice {
  try {
    const stored = localStorage.getItem(themeStorageKey);
    return stored === "light" || stored === "dark" ? stored : "system";
  } catch {
    return "system";
  }
}

const prefersLight = () => typeof window.matchMedia === "function" && window.matchMedia("(prefers-color-scheme: light)").matches;

export function resolveTheme(choice: ThemeChoice): "light" | "dark" {
  return choice === "system" ? (prefersLight() ? "light" : "dark") : choice;
}

export function useTheme() {
  const [choice, setChoice] = useState<ThemeChoice>(readChoice);
  useEffect(() => {
    const apply = () => {
      const theme = resolveTheme(choice);
      document.documentElement.setAttribute("data-theme", theme);
      document.querySelector('meta[name="theme-color"]')?.setAttribute("content", theme === "light" ? "#f4f6f9" : "#0d1420");
    };
    apply();
    if (choice !== "system" || typeof window.matchMedia !== "function") return;
    const media = window.matchMedia("(prefers-color-scheme: light)");
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [choice]);
  const select = (next: ThemeChoice) => {
    try {
      if (next === "system") localStorage.removeItem(themeStorageKey);
      else localStorage.setItem(themeStorageKey, next);
    } catch {
      // The choice still applies for this page view when storage is blocked.
    }
    setChoice(next);
  };
  return { choice, select };
}

export function ThemeToggle() {
  const { choice, select } = useTheme();
  return <div className="theme-toggle inline-flex items-center h-[34px] px-1 rounded-full backdrop-blur-3xl bg-white/75 dark:bg-white/5 border border-white/80 dark:border-white/10 shadow-[inset_0_1px_1px_rgba(255,255,255,0.7)] gap-0.5" role="group" aria-label="Color theme">
    {choices.map(([value, label, Icon]) => {
      const active = choice === value;
      return <button
        type="button"
        key={value}
        aria-pressed={active}
        aria-label={label}
        title={label}
        onClick={() => select(value)}
        className={`w-6 h-6 rounded-full flex items-center justify-center p-0 transition-all duration-200 cursor-pointer active:scale-90 ${
          active
            ? "text-white scale-100 shadow-[0_2px_10px_rgba(14,165,233,0.45),inset_0_1px_1px_rgba(255,255,255,0.5)]"
            : "text-slate-700 dark:text-sky-300/80 hover:text-slate-950 dark:hover:text-white hover:bg-white/60 dark:hover:bg-white/10"
        }`}
        style={{
          background: active ? "linear-gradient(135deg, #0284c7 0%, #0ea5e9 100%)" : "transparent",
          border: "none",
        }}
      >
        <Icon size={13} className={active ? "text-white" : "text-slate-700 dark:text-sky-300"} aria-hidden="true" />
      </button>;
    })}
  </div>;
}
