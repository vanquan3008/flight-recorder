import { describe, expect, it } from "vitest";

const sources = import.meta.glob<string>("/src/**/*.{ts,tsx}", { query: "?raw", import: "default", eager: true });

function resolve(from: string, specifier: string): string | undefined {
  if (!specifier.startsWith(".")) return undefined;
  const parts = from.split("/").slice(0, -1);
  for (const part of specifier.split("/")) {
    if (part === "..") parts.pop();
    else if (part !== ".") parts.push(part);
  }
  const base = parts.join("/");
  return [`${base}.ts`, `${base}.tsx`, `${base}/index.ts`, `${base}/index.tsx`, base].find((candidate) => candidate in sources);
}

const imports = new Map(Object.entries(sources).filter(([file]) => !/\.test\.tsx?$/.test(file)).map(([file, text]) => [
  file,
  [...text.matchAll(/(?:\bfrom\s*|\bimport\s*(?:\(\s*)?)["'](\.[^"']*)["']/g)].map((match) => resolve(file, match[1])).filter((target): target is string => Boolean(target)),
]));

const area = (file: string) => file.split("/")[2];
const feature = (file: string) => (file.startsWith("/src/features/") ? file.split("/")[3] : undefined);

describe("frontend module boundaries", () => {
  it("keeps shared code independent of app and features", () => {
    const violations = [...imports].flatMap(([file, targets]) => area(file) === "shared" ? targets.filter((target) => ["app", "features"].includes(area(target))).map((target) => `${file} -> ${target}`) : []);
    expect(violations).toEqual([]);
  });

  it("keeps features independent of app composition", () => {
    const violations = [...imports].flatMap(([file, targets]) => feature(file) ? targets.filter((target) => area(target) === "app").map((target) => `${file} -> ${target}`) : []);
    expect(violations).toEqual([]);
  });

  it("allows only the documented composition between features", () => {
    const allowed = new Set(["overview>onboarding", "overview>performance"]);
    const violations = [...imports].flatMap(([file, targets]) => targets
      .filter((target) => feature(file) && feature(target) && feature(file) !== feature(target) && !allowed.has(`${feature(file)}>${feature(target)}`))
      .map((target) => `${file} -> ${target}`));
    expect(violations).toEqual([]);
  });

  it("keeps test fixtures out of production code", () => {
    const violations = [...imports].flatMap(([file, targets]) => area(file) === "testing" ? [] : targets.filter((target) => area(target) === "testing").map((target) => `${file} -> ${target}`));
    expect(violations).toEqual([]);
  });

  it("has no import cycles", () => {
    const state = new Map<string, "visiting" | "done">();
    const cycles: string[] = [];
    const visit = (file: string, path: string[]) => {
      if (state.get(file) === "done") return;
      if (state.get(file) === "visiting") { cycles.push([...path.slice(path.indexOf(file)), file].join(" -> ")); return; }
      state.set(file, "visiting");
      (imports.get(file) ?? []).forEach((target) => visit(target, [...path, file]));
      state.set(file, "done");
    };
    [...imports.keys()].forEach((file) => visit(file, []));
    expect(cycles).toEqual([]);
  });

  it("finds the modules it is meant to guard", () => {
    expect(imports.size).toBeGreaterThan(20);
    expect([...imports.keys()].some((file) => file.startsWith("/src/features/logs/"))).toBe(true);
    expect(imports.get("/src/app/routes.tsx")).toContain("/src/features/traces/page.tsx");
  });
});
