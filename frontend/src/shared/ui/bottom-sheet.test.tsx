import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { BottomSheet } from "./bottom-sheet";
import { mobileQuery } from "./media";
import { DataTable } from "./table";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

function Harness({ onClose = () => undefined }: { onClose?: () => void }) {
  const [open, setOpen] = useState(false);
  return <>
    <button type="button" onClick={() => setOpen(true)}>Open filters</button>
    <BottomSheet open={open} title="Filters" onClose={() => { onClose(); setOpen(false); }}>
      <button type="button">First</button>
      <button type="button">Last</button>
    </BottomSheet>
  </>;
}

describe("BottomSheet", () => {
  it("is a labelled modal that takes focus, keeps Tab inside and returns focus on Escape", () => {
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    const opener = screen.getByRole("button", { name: "Open filters" });
    opener.focus();
    fireEvent.click(opener);
    const dialog = screen.getByRole("dialog", { name: "Filters" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(document.body.style.overflow).toBe("hidden");
    const close = screen.getByRole("button", { name: "Close" });
    expect(document.activeElement).toBe(close);
    screen.getByRole("button", { name: "Last" }).focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(close);
    fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Last" }));
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.body.style.overflow).toBe("");
    expect(document.activeElement).toBe(opener);
  });
});

describe("DataTable on phones", () => {
  it("labels every value with its column so a row reads as a card", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({ matches: query === mobileQuery, addEventListener() {}, removeEventListener() {} }));
    render(<DataTable headers={["Started", "Service", ""]} rows={[["12:00", "checkout", <a href="#t" key="a">Inspect</a>]]} />);
    const cells = screen.getAllByRole("cell");
    expect(cells[0].textContent).toBe("12:00");
    expect(cells[1].textContent).toBe("Servicecheckout");
    expect(cells[2].textContent).toBe("Inspect");
  });

  it("keeps the plain table cells on wider screens", () => {
    render(<DataTable headers={["Started", "Service"]} rows={[["12:00", "checkout"]]} />);
    expect(screen.getAllByRole("cell")[1].textContent).toBe("checkout");
  });
});
