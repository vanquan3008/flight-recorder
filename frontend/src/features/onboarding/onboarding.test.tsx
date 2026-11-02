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
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) } });
    try {
      vi.stubGlobal("fetch", vi.fn().mockImplementation(() => new Response(JSON.stringify({ jwt: "tracepilot_ingest_secret" }), { status: 201 })));
      render(<MemoryRouter><Onboarding credential="session" projectID="project-1" canCreate hasTrace={false} /></MemoryRouter>);
      fireEvent.click(screen.getByRole("button", { name: "Create ingest token" }));
      await screen.findByText("tracepilot_ingest_secret");
      fireEvent.click(screen.getByRole("button", { name: "Copy OTLP config" }));
      expect((await screen.findByRole("status")).textContent).toContain("Clipboard access was denied");
    } finally {
      if (descriptor) Object.defineProperty(navigator, "clipboard", descriptor);
      else Reflect.deleteProperty(navigator, "clipboard");
    }
  });

  it("creates an ingest token for the project and puts it in the OTLP template", async () => {
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => new Response(JSON.stringify({ jwt: "tracepilot_ingest_secret" }), { status: 201 })));
    render(<MemoryRouter><Onboarding credential="session" projectID="project-1" canCreate hasTrace={false} /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: "Create ingest token" }));

    expect(await screen.findByText("tracepilot_ingest_secret")).toBeTruthy();
    expect(screen.getByText(/authorization=Bearer%20tracepilot_ingest_secret/)).toBeTruthy();
    expect(fetch).toHaveBeenCalledWith("/api/v1/projects/project-1/ingest-tokens", expect.objectContaining({ method: "POST" }));
  });
});

describe("Onboarding for a user who is not an owner", () => {
  it("asks an owner for the token instead of offering to create one", () => {
    cleanup();
    render(<MemoryRouter><Onboarding credential="session" projectID="project-1" canCreate={false} hasTrace={false} /></MemoryRouter>);
    expect(screen.queryByRole("button", { name: "Create ingest token" })).toBeNull();
    expect(screen.getByText(/Ask an owner of this project/)).toBeTruthy();
  });
});
