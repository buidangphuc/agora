// ⚠️ requires `make proto` ─────────────────────────────────────────────────────
// This inbound adapter implements the buf-GENERATED gRPC base class and uses the
// GENERATED proto message types from `com.platform.listing.v1` / `com.platform.common.v1`.
// Those types live under ./generated and DO NOT EXIST until you run `make proto`
// (buf generate against the vendored proto module — see proto-vendor/README.md).
// Until then this file will not compile — that is expected for the seed.
//
// After `make proto`, the generated Kotlin lives in packages derived from the proto
// `package` plus buf managed-mode's `com` java prefix (e.g. proto `platform.listing.v1`
// -> `com.platform.listing.v1`, proto `platform.common.v1` -> `com.platform.common.v1`).
// Verify the exact generated names and adjust the imports below if buf's naming
// differs in your toolchain version.
package infrastructure.grpc

import application.GetListingUseCase
import domain.model.AuthorizationException
import domain.model.Listing as DomainListing
import domain.model.ListingStatus
import domain.model.NotFoundException
import domain.port.GetListingPort
import io.grpc.Status
import io.grpc.StatusException

// ── Generated types (available only after `make proto`) ───────────────────────
import com.platform.listing.v1.GetListingRequest
import com.platform.listing.v1.GetListingResponse
import com.platform.listing.v1.ListListingsRequest
import com.platform.listing.v1.ListListingsResponse
import com.platform.listing.v1.ListingServiceGrpcKt.ListingServiceCoroutineImplBase
import com.platform.listing.v1.Listing as ProtoListing
import com.platform.listing.v1.ListingStatus as ProtoListingStatus
import com.platform.listing.v1.getListingResponse
import com.platform.listing.v1.listing as protoListing

/**
 * INBOUND adapter (driving side of the hexagon). It:
 *   1. reads the Principal the AuthInterceptor resolved into the gRPC Context,
 *   2. translates proto request → domain call,
 *   3. invokes the use-case through its driving port,
 *   4. translates the domain result → proto response,
 *   5. maps domain exceptions → gRPC status codes.
 *
 * No business logic lives here — only translation + error mapping.
 */
class ListingGrpcService(
    private val getListing: GetListingPort,
) : ListingServiceCoroutineImplBase() {

    override suspend fun getListing(request: GetListingRequest): GetListingResponse {
        val principal = AuthInterceptor.principalFrom()
        val listing = try {
            getListing.getListing(id = request.id, principal = principal)
        } catch (e: AuthorizationException) {
            throw e.toStatusException(Status.PERMISSION_DENIED)
        } catch (e: NotFoundException) {
            throw e.toStatusException(Status.NOT_FOUND)
        }
        return getListingResponse { this.listing = listing.toProto() }
    }

    override suspend fun listListings(request: ListListingsRequest): ListListingsResponse {
        // Seed leaves ListListings as a TODO: wire a ListListingsUseCase over the
        // ListingRepository.list() port and translate the page here, mirroring
        // getListing above. Fail loudly rather than returning a fake empty page.
        throw Status.UNIMPLEMENTED
            .withDescription("ListListings not implemented in seed — wire the use-case")
            .asRuntimeException()
    }

    // ── proto → domain / domain → proto translation ───────────────────────────

    private fun DomainListing.toProto(): ProtoListing = protoListing {
        id = this@toProto.id
        title = this@toProto.title
        description = this@toProto.description
        price = this@toProto.priceMinor
        currency = this@toProto.currency
        status = this@toProto.status.toProto()
    }

    private fun ListingStatus.toProto(): ProtoListingStatus = when (this) {
        ListingStatus.DRAFT -> ProtoListingStatus.LISTING_STATUS_DRAFT
        ListingStatus.PUBLISHED -> ProtoListingStatus.LISTING_STATUS_PUBLISHED
        ListingStatus.REJECTED -> ProtoListingStatus.LISTING_STATUS_REJECTED
        ListingStatus.UNSPECIFIED -> ProtoListingStatus.LISTING_STATUS_UNSPECIFIED
    }

    private fun domain.model.DomainException.toStatusException(status: Status): StatusException =
        status.withDescription("[$code] $message").asException()
}
