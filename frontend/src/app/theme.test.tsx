import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ThemeToggle, resolveTheme } from "./theme";

function mockSystem(light: boolean) {
  const listeners = new Set<() => void>();
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: query.includes("light") ? light : false, media: query,
    addEventListener: (_: string, listener: () => void) => listeners.add(listener), removeEventListener: (_: string, listener: () => void) => listeners.delete(listener),
  }));
  return listeners;
}

beforeEach(() => { localStorage.clear(); document.documentElement.removeAttribute("data-theme"); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("follows the system theme by default", () => {
  mockSystem(true);
  render(<ThemeToggle />);
  expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  expect(screen.getByRole("button", { name: "System theme" }).getAttribute("aria-pressed")).toBe("true");
});

it("stores a manual choice and restores it after reload", () => {
  mockSystem(true);
  const first = render(<ThemeToggle />);
  fireEvent.click(screen.getByRole("button", { name: "Dark theme" }));
  expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  expect(localStorage.getItem("tracepilot-theme")).toBe("dark");
  first.unmount();
  render(<ThemeToggle />);
  expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  expect(screen.getByRole("button", { name: "Dark theme" }).getAttribute("aria-pressed")).toBe("true");
});

it("returns to the system theme and tracks later system changes", () => {
  const listeners = mockSystem(false);
  localStorage.setItem("tracepilot-theme", "light");
  render(<ThemeToggle />);
  expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  fireEvent.click(screen.getByRole("button", { name: "System theme" }));
  expect(localStorage.getItem("tracepilot-theme")).toBeNull();
  expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  expect(listeners.size).toBe(1);
});

it("still works when storage is blocked", () => {
  mockSystem(false);
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked"); });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); });
  render(<ThemeToggle />);
  fireEvent.click(screen.getByRole("button", { name: "Light theme" }));
  expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  expect(resolveTheme("system")).toBe("dark");
  vi.restoreAllMocks();
});
