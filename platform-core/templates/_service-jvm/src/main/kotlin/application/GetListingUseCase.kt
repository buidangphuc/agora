package application

import domain.model.AuthorizationException
import domain.model.Listing
import domain.model.NotFoundException
import domain.model.Principal
import domain.port.GetListingPort
import domain.port.ListingRepository

/**
 * Application use-case: orchestrates the domain via ports. It knows nothing
 * about gRPC, proto, or JDBC — it takes a domain Principal + id, enforces
 * authorization, calls the repository port, and returns a domain Listing.
 *
 * Implements the driving port so the inbound adapter depends on the interface.
 */
class GetListingUseCase(
    private val repository: ListingRepository,
) : GetListingPort {
    override suspend fun getListing(id: String, principal: Principal): Listing {
        // ── Authorization hook (ADR-0003) ────────────────────────────────────
        // Coarse scope check: caller must hold `listing:read`. On failure throw
        // a DOMAIN exception; the gRPC adapter maps it to PERMISSION_DENIED with
        // Error.code = "insufficient_scope". Swap for a richer policy later.
        requireScope(principal, REQUIRED_SCOPE)

        return repository.findById(id)
            ?: throw NotFoundException(entity = "listing", id = id)
    }

    private fun requireScope(principal: Principal, scope: String) {
        if (!principal.hasScope(scope)) {
            throw AuthorizationException(requiredScope = scope)
        }
    }

    companion object {
        const val REQUIRED_SCOPE = "listing:read"
    }
}
