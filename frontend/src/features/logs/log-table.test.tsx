import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LogTable } from "./log-table";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("observes the scrollable log viewport and stops observing while another page is loading", () => {
  const load = vi.fn();
  const disconnect = vi.fn();
  let callback: IntersectionObserverCallback;
  const observer = vi.fn(function (this: object, cb: IntersectionObserverCallback, _options?: IntersectionObserverInit) {
    callback = cb;
    return { observe: vi.fn(), disconnect };
  });
  vi.stubGlobal("IntersectionObserver", observer);
  const { rerender } = render(<LogTable logs={[]} expanded={false} hasMore loading={false} onLoadMore={load} />);
  expect(observer.mock.calls[0][1]).toMatchObject({ root: screen.getByRole("region", { name: "Log records, scrollable" }) });
  callback!([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver);
  expect(load).toHaveBeenCalledTimes(1);
  rerender(<LogTable logs={[]} expanded={false} hasMore loading onLoadMore={load} />);
  expect(disconnect).toHaveBeenCalledTimes(1);
  expect(observer).toHaveBeenCalledTimes(1);
});

it("lets keyboard users request another page when intersection observation is unavailable", () => {
  vi.stubGlobal("IntersectionObserver", undefined);
  const load = vi.fn();
  render(<LogTable logs={[]} expanded={false} hasMore loading={false} onLoadMore={load} />);
  fireEvent.click(screen.getByRole("button", { name: "Load more logs" }));
  expect(load).toHaveBeenCalledTimes(1);
});
