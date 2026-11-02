import type { ReactNode } from "react";
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

export function PagedTable({ query, headers, rows, empty }: { query: PagedQuery; headers: string[]; rows: TableRow[]; empty: ReactNode }) {
  if (query.isPending) return <Loading />;
  if (query.isError) return <ErrorState error={query.error} retry={() => void query.refetch()} />;
  return (
    <>
      <DataTable headers={headers} rows={rows} empty={empty} />
      <LoadMore hasMore={query.hasNextPage} loading={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()} />
    </>
  );
}
