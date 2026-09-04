package domain.model

/**
 * Domain-level errors. These are framework-free: the gRPC adapter maps them to
 * transport status codes + a `platform.common.v1.Error` envelope. Keeping them
 * here means use-cases signal intent ("not allowed", "not found") without
 * knowing anything about gRPC.
 */
sealed class DomainException(message: String) : RuntimeException(message) {
    /** Stable, machine-readable code carried in the Error envelope. */
    abstract val code: String
}

/**
 * Missing required scope. Adapter maps → gRPC PERMISSION_DENIED with
 * Error.code = "insufficient_scope" (ADR-0003).
 */
class AuthorizationException(
    val requiredScope: String,
) : DomainException("missing required scope: $requiredScope") {
    override val code: String = "insufficient_scope"
}

/** Requested entity does not exist. Adapter maps → gRPC NOT_FOUND. */
class NotFoundException(
    val entity: String,
    val id: String,
) : DomainException("$entity not found: $id") {
    override val code: String = "not_found"
}
