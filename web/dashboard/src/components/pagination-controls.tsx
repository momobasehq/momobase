import { ChevronLeft, ChevronRight } from "lucide-react"

import { Button } from "@/components/ui/button"

interface PaginationControlsProps {
  page: number
  perPage: number
  /** Total rows across all pages, as reported by the API envelope. */
  total: number
  /** Rows on the current page, used to detect the last page. */
  count: number
  onPageChange: (page: number) => void
  busy?: boolean
}

/**
 * PaginationControls drives the page number for a list query. The legacy panel had
 * no pagination UI at all, so every list silently showed only its first page.
 */
export function PaginationControls({ page, perPage, total, count, onPageChange, busy }: PaginationControlsProps) {
  const first = total === 0 ? 0 : (page - 1) * perPage + 1
  const last = total === 0 ? 0 : first + count - 1
  const hasNext = last < total

  return (
    <div className="flex items-center justify-between gap-4 pt-2">
      <p className="text-muted-foreground text-xs" aria-live="polite">
        {total === 0 ? "No results" : `${first}–${last} of ${total}`}
      </p>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => onPageChange(page - 1)}
          disabled={busy || page <= 1}
          aria-label="Previous page"
        >
          <ChevronLeft />
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={() => onPageChange(page + 1)}
          disabled={busy || !hasNext}
          aria-label="Next page"
        >
          Next
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
