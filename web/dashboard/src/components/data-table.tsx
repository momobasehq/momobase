import type { ReactNode } from "react"

import { Skeleton } from "@/components/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

/** Column describes one column of a DataTable. */
export interface Column<T> {
  /** Stable identifier, also used as the React key. */
  key: string
  /** Header label. */
  header: ReactNode
  /** Renders the cell for one row. */
  cell: (row: T) => ReactNode
  /** Right-aligns numeric columns. */
  align?: "start" | "end"
}

interface DataTableProps<T> {
  columns: Column<T>[]
  rows: T[]
  rowKey: (row: T) => string
  loading?: boolean
  /** Shown instead of rows when the result set is empty. */
  empty?: ReactNode
}

/**
 * DataTable renders a list response as a table. It is a composition of shadcn/ui's
 * Table parts rather than a component of its own, so it inherits the theme and every
 * screen's tables stay identical without a shared abstraction to maintain.
 */
export function DataTable<T>({ columns, rows, rowKey, loading, empty }: DataTableProps<T>) {
  return (
    // Wide tables scroll inside their own container rather than the page.
    <div className="w-full overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((column) => (
              <TableHead key={column.key} className={column.align === "end" ? "text-end" : undefined}>
                {column.header}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading && rows.length === 0
            ? Array.from({ length: 5 }, (_, index) => (
                <TableRow key={`skeleton-${index}`}>
                  {columns.map((column) => (
                    <TableCell key={column.key}>
                      <Skeleton className="h-4 w-full" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            : rows.map((row) => (
                <TableRow key={rowKey(row)}>
                  {columns.map((column) => (
                    <TableCell key={column.key} className={column.align === "end" ? "text-end" : undefined}>
                      {column.cell(row)}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
          {!loading && rows.length === 0 && (
            <TableRow>
              <TableCell colSpan={columns.length} className="text-muted-foreground py-8 text-center">
                {empty ?? "Nothing to show yet."}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )
}
