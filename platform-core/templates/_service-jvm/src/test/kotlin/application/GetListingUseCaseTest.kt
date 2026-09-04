package application

import domain.model.AuthorizationException
import domain.model.Listing
import domain.model.ListingStatus
import domain.model.NotFoundException
import domain.model.Principal
import domain.model.PrincipalType
import domain.port.ListingPage
import domain.port.ListingRepository
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * Pure use-case test — no gRPC, no DB, no generated proto. Compiles and runs on
 * `make check` BEFORE `make proto`, which is the whole point of keeping the
 * domain/application layers framework-free: a fake port is all you need.
 */
class GetListingUseCaseTest {
    private val listing = Listing(
        id = "lst_1",
        title = "t",
        description = "d",
        priceMinor = 100L,
        currency = "VND",
        status = ListingStatus.PUBLISHED,
    )

    private fun repo(found: Listing? = listing) = object : ListingRepository {
        override suspend fun findById(id: String): Listing? = found
        override suspend fun list(cursor: String?, pageSize: Int): ListingPage =
            ListingPage(emptyList(), null, 0)
    }

    private fun principal(vararg scopes: String) =
        Principal(id = "u", type = PrincipalType.USER, scopes = scopes.toSet())

    @Test
    fun `returns listing when scope present and found`() = runTest {
        val useCase = GetListingUseCase(repo())
        val result = useCase.getListing("lst_1", principal("listing:read"))
        assertEquals(listing, result)
    }

    @Test
    fun `throws AuthorizationException when scope missing`() = runTest {
        val useCase = GetListingUseCase(repo())
        assertFailsWith<AuthorizationException> {
            useCase.getListing("lst_1", principal())
        }
    }

    @Test
    fun `throws NotFoundException when listing absent`() = runTest {
        val useCase = GetListingUseCase(repo(found = null))
        assertFailsWith<NotFoundException> {
            useCase.getListing("missing", principal("listing:read"))
        }
    }
}
