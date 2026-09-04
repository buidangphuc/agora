package domain.port

import domain.model.Listing

/**
 * DRIVEN port (outbound). The domain declares WHAT it needs from persistence;
 * infrastructure/db supplies HOW (InMemoryListingRepository now, a Postgres
 * adapter later). The interface lives in the domain so the dependency arrow
 * points inward — infrastructure depends on the domain, never the reverse.
 *
 * `suspend` because the inbound gRPC path is coroutine-based (grpc-kotlin); a
 * JDBC adapter runs its blocking calls on Dispatchers.IO.
 */
interface ListingRepository {
    /** Returns the listing, or null when absent (use-case decides how to signal). */
    suspend fun findById(id: String): Listing?

    /**
     * Cursor-paginated page of listings.
     *
     * @param cursor opaque cursor from a previous page; null/empty = first page.
     * @param pageSize server-clamped page size; 0 = server default.
     * @return the page plus the next cursor (null when no further pages).
     */
    suspend fun list(cursor: String?, pageSize: Int): ListingPage
}

/** Result of a paged read — mirrors the shape of PageResponse without the proto. */
data class ListingPage(
    val listings: List<Listing>,
    val nextCursor: String?,
    val total: Long,
)
