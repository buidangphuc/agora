package domain.model

/**
 * Transport-neutral identity — the domain's own copy of
 * `platform.common.v1.Principal` (ADR-0003). The auth interceptor resolves a
 * credential (bearer today) into this shape and hands it to use-cases; the
 * domain never sees tokens, headers, or gRPC metadata.
 *
 * Pure Kotlin: no gRPC/proto import so the domain stays framework-free.
 */
data class Principal(
    val id: String,
    val type: PrincipalType,
    val scopes: Set<String>,
) {
    fun hasScope(scope: String): Boolean = scope in scopes

    companion object {
        /** Fallback identity when no/failed credential was presented. */
        val ANONYMOUS = Principal(id = "", type = PrincipalType.ANONYMOUS, scopes = emptySet())
    }
}

enum class PrincipalType {
    UNSPECIFIED,
    ANONYMOUS,
    USER,
    SERVICE,
}
