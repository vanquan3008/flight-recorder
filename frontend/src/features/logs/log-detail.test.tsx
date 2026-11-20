import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it } from "vitest";
import type { LogRecord } from "../../shared/types";
import { LogDetail } from "./log-detail";

afterEach(cleanup);

it("shows the log's own attributes and how many the SDK dropped", () => {
  const log: LogRecord = {
    event_id: "log-1",
    occurred_at: "2026-11-09T09:36:19.266Z",
    service_name: "checkout",
    body: "payment failed",
    resource_attributes: { "service.name": "checkout" },
    attributes: {
      "http.response.status_code": "500",
      "order.id": "o-42",
    },
    dropped_attributes_count: 3,
  };

  render(
    <MemoryRouter>
      <LogDetail log={log} body="payment failed" http={null} />
    </MemoryRouter>,
  );

  expect(screen.getByText("Log attributes (2)")).toBeTruthy();
  expect(screen.getByText("order.id")).toBeTruthy();
  expect(screen.getByText("o-42")).toBeTruthy();
  expect(screen.getByText("Resource attributes (1)")).toBeTruthy();
  expect(screen.getByRole("note").textContent).toContain("3 attributes");
});
