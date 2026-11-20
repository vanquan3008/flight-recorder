import { useEffect, useState } from "react";
import { queryString, usePagedQuery } from "../../shared/api";
import { useAuth } from "../../shared/session";
import type { TraceSummary } from "../../shared/types";
import type { TablePagination } from "../../shared/ui";

export const PAGE_SIZE_OPTIONS = [10, 15, 25, 50];

export type TraceQueryFilters = {
  service?: string;
  status?: string;
  traceId?: string;
  from: string;
  to: string;
};

export function useTracesQuery(
  filters: TraceQueryFilters,
  initialPageSize = 15,
  onPageSizeChange?: (newSize: number) => void
) {
  const { credential } = useAuth();
  const validInitialSize = PAGE_SIZE_OPTIONS.includes(initialPageSize) ? initialPageSize : 15;
  const [pageSize, setPageSize] = useState(validInitialSize);
  const [pageIndex, setPageIndex] = useState(0);

  const filterKey = `${filters.service || ""}|${filters.status || ""}|${filters.traceId || ""}|${filters.from}|${filters.to}|${pageSize}`;
  useEffect(() => {
    setPageIndex(0);
  }, [filterKey]);

  const path = `/api/v1/traces?${queryString({
    service: filters.service || undefined,
    status: filters.status || undefined,
    traceId: filters.traceId || undefined,
    from: filters.from,
    to: filters.to,
    limit: String(pageSize),
  })}`;

  const query = usePagedQuery<TraceSummary>(["traces"], path, credential);
  const pages = query.data?.pages ?? [];
  const safePageIndex = Math.min(pageIndex, Math.max(0, pages.length - 1));
  const currentPage = pages[safePageIndex];
  const traces = currentPage?.data ?? [];

  const canPrev = safePageIndex > 0;
  const canNext = safePageIndex < pages.length - 1 || Boolean(query.hasNextPage);

  const handleNext = async () => {
    if (safePageIndex < pages.length - 1) {
      setPageIndex(safePageIndex + 1);
    } else if (query.hasNextPage && !query.isFetchingNextPage) {
      const res = await query.fetchNextPage();
      if (res.data && res.data.pages.length > pages.length) {
        setPageIndex(safePageIndex + 1);
      }
    }
  };

  const handlePrev = () => setPageIndex(Math.max(0, safePageIndex - 1));
  const handleSelectPage = (index: number) => {
    if (index >= 0 && index < pages.length) setPageIndex(index);
  };
  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize);
    setPageIndex(0);
    onPageSizeChange?.(newSize);
  };

  const pagination: TablePagination = {
    pageIndex: safePageIndex,
    pageSize,
    pageSizeOptions: PAGE_SIZE_OPTIONS,
    canPrev,
    canNext,
    isFetchingNext: query.isFetchingNextPage,
    pageCount: pages.length,
    totalCurrentPage: traces.length,
    onPrev: handlePrev,
    onNext: handleNext,
    onSelectPage: handleSelectPage,
    onPageSizeChange: handlePageSizeChange,
  };

  return { query, traces, safePageIndex, pagination };
}
