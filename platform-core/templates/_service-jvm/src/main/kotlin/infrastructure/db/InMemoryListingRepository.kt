package infrastructure.db

import domain.model.Listing
import domain.model.ListingStatus
import domain.port.ListingPage
import domain.port.ListingRepository

/**
 * OUTBOUND adapter — a stub implementation of the ListingRepository port so the
 * service runs end-to-end before a database exists. Seeded with a couple of
 * fixtures; pagination is naive (single page).
 *
 * ── Where the real DB plugs in ────────────────────────────────────────────────
 * Replace this with a `PostgresListingRepository` implementing the same port:
 *   - Flyway runs migrations from src/main/resources/db/migration at startup
 *     (or via `make migrate`) against DATABASE_URL — see .env.example.
 *   - Use a JDBC/Hikari pool (deps commented in build.gradle.kts); wrap blocking
 *     calls in `withContext(Dispatchers.IO)`.
 *   - DB-per-service: this service owns `listing_db` and touches NO other
 *     service's tables. Cross-service reads go over gRPC, never shared SQL.
 * Because the port is the seam, swapping this for Postgres does not touch the
 * domain or the use-case — only Server.kt's wiring line changes.
 */
class InMemoryListingRepository : ListingRepository {
    private val store: Map<String, Listing> =
        listOf(
            Listing(
                id = "lst_0001",
                title = "Seed listing — 2BR apartment",
                description = "Placeholder fixture. Replace with a real repository.",
                priceMinor = 3_500_000_000L, // 3.5B VND, minor units
                currency = "VND",
                status = ListingStatus.PUBLISHED,
            ),
            Listing(
                id = "lst_0002",
                title = "Seed listing — office space",
                description = "Second fixture for ListListings paging.",
                priceMinor = 1_200_000_000L,
                currency = "VND",
                status = ListingStatus.DRAFT,
            ),
        ).associateBy { it.id }

    override suspend fun findById(id: String): Listing? = store[id]

    override suspend fun list(cursor: String?, pageSize: Int): ListingPage {
        // Naive stub: returns everything on the first page, no further pages.
        val items = store.values.toList()
        return ListingPage(listings = items, nextCursor = null, total = items.size.toLong())
    }
}
