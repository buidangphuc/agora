package domain.port

import domain.model.Listing
import domain.model.Principal

/**
 * DRIVING port (inbound). The application exposes its use-cases through ports
 * like this one; the gRPC adapter (infrastructure/grpc) depends on the port,
 * not on the concrete use-case class. This keeps the inbound adapter decoupled
 * from application wiring and makes the use-case trivially testable/mockable.
 */
interface GetListingPort {
    /**
     * @param principal resolved identity (from the auth interceptor); the
     *   use-case enforces the required scope and throws AuthorizationException
     *   when it is missing.
     * @throws domain.model.AuthorizationException on insufficient scope.
     * @throws domain.model.NotFoundException when no listing has this id.
     */
    suspend fun getListing(id: String, principal: Principal): Listing
}
