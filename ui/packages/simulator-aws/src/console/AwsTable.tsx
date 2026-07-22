import { useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { AwsButton, AwsContainer, AwsPageHeader } from "./AwsConsole.js";

export interface AwsColumn<T> {
  id: string;
  header: string;
  cell: (row: T) => ReactNode;
  /** Plain text used for filtering and sorting, so both act on what is shown. */
  value: (row: T) => string;
}

export interface AwsResourceTableProps<T> {
  title: string;
  breadcrumbLabel: string;
  description?: string;
  columns: AwsColumn<T>[];
  queryKey: unknown[];
  queryFn: () => Promise<T[]>;
  filterPlaceholder: string;
  /** Cloudscape empty states name the resource and offer the way to make one. */
  emptyTitle: string;
  emptyDescription: string;
  rowKey: (row: T) => string;
}

const PAGE_SIZE = 10;

export function AwsResourceTable<T>({
  title,
  description,
  columns,
  queryKey,
  queryFn,
  filterPlaceholder,
  emptyTitle,
  emptyDescription,
  rowKey,
}: AwsResourceTableProps<T>) {
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({ queryKey, queryFn });
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState<{ column: string; ascending: boolean } | null>(null);
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const rows = useMemo(() => {
    const all = data ?? [];
    const needle = filter.trim().toLowerCase();
    const matched = needle
      ? all.filter((row) => columns.some((column) => column.value(row).toLowerCase().includes(needle)))
      : all;
    if (!sort) return matched;
    const column = columns.find((candidate) => candidate.id === sort.column);
    if (!column) return matched;
    return [...matched].sort((left, right) => {
      const comparison = column.value(left).localeCompare(column.value(right), undefined, { numeric: true });
      return sort.ascending ? comparison : -comparison;
    });
  }, [data, filter, sort, columns]);

  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount - 1);
  const visible = rows.slice(currentPage * PAGE_SIZE, currentPage * PAGE_SIZE + PAGE_SIZE);
  const allVisibleSelected = visible.length > 0 && visible.every((row) => selected.has(rowKey(row)));

  function toggleSort(columnId: string) {
    setSort((current) =>
      current && current.column === columnId
        ? { column: columnId, ascending: !current.ascending }
        : { column: columnId, ascending: true },
    );
  }

  function toggleRow(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <>
      <AwsPageHeader
        title={title}
        count={data?.length}
        description={description}
        actions={
          <>
            <AwsButton disabled={selected.size !== 1}>View details</AwsButton>
            <AwsButton disabled={selected.size === 0}>Delete</AwsButton>
            <AwsButton onClick={() => void refetch()} disabled={isFetching}>
              {isFetching ? "Refreshing…" : "Refresh"}
            </AwsButton>
          </>
        }
      />
      <AwsContainer>
        <div className="aws-table-tools">
          <input
            className="aws-input"
            type="search"
            value={filter}
            placeholder={filterPlaceholder}
            aria-label={filterPlaceholder}
            onChange={(event) => {
              setFilter(event.target.value);
              setPage(0);
            }}
          />
          <div className="aws-pagination">
            <span className="aws-pagination-summary">
              {rows.length === 0 ? "0 matches" : `${currentPage * PAGE_SIZE + 1}–${currentPage * PAGE_SIZE + visible.length} of ${rows.length}`}
            </span>
            <button type="button" aria-label="Previous page" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>‹</button>
            <span aria-current="page">{currentPage + 1}</span>
            <button type="button" aria-label="Next page" disabled={currentPage >= pageCount - 1} onClick={() => setPage(currentPage + 1)}>›</button>
          </div>
        </div>

        {isError ? (
          <div className="aws-flash aws-flash-error" role="alert">
            <strong>Could not load {title.toLowerCase()}.</strong>{" "}
            {error instanceof Error ? error.message : "The simulator did not respond."}
          </div>
        ) : isLoading ? (
          <div className="aws-empty">Loading {title.toLowerCase()}…</div>
        ) : rows.length === 0 ? (
          <div className="aws-empty">
            <strong>{emptyTitle}</strong>
            <p>{emptyDescription}</p>
          </div>
        ) : (
          <table className="aws-table">
            <thead>
              <tr>
                <th className="aws-table-select">
                  <input
                    type="checkbox"
                    aria-label="Select all resources on this page"
                    checked={allVisibleSelected}
                    onChange={() =>
                      setSelected((current) => {
                        const next = new Set(current);
                        for (const row of visible) {
                          if (allVisibleSelected) next.delete(rowKey(row));
                          else next.add(rowKey(row));
                        }
                        return next;
                      })
                    }
                  />
                </th>
                {columns.map((column) => {
                  const active = sort?.column === column.id;
                  return (
                    <th key={column.id} aria-sort={active ? (sort.ascending ? "ascending" : "descending") : "none"}>
                      <button type="button" onClick={() => toggleSort(column.id)}>
                        {column.header}
                        <span aria-hidden className="aws-sort">{active ? (sort.ascending ? "▲" : "▼") : "⇅"}</span>
                      </button>
                    </th>
                  );
                })}
              </tr>
            </thead>
            <tbody>
              {visible.map((row) => {
                const id = rowKey(row);
                return (
                  <tr key={id} className={selected.has(id) ? "aws-row-selected" : undefined}>
                    <td className="aws-table-select">
                      <input
                        type="checkbox"
                        aria-label={`Select ${id}`}
                        checked={selected.has(id)}
                        onChange={() => toggleRow(id)}
                      />
                    </td>
                    {columns.map((column) => (
                      <td key={column.id}>{column.cell(row)}</td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </AwsContainer>
    </>
  );
}
