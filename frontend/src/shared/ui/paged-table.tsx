import type { ReactNode } from "react";
import { ChevronLeft, ChevronRight, LoaderCircle } from "lucide-react";
import { DataTable, ErrorState, LoadMore, Loading, type TableRow } from "./components";

export type PagedQuery = {
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => Promise<unknown>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => Promise<unknown>;
};

export type TablePagination = {
  pageIndex: number;
  pageSize: number;
  pageSizeOptions?: number[];
  canPrev: boolean;
  canNext: boolean;
  isFetchingNext?: boolean;
  pageCount?: number;
  totalCurrentPage?: number;
  onPrev: () => void;
  onNext: () => void;
  onSelectPage?: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
};

export function PaginationBar({ pagination }: { pagination: TablePagination }) {
  const {
    pageIndex,
    pageSize,
    pageSizeOptions = [10, 15, 25, 50],
    canPrev,
    canNext,
    isFetchingNext,
    pageCount = pageIndex + 1,
    totalCurrentPage,
    onPrev,
    onNext,
    onSelectPage,
    onPageSizeChange,
  } = pagination;

  return (
    <div className="pagination-bar shrink-0 flex flex-wrap items-center justify-between gap-3 px-6 py-3 border-t border-black/5 dark:border-white/10 text-xs text-slate-600 dark:text-slate-300">
      <div className="flex items-center gap-3">
        <span className="font-medium">
          Trang <span className="font-bold text-slate-900 dark:text-white">{pageIndex + 1}</span>
          {typeof totalCurrentPage === "number" && (
            <span className="text-slate-400 dark:text-slate-400 font-normal"> · {totalCurrentPage} kết quả</span>
          )}
        </span>
        {onPageSizeChange && (
          <div className="flex items-center gap-1.5 ml-2">
            <span className="text-slate-400 dark:text-slate-400">Hiển thị:</span>
            <select
              aria-label="Số dòng mỗi trang"
              value={pageSize}
              onChange={(e) => onPageSizeChange(Number(e.target.value))}
              className="bg-white/80 dark:bg-slate-800/80 border border-slate-200 dark:border-white/10 rounded-lg px-2 py-1 text-xs font-semibold text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-1 focus:ring-sky-500 cursor-pointer"
            >
              {pageSizeOptions.map((size) => (
                <option key={size} value={size}>
                  {size} / trang
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={onPrev}
          disabled={!canPrev}
          className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-semibold border transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed border-black/10 dark:border-white/10 hover:bg-black/5 dark:hover:bg-white/5 active:scale-95 disabled:active:scale-100"
          aria-label="Trang trước"
        >
          <ChevronLeft size={14} />
          Trước
        </button>

        {onSelectPage && pageCount > 1 && (
          <div className="flex items-center gap-1 mx-1" role="navigation" aria-label="Các trang đã xem">
            {Array.from({ length: pageCount }, (_, i) => (
              <button
                key={i}
                type="button"
                onClick={() => onSelectPage(i)}
                className={`w-7 h-7 rounded-lg text-xs font-bold transition-all cursor-pointer ${
                  i === pageIndex
                    ? "bg-sky-500 text-white shadow-xs"
                    : "border border-black/10 dark:border-white/10 hover:bg-black/5 dark:hover:bg-white/5 text-slate-700 dark:text-slate-300"
                }`}
                aria-label={`Trang ${i + 1}`}
                aria-current={i === pageIndex ? "page" : undefined}
              >
                {i + 1}
              </button>
            ))}
          </div>
        )}

        <button
          type="button"
          onClick={onNext}
          disabled={!canNext || isFetchingNext}
          className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg text-xs font-semibold border transition-all cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed border-black/10 dark:border-white/10 hover:bg-black/5 dark:hover:bg-white/5 active:scale-95 disabled:active:scale-100"
          aria-label="Trang sau"
        >
          {isFetchingNext ? (
            <>
              <LoaderCircle size={14} className="animate-spin" />
              Đang tải…
            </>
          ) : (
            <>
              Sau
              <ChevronRight size={14} />
            </>
          )}
        </button>
      </div>
    </div>
  );
}

export function PagedTable({
  query,
  headers,
  rows,
  empty,
  pagination,
}: {
  query: PagedQuery;
  headers: string[];
  rows: TableRow[];
  empty: ReactNode;
  pagination?: TablePagination;
}) {
  if (query.isPending) return <Loading />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  return (
    <>
      <DataTable headers={headers} rows={rows} empty={empty} />
      {pagination ? (
        (rows.length > 0 || pagination.canPrev) && <PaginationBar pagination={pagination} />
      ) : (
        <LoadMore hasMore={query.hasNextPage} loading={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()} />
      )}
    </>
  );
}
