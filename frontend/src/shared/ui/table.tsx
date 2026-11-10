import type { ReactNode } from "react";
import { ChevronDown, LoaderCircle } from "lucide-react";
import { Empty } from "./components";

export type TableRow = ReactNode[];

export function DataTable({ headers, rows, rowKeys, empty }: { headers: string[]; rows: TableRow[]; rowKeys?: string[]; empty?: ReactNode }) {
  if (!rows.length) return <Empty>{empty || "No data yet."}</Empty>;
  return (
    <div className="table-wrap flex-1 min-h-0 w-full overflow-y-auto overflow-x-hidden overscroll-contain focus-visible:-outline-offset-2" tabIndex={0} role="region" aria-label="Data table, scrollable">
      <table className="w-full min-w-0 border-collapse text-[12.5px]">
        <thead>
          <tr className="border-b border-black/5 dark:border-white/10">
            {headers.map((header) => (
              <th key={header} className="sticky top-0 z-10 border-b border-glass-divider bg-glass-strong px-4 py-2 text-left text-[10px] font-bold uppercase tracking-wider text-muted">
                {header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr key={rowKeys?.[rowIndex] ?? rowIndex} className="transition-colors hover:bg-glass-hover last:[&>td]:border-b-0">
              {row.map((cell, cellIndex) => (
                <td key={cellIndex} className="border-b border-glass-divider px-4 py-2 align-middle font-medium text-text">{cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function LoadMore({
  hasMore,
  loading,
  onClick,
}: {
  hasMore: boolean;
  loading: boolean;
  onClick: () => void;
}) {
  if (!hasMore) return null;

  return (
    <div className="load-more flex items-center justify-center rounded-b-3xl border-t border-line-soft bg-surface-raised p-4 [&>button]:inline-flex [&>button]:items-center [&>button]:gap-2 [&>button]:rounded-full [&>button]:border [&>button]:border-line [&>button]:bg-surface [&>button]:px-4.5 [&>button]:py-1.5 [&>button]:text-xs [&>button]:font-semibold [&>button]:text-muted [&>button:hover]:bg-hover [&>button:hover]:text-text">
      <button disabled={loading} onClick={onClick} aria-label={loading ? "Loading…" : "Load more"}>
        {loading ? (
          <>
            <LoaderCircle size={14} className="animate-spin" aria-hidden="true" />
            <span>Loading…</span>
          </>
        ) : (
          <>
            <ChevronDown size={16} aria-hidden="true" />
            <span>Load more</span>
          </>
        )}
      </button>
    </div>
  );
}

