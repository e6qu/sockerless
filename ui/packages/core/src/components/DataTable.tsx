import { useState } from "react";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  getFilteredRowModel,
  flexRender,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export interface DataTableProps<T> {
  data: T[];
  columns: ColumnDef<T, any>[];
  filterPlaceholder?: string;
  onRowClick?: (row: T) => void;
  /** Optional empty-state body when no rows match. */
  emptyMessage?: string;
}

/**
 * Operator-grade data table. Dense, monospace, sticky headers,
 * subtle row striping, no shadows, sharp corners. Filter input lives
 * inside the table chrome — single composed unit, no floating bar.
 * Hover background pulls the per-app accent in lightly so the row
 * announces itself without screaming.
 */
export function DataTable<T>({
  data,
  columns,
  filterPlaceholder,
  onRowClick,
  emptyMessage = "No rows match.",
}: DataTableProps<T>) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState("");

  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  });

  const rows = table.getRowModel().rows;

  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-sm)",
      }}
    >
      <div
        className="flex items-center justify-between gap-4 px-3 py-2"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <input
          type="text"
          value={globalFilter}
          onChange={(e) => setGlobalFilter(e.target.value)}
          placeholder={filterPlaceholder ?? "Search…"}
          aria-label="Filter table"
          className="w-full max-w-sm"
          style={{
            background: "transparent",
            border: "none",
            padding: "0.25rem 0",
            fontSize: "0.78rem",
          }}
        />
        <div
          className="font-mono text-[10px] uppercase tracking-[0.18em]"
          style={{ color: "var(--color-fg-subtle)" }}
        >
          {rows.length} / {data.length} rows
        </div>
      </div>

      <div className="overflow-x-auto">
        <table
          className="min-w-full font-mono"
          style={{ fontSize: "0.78rem", borderCollapse: "collapse" }}
        >
          <thead style={{ background: "var(--color-bg-subtle)" }}>
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id}>
                {hg.headers.map((header) => {
                  const sort = header.column.getIsSorted();
                  return (
                    <th
                      key={header.id}
                      onClick={header.column.getToggleSortingHandler()}
                      className="cursor-pointer select-none px-3 py-2 text-left uppercase tracking-[0.15em]"
                      style={{
                        fontSize: "0.62rem",
                        fontWeight: 500,
                        color: sort
                          ? "var(--color-accent)"
                          : "var(--color-fg-subtle)",
                        borderBottom: "1px solid var(--color-border)",
                        whiteSpace: "nowrap",
                      }}
                    >
                      <span className="inline-flex items-center gap-1">
                        {header.isPlaceholder
                          ? null
                          : flexRender(
                              header.column.columnDef.header,
                              header.getContext(),
                            )}
                        <span
                          aria-hidden
                          style={{ opacity: sort ? 1 : 0.25, fontSize: "0.7em" }}
                        >
                          {sort === "asc" ? "↑" : sort === "desc" ? "↓" : "↕"}
                        </span>
                      </span>
                    </th>
                  );
                })}
              </tr>
            ))}
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length}
                  className="px-3 py-8 text-center font-mono uppercase tracking-[0.2em]"
                  style={{
                    fontSize: "0.7rem",
                    color: "var(--color-fg-subtle)",
                  }}
                >
                  {emptyMessage}
                </td>
              </tr>
            ) : (
              rows.map((row, i) => (
                <tr
                  key={row.id}
                  onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                  data-row-index={i}
                  className="reveal"
                  style={{
                    background:
                      i % 2 === 0
                        ? "var(--color-surface)"
                        : "var(--color-bg-subtle)",
                    cursor: onRowClick ? "pointer" : undefined,
                    transition: "background-color 0.1s var(--ease-out-quint)",
                    "--reveal-delay": `${Math.min(i * 16, 240)}ms`,
                  } as React.CSSProperties}
                  onMouseEnter={(e) =>
                    (e.currentTarget.style.background =
                      "color-mix(in oklch, var(--color-accent-soft) 55%, var(--color-surface))")
                  }
                  onMouseLeave={(e) =>
                    (e.currentTarget.style.background =
                      i % 2 === 0
                        ? "var(--color-surface)"
                        : "var(--color-bg-subtle)")
                  }
                >
                  {row.getVisibleCells().map((cell) => (
                    <td
                      key={cell.id}
                      className="px-3 py-1.5"
                      style={{
                        borderBottom:
                          "1px solid color-mix(in oklch, var(--color-border) 60%, transparent)",
                        whiteSpace: "nowrap",
                        color: "var(--color-fg)",
                      }}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
