/** Lower-case CSS key of a node type, such as "database" for "Database". */
export const kindClass = (type: string) => type.toLowerCase().replace(/[^a-z0-9]/g, "");

// Tailwind only generates classes it finds written out, so each color keeps its full class names.
const external = { dot: "bg-violet", stripe: "fill-violet" };
const database = { dot: "bg-cyan", stripe: "fill-cyan" };
const messaging = { dot: "bg-warning", stripe: "fill-warning" };
const service = { dot: "bg-accent", stripe: "fill-accent" };

function palette(type: string) {
  const kind = kindClass(type);
  if (kind === "external") return external;
  if (kind === "database" || kind === "db") return database;
  if (kind === "messaging" || kind === "queue") return messaging;
  return service;
}

export const typeDotBg = (type: string) => palette(type).dot;
export const typeStripeFill = (type: string) => palette(type).stripe;

export const plural = (value: number, noun: string) => `${value.toLocaleString()} ${noun}${value === 1 ? "" : "s"}`;
