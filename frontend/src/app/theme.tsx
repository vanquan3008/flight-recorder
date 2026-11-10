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
  return <div className="theme-toggle inline-flex h-8.5 items-center gap-0.5 rounded-full border border-glass-field-edge bg-glass-field p-0.75" role="group" aria-label="Color theme">
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
            ? "bg-glass-strong text-accent"
            : "text-muted hover:bg-glass-hover hover:text-text"
        }`}
      >
        <Icon size={13} aria-hidden="true" />
      </button>;
    })}
  </div>;
}
