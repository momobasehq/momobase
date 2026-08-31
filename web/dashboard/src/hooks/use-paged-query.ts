import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { PaginatedData } from "momobase";

import type { Page } from "@/lib/query-keys";

const defaultPerPage = 20;

/**
 * usePagedQuery wires a list endpoint to page state.
 *
 * `keepPreviousData` is the point: without it every page change unmounts the table
 * back to a skeleton, so paging through transactions flickers. With it the previous
 * page stays on screen, dimmed, until the next one resolves.
 */
export function usePagedQuery<T>(
	queryKey: (page: Page) => readonly unknown[],
	fetcher: (page: Page) => Promise<PaginatedData<T>>,
	perPage = defaultPerPage,
	/** When false the query never runs, so a section the caller has hidden for want of a
	 * permission does not fire a request that is certain to 403 and toast. */
	enabled = true,
) {
	const [page, setPage] = useState(1);
	const slice: Page = { page, perPage };
	const query = useQuery({
		queryKey: queryKey(slice),
		queryFn: () => fetcher(slice),
		placeholderData: keepPreviousData,
		enabled,
	});

	return {
		page,
		perPage,
		setPage,
		items: query.data?.items ?? [],
		total: query.data?.total ?? 0,
		count: query.data?.count ?? 0,
		loading: query.isPending && enabled,
		/** True while a different page is in flight but the old one is still shown. */
		fetching: query.isFetching,
		error: query.error,
	};
}
