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
      vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ key: "tracepilot_ingest_secret" }), { status: 201 })));
      render(<MemoryRouter><Onboarding apiKey="admin-key" hasTrace={false} /></MemoryRouter>);
      fireEvent.click(screen.getByRole("button", { name: "Create ingestion key" }));
      await screen.findByText("tracepilot_ingest_secret");
      fireEvent.click(screen.getByRole("button", { name: "Copy OTLP config" }));
      expect((await screen.findByRole("status")).textContent).toContain("Clipboard access was denied");
    } finally {
      if (descriptor) Object.defineProperty(navigator, "clipboard", descriptor);
      else Reflect.deleteProperty(navigator, "clipboard");
    }
  });

  it("creates a least-privilege ingestion key and puts it in the OTLP template", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ key: "tracepilot_ingest_secret" }), { status: 201 })));
    render(<MemoryRouter><Onboarding apiKey="admin-key" hasTrace={false} /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: "Create ingestion key" }));

    expect(await screen.findByText("tracepilot_ingest_secret")).toBeTruthy();
    expect(screen.getByText(/x-api-key=tracepilot_ingest_secret/)).toBeTruthy();
    expect(fetch).toHaveBeenCalledWith("/api/v1/api-keys", expect.objectContaining({
      method: "POST",
      body: expect.stringContaining('"scopes":["ingest"]'),
    }));
  });
});
