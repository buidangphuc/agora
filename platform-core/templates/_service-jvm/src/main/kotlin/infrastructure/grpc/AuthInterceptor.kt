package infrastructure.grpc

import domain.model.Principal
import domain.model.PrincipalType
import io.grpc.Context
import io.grpc.Contexts
import io.grpc.Metadata
import io.grpc.ServerCall
import io.grpc.ServerCallHandler
import io.grpc.ServerInterceptor

/**
 * INBOUND cross-cutting adapter — resolves a credential into a domain Principal
 * and attaches it to the gRPC Context (ADR-0003). Services/use-cases never parse
 * credentials themselves; they read the resolved Principal via [principalFrom].
 *
 * SEED behaviour: trusts a static bearer token and grants a fixed scope set.
 * TODO(ADR-0003): replace with real verification — JWT validation for native
 * app tokens, and dual-auth at the edge (cookie/session for web SSR). The edge
 * Gateway (Phase 2) will resolve web sessions → Principal; this interceptor
 * stays mechanism-agnostic and only maps credential → Principal.
 */
class AuthInterceptor : ServerInterceptor {
    override fun <ReqT, RespT> interceptCall(
        call: ServerCall<ReqT, RespT>,
        headers: Metadata,
        next: ServerCallHandler<ReqT, RespT>,
    ): ServerCall.Listener<ReqT> {
        val principal = resolvePrincipal(headers)
        val ctx = Context.current().withValue(PRINCIPAL_KEY, principal)
        return Contexts.interceptCall(ctx, call, headers, next)
    }

    private fun resolvePrincipal(headers: Metadata): Principal {
        val authorization = headers.get(AUTHORIZATION_KEY)?.trim()
        val bearer = authorization
            ?.takeIf { it.regionMatches(0, BEARER_PREFIX, 0, BEARER_PREFIX.length, ignoreCase = true) }
            ?.substring(BEARER_PREFIX.length)
            ?.trim()

        // TODO(ADR-0003): verify `bearer` (JWT signature / introspection) and
        // derive real id + scopes. The seed grants a fixed identity to any
        // non-blank token and treats everything else as anonymous.
        return if (!bearer.isNullOrBlank()) {
            Principal(
                id = "seed-user",
                type = PrincipalType.USER,
                scopes = SEED_SCOPES,
            )
        } else {
            Principal.ANONYMOUS
        }
    }

    companion object {
        private const val BEARER_PREFIX = "Bearer "

        /** gRPC metadata key for the bearer credential (lowercase, ASCII). */
        val AUTHORIZATION_KEY: Metadata.Key<String> =
            Metadata.Key.of("authorization", Metadata.ASCII_STRING_MARSHALLER)

        /** Context key holding the resolved Principal for the duration of a call. */
        val PRINCIPAL_KEY: Context.Key<Principal> = Context.key("principal")

        /** Scopes granted to a valid seed token. Tighten/derive from JWT later. */
        private val SEED_SCOPES = setOf("listing:read", "listing:write")

        /** Read the resolved Principal inside a service method; anonymous default. */
        fun principalFrom(context: Context = Context.current()): Principal =
            PRINCIPAL_KEY.get(context) ?: Principal.ANONYMOUS
    }
}
