import type { ReactNode } from "react";
import { ChevronDown, LoaderCircle } from "lucide-react";
import { Empty } from "./components";
import { useIsMobile } from "./media";

export type TableRow = ReactNode[];

// On phones each row becomes a card: the first cell is its title and the others read "label  value",
// with the label taken from the column header (the header row itself is hidden there).
const mobileRow = "max-md:grid max-md:gap-1.5 max-md:rounded-2xl max-md:border max-md:border-glass-divider max-md:bg-surface-raised max-md:px-3.5 max-md:py-3";
const mobileCell = {
  title: "max-md:block max-md:border-0 max-md:p-0 max-md:pb-1 max-md:text-[13.5px] max-md:font-semibold max-md:break-words",
  labelled: "max-md:grid max-md:grid-cols-[minmax(76px,auto)_minmax(0,1fr)] max-md:items-baseline max-md:gap-3 max-md:border-0 max-md:p-0 max-md:text-right",
  plain: "max-md:block max-md:border-0 max-md:p-0 max-md:pt-1",
};
const cellKind = (index: number, header?: string) => index === 0 ? "title" : header ? "labelled" : "plain";

export function DataTable({ headers, rows, rowKeys, empty }: { headers: string[]; rows: TableRow[]; rowKeys?: string[]; empty?: ReactNode }) {
  const mobile = useIsMobile();
  if (!rows.length) return <Empty>{empty || "No data yet."}</Empty>;
  return (
    <div className="table-wrap flex-1 min-h-0 w-full overflow-y-auto overflow-x-hidden overscroll-contain focus-visible:-outline-offset-2" tabIndex={0} role="region" aria-label="Data table, scrollable">
      <table className="w-full min-w-0 border-collapse text-[12.5px] max-md:block">
        <thead className="max-md:hidden">
          <tr className="border-b border-black/5 dark:border-white/10">
            {headers.map((header) => (
              <th key={header} className="sticky top-0 z-10 border-b border-glass-divider bg-glass-strong px-4 py-2 text-left text-[10px] font-bold uppercase tracking-wider text-muted">
                {header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="max-md:grid max-md:gap-2 max-md:p-2.5">
          {rows.map((row, rowIndex) => (
            <tr key={rowKeys?.[rowIndex] ?? rowIndex} className={`transition-colors hover:bg-glass-hover last:[&>td]:border-b-0 ${mobileRow}`}>
              {row.map((cell, cellIndex) => (
                <td key={cellIndex} className={`border-b border-glass-divider px-4 py-2 align-middle font-medium text-text ${mobileCell[cellKind(cellIndex, headers[cellIndex])]}`}>
                  {mobile && cellKind(cellIndex, headers[cellIndex]) === "labelled" ? <>
                    <span className="text-left text-[10.5px] font-semibold tracking-wide text-faint uppercase">{headers[cellIndex]}</span>
                    <span className="min-w-0 break-words">{cell}</span>
                  </> : cell}
                </td>
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

