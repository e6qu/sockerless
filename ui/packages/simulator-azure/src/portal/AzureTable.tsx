import { useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { AzureCommandBar, AzureEssentials, type EssentialsProperty } from "./AzurePortal.js";

export interface AzureColumn<T> {
  id: string;
  header: string;
  cell: (row: T) => ReactNode;
  /** Plain text used for filtering and sorting, so both act on what is shown. */
  value: (row: T) => string;
}

export interface AzureResourceTableProps<T> {
  columns: AzureColumn<T>[];
  queryKey: unknown[];
  queryFn: () => Promise<T[]>;
  filterPlaceholder: string;
  emptyTitle: string;
  emptyDescription: string;
  rowKey: (row: T) => string;
  /** Resource-level properties the portal leads the pane with. */
  essentials: (rows: T[]) => EssentialsProperty[];
  resourceNoun: string;
}

const PAGE_SIZE = 10;

export function AzureResourceTable<T>({
  columns,
  queryKey,
  queryFn,
  filterPlaceholder,
  emptyTitle,
  emptyDescription,
  rowKey,
  essentials,
  resourceNoun,
}: AzureResourceTableProps<T>) {
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

  return (
    <>
      <AzureCommandBar
        commands={[
          { label: "Refresh", icon: "refresh", onSelect: () => void refetch(), disabled: isFetching },
          { label: "Delete", icon: "delete", disabled: selected.size === 0 },
          { label: "Assign tags", icon: "tag", disabled: selected.size === 0 },
          { label: "Export to CSV", icon: "download", disabled: rows.length === 0 },
          { label: "Feedback", icon: "feedback" },
        ]}
      />
      <div className="az-main">
        <AzureEssentials properties={essentials(data ?? [])} />
        <div className="az-table-tools">
          <input
            className="az-input"
            type="search"
            value={filter}
            placeholder={filterPlaceholder}
            aria-label={filterPlaceholder}
            onChange={(event) => {
              setFilter(event.target.value);
              setPage(0);
            }}
          />
          <div className="az-pagination">
            <span>
              {rows.length === 0
                ? `No ${resourceNoun}`
                : `${currentPage * PAGE_SIZE + 1}–${currentPage * PAGE_SIZE + visible.length} of ${rows.length}`}
            </span>
            <button type="button" aria-label="Previous page" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>‹</button>
            <span aria-current="page">{currentPage + 1}</span>
            <button type="button" aria-label="Next page" disabled={currentPage >= pageCount - 1} onClick={() => setPage(currentPage + 1)}>›</button>
          </div>
        </div>

        {/* The column headers stay whatever the body holds, so what the
         * resource is described by remains readable while it is loading,
         * empty, or failed. */}
        <table className="az-table">
          <thead>
            <tr>
              <th className="az-table-select">
                <input
                  type="checkbox"
                  aria-label="Select all resources on this page"
                  checked={allVisibleSelected}
                  disabled={visible.length === 0}
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
                      <span aria-hidden className="az-sort">{active ? (sort.ascending ? "▲" : "▼") : "⇅"}</span>
                    </button>
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {isError ? (
              <tr>
                <td className="az-table-state" colSpan={columns.length + 1}>
                  <div className="az-message az-message-error" role="alert">
                    <strong>Could not load {resourceNoun}.</strong>{" "}
                    {error instanceof Error ? error.message : "The simulator did not respond."}
                  </div>
                </td>
              </tr>
            ) : isLoading ? (
              <tr>
                <td className="az-table-state" colSpan={columns.length + 1}>
                  <div className="az-empty" role="status">Loading {resourceNoun}…</div>
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td className="az-table-state" colSpan={columns.length + 1}>
                  <div className="az-empty">
                    <strong>{emptyTitle}</strong>
                    <p>{emptyDescription}</p>
                  </div>
                </td>
              </tr>
            ) : (
              visible.map((row) => {
                const id = rowKey(row);
                return (
                  <tr key={id} className={selected.has(id) ? "az-row-selected" : undefined}>
                    <td className="az-table-select">
                      <input
                        type="checkbox"
                        aria-label={`Select ${id}`}
                        checked={selected.has(id)}
                        onChange={() =>
                          setSelected((current) => {
                            const next = new Set(current);
                            if (next.has(id)) next.delete(id);
                            else next.add(id);
                            return next;
                          })
                        }
                      />
                    </td>
                    {columns.map((column) => (
                      <td key={column.id}>{column.cell(row)}</td>
                    ))}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
