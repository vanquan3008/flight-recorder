import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import type { LogRecord } from "../../shared/types";
import { LogRow } from "./log-row";

const log: LogRecord = { event_id: "log-1", occurred_at: "2026-11-09T09:36:19.266Z", service_name: "kalo-service", severity_text: "INFORMATION", body: "audit event", resource_attributes: {} };
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function open(item = log) { return render(<MemoryRouter><table><tbody><LogRow log={item} /></tbody></table></MemoryRouter>); }

it("keeps long messages compact and opens the complete payload with keyboard navigation", () => {
  const body = `audit event ${"long property ".repeat(200)}`;
  const { container } = open({ ...log, body, resource_attributes: { nested: { deployment: "production" } } });
  expect(container.querySelector(".log-detail-row")).toBeNull();
  fireEvent.keyDown(container.querySelector(".log-table-row")!, { key: "Enter" });
  expect(container.querySelector(".log-detail-row pre")?.textContent).toBe(body);
  const trigger = screen.getByRole("button", { name: "Collapse log details" });
  expect(document.getElementById(trigger.getAttribute("aria-controls")!)).not.toBeNull();
  fireEvent.click(trigger);
  expect(container.querySelector(".log-detail-row")).toBeNull();
});

it("copies the full payload without expanding the row and reports clipboard denial without changing columns", async () => {
  const writeText = vi.fn().mockRejectedValue(new Error("denied"));
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  const { container } = open();
  fireEvent.click(screen.getByRole("button", { name: "Copy log body" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(log.body));
  expect(container.querySelector(".log-detail-row")).toBeNull();
  const copy = screen.getByRole("button", { name: "Copy log body" });
  await waitFor(() => expect(copy.getAttribute("title")).toContain("Clipboard access was denied"));
  expect(screen.getByRole("status").textContent).toContain("copy it manually");
});

it("keeps HTTP metadata and raw payload available in details", () => {
  const body = 'HTTP Inbound "GET" "/api/v1/chats/123/members" returned 200 in 288.2ms\nRemote IP: "::1"\nRequest Headers: [{"Accept":"application/json"}]';
  const { container } = open({ ...log, body });
  fireEvent.click(screen.getByRole("button", { name: "Expand log details" }));
  const details = within(container.querySelector(".log-detail-row")! as HTMLElement);
  expect(details.getByText("::1")).toBeTruthy();
  expect(details.getByText("288.2ms")).toBeTruthy();
  expect(container.querySelector(".log-detail-row pre")?.textContent).toBe(body);
  fireEvent.click(details.getByText("Request headers (1)"));
  expect(details.getByText("application/json")).toBeTruthy();
});
