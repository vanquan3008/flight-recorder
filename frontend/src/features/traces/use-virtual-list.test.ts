import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { useVirtualList } from "./use-virtual-list";

describe("useVirtualList", () => {
  it("returns empty virtual items when count is zero", () => {
    const container = { current: null };
    const { result } = renderHook(() =>
      useVirtualList({
        count: 0,
        estimateHeight: () => 36,
        containerRef: container,
      }),
    );
    expect(result.current.virtualItems).toEqual([]);
    expect(result.current.totalHeight).toBe(0);
  });

  it("renders all items when count is small or unmeasured (JSDOM fallback)", () => {
    const container = { current: document.createElement("div") };
    const { result } = renderHook(() =>
      useVirtualList({
        count: 10,
        estimateHeight: () => 36,
        containerRef: container,
      }),
    );
    expect(result.current.virtualItems.length).toBe(10);
    expect(result.current.totalHeight).toBe(360);
    expect(result.current.virtualItems[0].top).toBe(0);
    expect(result.current.virtualItems[9].top).toBe(9 * 36);
  });
});
