import { Search } from "lucide-react";
import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useAuth } from "../../app/auth";
import { compactSearchParams, queryString, usePagedQuery } from "../../shared/api";
import type { TraceSummary } from "../../shared/types";
import { Badge, DetailLink, ErrorState, PageHeader, PagedTable, Panel, formatDuration, formatTime } from "../../shared/ui";
import "./traces.css";

const PAGE_SIZE_OPTIONS = [10, 15, 25, 50];

export function TracesPage() {
  const { credential } = useAuth();
  const [params, setParams] = useSearchParams();
  const [pageSize, setPageSize] = useState<number>(() => {
    const raw = Number(params.get("limit"));
    return PAGE_SIZE_OPTIONS.includes(raw) ? raw : 15;
  });
  const [pageIndex, setPageIndex] = useState(0);

  // Reset to first page when search filters change
  const filterKey = `${params.get("service") || ""}|${params.get("status") || ""}|${params.get("traceId") || ""}|${params.get("from") || ""}|${params.get("to") || ""}|${pageSize}`;
  useEffect(() => {
    setPageIndex(0);
  }, [filterKey]);

  const path = `/api/v1/traces?${queryString({
    service: params.get("service") || undefined,
    status: params.get("status") || undefined,
    traceId: params.get("traceId") || undefined,
    from: params.get("from") || undefined,
    to: params.get("to") || undefined,
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

  const handlePrev = () => {
    setPageIndex(Math.max(0, safePageIndex - 1));
  };

  const handleSelectPage = (index: number) => {
    if (index >= 0 && index < pages.length) {
      setPageIndex(index);
    }
  };

  const handlePageSizeChange = (newSize: number) => {
    setPageSize(newSize);
    setPageIndex(0);
    const next = new URLSearchParams(params);
    next.set("limit", String(newSize));
    setParams(next);
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setParams(compactSearchParams(event.currentTarget, params, ["from", "to", "limit"]));
  };

  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} fullPage />;

  return (
    <div className="trace-explorer-page">
      <div className="shrink-0">
        <PageHeader title="Trace explorer">Find slow or failed requests, then inspect their spans and correlated logs.</PageHeader>
        <form className="toolbar" onSubmit={submit}>
          <input name="service" placeholder="Service" aria-label="Filter by service" defaultValue={params.get("service") || ""} />
          <input name="status" placeholder="Status" aria-label="Filter by status" defaultValue={params.get("status") || ""} />
          <input name="traceId" placeholder="Trace ID" aria-label="Filter by trace ID" defaultValue={params.get("traceId") || ""} />
          <button><Search size={16} /> Search traces</button>
        </form>
      </div>
      <Panel
        title="Recent traces"
        className="panel-fit"
        meta={query.isPending ? undefined : `Trang ${safePageIndex + 1} · ${traces.length} traces`}
      >
        <PagedTable
          query={query}
          pagination={{
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
          }}
          empty={<>No traces match those filters. <Link to="/overview">Review the setup steps.</Link></>}
          headers={["Started", "Service", "Duration", "Spans", "Errors", "Trace ID", ""]}
          rows={traces.map((trace) => [
            formatTime(trace.started_at),
            trace.root_service,
            formatDuration(trace.duration_nanos),
            trace.span_count,
            <Badge value={trace.error_count ? "ERROR" : "OK"} />,
            <code>{trace.trace_id}</code>,
            <DetailLink to={`/traces/${trace.trace_id}`}>Inspect</DetailLink>,
          ])}
        />
      </Panel>
    </div>
  );
}

export { TraceDetailPage } from "./trace-detail";
