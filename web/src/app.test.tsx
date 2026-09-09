import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { App } from "./app";

afterEach(() => {
  cleanup();
  sessionStorage.clear();
});

describe("App", () => {
  it("keeps protected routes behind the session-only API key prompt", () => {
    render(<App />);
    expect(screen.getByRole("heading", { name: "Welcome to TracePilot" })).toBeTruthy();
    expect(screen.getByLabelText("API key")).toBeTruthy();
  });
});
