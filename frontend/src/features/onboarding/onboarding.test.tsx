import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Onboarding } from "./onboarding";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Onboarding", () => {
  it("explains how to copy the template when clipboard access is denied", async () => {
    const descriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    try {
      vi.stubGlobal("fetch", vi.fn().mockImplementation(() => new Response(JSON.stringify({ jwt: "tracepilot_ingest_secret" }), { status: 201 })));
      render(<MemoryRouter><Onboarding credential="session" projectID="project-1" environment="staging" canCreate hasTrace={false} /></MemoryRouter>);
      fireEvent.click(screen.getByRole("button", { name: "Create ingest token" }));
      await screen.findByText("tracepilot_ingest_secret");
      fireEvent.click(screen.getByRole("button", { name: "Copy OTLP config" }));
      expect((await screen.findByRole("status")).textContent).toContain("Clipboard access was denied");
      expect(writeText).toHaveBeenCalledWith(expect.stringContaining("authorization=Bearer%20tracepilot_ingest_secret,x-environment=staging"));
    } finally {
      if (descriptor) Object.defineProperty(navigator, "clipboard", descriptor);
      else Reflect.deleteProperty(navigator, "clipboard");
    }
  });

  it("puts the issued token and encoded environment in the OTLP template", async () => {
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => new Response(JSON.stringify({ jwt: "tracepilot_ingest_secret" }), { status: 201 })));
    render(<MemoryRouter><Onboarding credential="session" projectID="project-1" environment="QA, West" canCreate hasTrace={false} /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: "Create ingest token" }));

    expect(await screen.findByText("tracepilot_ingest_secret")).toBeTruthy();
    expect(screen.getByText(/authorization=Bearer%20tracepilot_ingest_secret,x-environment=QA%2C%20West/)).toBeTruthy();
    expect(fetch).toHaveBeenCalledWith("/api/v1/projects/project-1/ingest-tokens", expect.objectContaining({ method: "POST" }));
  });
});

describe("Onboarding for a user who is not an owner", () => {
  it("asks an owner for the token instead of offering to create one", () => {
    cleanup();
    render(<MemoryRouter><Onboarding credential="session" projectID="project-1" environment="" canCreate={false} hasTrace={false} /></MemoryRouter>);
    expect(screen.queryByRole("button", { name: "Create ingest token" })).toBeNull();
    expect(screen.getByText(/Ask an owner of this project/)).toBeTruthy();
    expect(screen.queryByText(/x-environment=/)).toBeNull();
  });
});
