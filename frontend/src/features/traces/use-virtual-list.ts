import { useCallback, useEffect, useLayoutEffect, useMemo, useState, type RefObject } from "react";

export type VirtualItem = {
  index: number;
  top: number;
  height: number;
};

type VirtualListOptions = {
  count: number;
  estimateHeight: (index: number) => number;
  containerRef: RefObject<HTMLElement | null>;
  overscan?: number;
};

export function useVirtualList({
  count,
  estimateHeight,
  containerRef,
  overscan = 8,
}: VirtualListOptions) {
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  // Pre-calculate positions and heights for all items.
  const { positions, totalHeight } = useMemo(() => {
    const list: { top: number; height: number }[] = new Array(count);
    let accumulated = 0;
    for (let index = 0; index < count; index++) {
      const height = estimateHeight(index);
      list[index] = { top: accumulated, height };
      accumulated += height;
    }
    return { positions: list, totalHeight: accumulated };
  }, [count, estimateHeight]);

  const updateMeasurements = useCallback(() => {
    const element = containerRef.current;
    if (!element) return;
    setScrollTop(element.scrollTop);
    setViewportHeight(element.clientHeight);
  }, [containerRef]);

  useLayoutEffect(() => {
    updateMeasurements();
  }, [updateMeasurements, count]);

  useEffect(() => {
    const element = containerRef.current;
    if (!element) return;
    const onScroll = () => {
      setScrollTop(element.scrollTop);
    };
    element.addEventListener("scroll", onScroll, { passive: true });
    const observer = typeof ResizeObserver !== "undefined" ? new ResizeObserver(updateMeasurements) : null;
    observer?.observe(element);
    return () => {
      element.removeEventListener("scroll", onScroll);
      observer?.disconnect();
    };
  }, [containerRef, updateMeasurements]);

  const virtualItems = useMemo<VirtualItem[]>(() => {
    if (count === 0) return [];
    // Fallback: In unit test environment (JSDOM) or when height cannot be measured, render all items.
    if (viewportHeight === 0 || count <= 25) {
      return positions.map((position, index) => ({
        index,
        top: position.top,
        height: position.height,
      }));
    }

    const scrollBottom = scrollTop + viewportHeight;
    let startIndex = 0;
    while (startIndex < count && positions[startIndex].top + positions[startIndex].height < scrollTop) {
      startIndex++;
    }
    startIndex = Math.max(0, startIndex - overscan);

    let endIndex = startIndex;
    while (endIndex < count && positions[endIndex].top <= scrollBottom) {
      endIndex++;
    }
    endIndex = Math.min(count - 1, endIndex + overscan);

    const items: VirtualItem[] = [];
    for (let index = startIndex; index <= endIndex; index++) {
      items.push({
        index,
        top: positions[index].top,
        height: positions[index].height,
      });
    }
    return items;
  }, [count, viewportHeight, positions, scrollTop, overscan]);

  return {
    virtualItems,
    totalHeight,
    isVirtualized: count > 25,
  };
}
