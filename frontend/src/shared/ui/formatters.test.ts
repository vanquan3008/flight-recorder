import { expect, it } from "vitest";
import { formatTime, formatTimeParts } from "./formatters";

it("keeps millisecond event ordering visible and tolerates missing timestamps", () => {
  expect(formatTime("2026-10-01T00:00:00.001Z")).not.toBe(formatTime("2026-10-01T00:00:00.002Z"));
  expect(formatTime("2026-10-01T00:00:00.123Z")).toContain("123");
  expect(formatTime("invalid")).toBe("—");
  expect(formatTime()).toBe("—");
});

it("splits timestamps into time and date parts for stacked views", () => {
  const parts = formatTimeParts("2026-10-01T14:30:15.456Z");
  expect(parts).not.toBeNull();
  expect(parts?.time).toContain("456");
  expect(parts?.date).toBeTruthy();
  expect(parts?.full).toBeTruthy();
  expect(formatTimeParts("invalid")).toBeNull();
  expect(formatTimeParts()).toBeNull();
});
