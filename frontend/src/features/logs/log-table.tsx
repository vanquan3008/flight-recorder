import { LoaderCircle } from "lucide-react";
import { useEffect, useRef } from "react";
import type { LogRecord } from "../../shared/types";
import { LogRow, requestColumns } from "./log-row";

type Props = { logs: LogRecord[]; expanded: boolean; hasMore: boolean; loading: boolean; limitReached?: boolean; onLoadMore: () => unknown };
const heading = "sticky top-0 z-10 border-b border-line bg-surface-solid px-3 py-3 text-left text-[10px] font-semibold tracking-wider text-muted uppercase";

export function LogTable({ logs, expanded, hasMore, loading, limitReached, onLoadMore }: Props) {
  const viewport = useRef<HTMLDivElement>(null);
  const sentinel = useRef<HTMLTableRowElement>(null);
  useEffect(() => {
    if (!hasMore || loading || !sentinel.current || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void onLoadMore();
    }, { root: viewport.current, rootMargin: "0px 0px 250px 0px" });
    observer.observe(sentinel.current);
    return () => observer.disconnect();
  }, [hasMore, loading, onLoadMore]);

  return <div ref={viewport} className="table-wrap log-table-wrap min-h-0 w-full flex-1 overflow-auto overscroll-contain focus-visible:-outline-offset-2" tabIndex={0} role="region" aria-label="Log records, scrollable">
    <table className="log-table w-full min-w-[960px] table-fixed border-collapse text-xs">
      <caption className="sr-only">Log records. Scroll horizontally on smaller screens to see all columns. Expand a row to read the full message and request metadata.</caption>
      <colgroup><col className="w-10" /><col className="w-32" /><col className="w-[148px]" /><col className="w-[100px]" /><col /><col className="w-[110px]" /><col className="w-[52px]" /></colgroup>
      <thead><tr>
        <th scope="col" className={`${heading} px-1.5`}><span className="sr-only">Expand</span></th>
        <th scope="col" className={heading}>Time</th>
        <th scope="col" className={heading}>Service</th>
        <th scope="col" className={heading}>Severity</th>
        <th scope="col" className={heading}><div className={requestColumns}><span className="col-span-2">Request / message</span><span className="text-center">HTTP</span><span className="text-right">Duration</span></div></th>
        <th scope="col" className={heading}>Trace</th>
        <th scope="col" className={heading}><span className="sr-only">Copy</span></th>
      </tr></thead>
      <tbody>
        {logs.map((log) => <LogRow key={log.event_id} log={log} forceOpen={expanded} />)}
        {hasMore && <tr ref={sentinel} className="border-t border-line-soft"><td colSpan={7} className="px-4 py-4 text-center text-xs text-muted">
          <button type="button" disabled={loading} onClick={() => void onLoadMore()} className="inline-flex items-center gap-2 rounded-lg border border-line bg-glass-field px-3 py-1.5 text-[11px] font-medium hover:bg-hover">
            {loading ? <><LoaderCircle size={13} className="animate-spin" aria-hidden="true" />Loading more logs…</> : "Load more logs"}
          </button>
        </td></tr>}
      </tbody>
    </table>
    {limitReached && <p className="m-0 border-t border-line-soft p-4 text-center text-xs text-muted" role="note">The loaded page limit has been reached. Narrow the filters or time range to inspect more data.</p>}
  </div>;
}
