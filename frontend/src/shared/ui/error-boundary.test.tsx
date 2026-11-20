import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { ErrorBoundary } from "./components";

afterEach(cleanup);

function Thrower({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error("Simulated render crash");
  }
  return <div>Component rendered successfully</div>;
}

describe("ErrorBoundary", () => {
  it("catches unhandled render errors and displays ErrorState notice", () => {
    // Suppress console.error during expected throw
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <ErrorBoundary>
        <Thrower shouldThrow={true} />
      </ErrorBoundary>
    );

    expect(screen.getByText("Simulated render crash")).toBeTruthy();
    expect(screen.getByText("We couldn't load this view")).toBeTruthy();
    expect(screen.getByRole("button", { name: /Retry/i })).toBeTruthy();

    errorSpy.mockRestore();
  });

  it("resets and recovers when clicking Retry after state fix", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    function StatefulTest() {
      const [crash, setCrash] = useState(true);
      return (
        <div>
          <button type="button" onClick={() => setCrash(false)}>Heal</button>
          <ErrorBoundary>
            <Thrower shouldThrow={crash} />
          </ErrorBoundary>
        </div>
      );
    }

    render(<StatefulTest />);
    expect(screen.getByText("Simulated render crash")).toBeTruthy();

    // Click heal to stop throwing, then click Retry on ErrorState
    fireEvent.click(screen.getByText("Heal"));
    fireEvent.click(screen.getByRole("button", { name: /Retry/i }));

    expect(screen.getByText("Component rendered successfully")).toBeTruthy();

    errorSpy.mockRestore();
  });
});
