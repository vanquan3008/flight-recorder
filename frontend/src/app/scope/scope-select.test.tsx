import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ScopeSelect } from "./scope-select";

const options = [
  { value: "", label: "Default environment" },
  { value: "dev", label: "development" },
  { value: "prod", label: "production" },
];
afterEach(cleanup);

it("opens the selected option and commits keyboard navigation only on Enter", () => {
  const onChange = vi.fn();
  render(<ScopeSelect label="Environment" value="dev" options={options} onChange={onChange} />);
  const trigger = screen.getByRole("combobox", { name: "Environment" });
  trigger.focus();
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  expect(screen.getByRole("option", { name: "development" }).getAttribute("aria-selected")).toBe("true");
  expect(trigger.getAttribute("aria-activedescendant")).toBe(screen.getByRole("option", { name: "development" }).id);
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  expect(onChange).not.toHaveBeenCalled();
  fireEvent.keyDown(trigger, { key: "Enter" });
  expect(onChange).toHaveBeenCalledWith("prod");
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(document.activeElement).toBe(trigger);
});

it("supports Home, End and typing, while Escape and Tab dismiss without changing scope", () => {
  const onChange = vi.fn();
  render(<ScopeSelect label="Environment" value="dev" options={options} onChange={onChange} />);
  const trigger = screen.getByRole("combobox", { name: "Environment" });
  fireEvent.keyDown(trigger, { key: "End" });
  expect(trigger.getAttribute("aria-activedescendant")).toBe(screen.getByRole("option", { name: "production" }).id);
  fireEvent.keyDown(trigger, { key: "Home" });
  expect(trigger.getAttribute("aria-activedescendant")).toBe(screen.getByRole("option", { name: "Default environment" }).id);
  fireEvent.keyDown(trigger, { key: "Escape" });
  expect(screen.queryByRole("listbox")).toBeNull();
  fireEvent.keyDown(trigger, { key: "p" });
  expect(trigger.getAttribute("aria-activedescendant")).toBe(screen.getByRole("option", { name: "production" }).id);
  fireEvent.keyDown(trigger, { key: "Tab" });
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(onChange).not.toHaveBeenCalled();
});

it("keeps the list outside clipping containers, selects by pointer and dismisses outside clicks", () => {
  const onChange = vi.fn();
  const { container } = render(<ScopeSelect label="Environment" value="dev" options={options} onChange={onChange} />);
  const trigger = screen.getByRole("combobox", { name: "Environment" });
  fireEvent.click(trigger);
  expect(container.contains(screen.getByRole("listbox"))).toBe(false);
  fireEvent.click(screen.getByRole("option", { name: "Default environment" }));
  expect(onChange).toHaveBeenCalledWith("");
  expect(screen.queryByRole("listbox")).toBeNull();
  fireEvent.click(trigger);
  fireEvent.pointerDown(document.body);
  expect(screen.queryByRole("listbox")).toBeNull();
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("option", { name: "development" }));
  expect(onChange).toHaveBeenCalledTimes(1);
});
