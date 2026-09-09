import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Onboarding } from "./onboarding";

afterEach(() => cleanup());

describe("Onboarding", () => {
  it("explains how to copy the template when clipboard access is denied", async () => {
    const descriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) } });
    try {
      render(<MemoryRouter><Onboarding hasTrace={false} /></MemoryRouter>);
      fireEvent.click(screen.getByRole("button", { name: "Copy OTLP config" }));
      expect((await screen.findByRole("status")).textContent).toContain("Clipboard access was denied");
    } finally {
      if (descriptor) Object.defineProperty(navigator, "clipboard", descriptor);
      else Reflect.deleteProperty(navigator, "clipboard");
    }
  });
});
