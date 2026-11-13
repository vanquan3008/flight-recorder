import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { DataTable, LoadMore } from "./table";
import { ExpandableText } from "./components";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("loads another page only when requested, even when the button is in view", () => {
  vi.stubGlobal("IntersectionObserver", class {
    constructor(private callback: IntersectionObserverCallback) {}
    observe() { this.callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver); }
    disconnect() {}
  });
  const load = vi.fn();
  render(<LoadMore hasMore loading={false} onClick={load} />);
  expect(load).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Load more" }));
  expect(load).toHaveBeenCalledTimes(1);
});

it("keeps expanded content attached to its log when new live rows arrive", () => {
  const first = "first ".repeat(100), second = "second ".repeat(100);
  const { rerender } = render(<DataTable headers={["Body"]} rowKeys={["first"]} rows={[[<ExpandableText value={first} />]]} />);
  fireEvent.click(screen.getByRole("button", { name: /Show all/ }));
  rerender(<DataTable headers={["Body"]} rowKeys={["second", "first"]} rows={[[<ExpandableText value={second} />], [<ExpandableText value={first} />]]} />);
  const rows = screen.getAllByRole("row");
  expect(within(rows[1]).getByRole("button", { name: /Show all/ })).toBeTruthy();
  expect(within(rows[2]).getByRole("button", { name: "Show less" })).toBeTruthy();
});
