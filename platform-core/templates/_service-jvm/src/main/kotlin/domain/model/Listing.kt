package domain.model

/**
 * Pure domain model. NO framework imports (no gRPC, no proto, no JDBC).
 *
 * This is the seed shape mirroring `platform.listing.v1.Listing`, but it is a
 * DELIBERATELY SEPARATE type: the proto message is a transport DTO; this is the
 * business object team-domain owns and extends. The gRPC adapter translates
 * between the two (infrastructure/grpc/ListingGrpcService.kt) so the wire
 * contract can evolve independently of the domain.
 *
 * Money is an integer count of minor units (e.g. VND) + an ISO-4217 currency
 * code — never a float, to avoid rounding drift.
 */
data class Listing(
    val id: String,
    val title: String,
    val description: String,
    val priceMinor: Long,
    val currency: String,
    val status: ListingStatus,
)

/** Lifecycle of a listing. `UNSPECIFIED` guards against unset proto enums. */
enum class ListingStatus {
    UNSPECIFIED,
    DRAFT,
    PUBLISHED,
    REJECTED,
}
