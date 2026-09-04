// Composition root + process entry point.
//
// This is where the hexagon is WIRED: concrete adapters are constructed and
// injected into use-cases, and the gRPC server is assembled with its
// interceptors. It is the only place that knows about every layer at once.
//
// ⚠️ The line that registers ListingGrpcService depends on GENERATED proto types
// (see infrastructure/grpc/ListingGrpcService.kt). It compiles only after
// `make proto`. Everything else here is proto-independent.

import application.GetListingUseCase
import infrastructure.db.InMemoryListingRepository
import infrastructure.grpc.AuthInterceptor
import infrastructure.grpc.ListingGrpcService
import infrastructure.grpc.TracingInterceptor
import io.grpc.Server
import io.grpc.ServerBuilder
import io.grpc.protobuf.services.HealthStatusManager
import io.grpc.protobuf.services.ProtoReflectionService

private const val DEFAULT_GRPC_PORT = 50053

fun main() {
    val port = System.getenv("GRPC_PORT")?.toIntOrNull() ?: DEFAULT_GRPC_PORT

    // ── Wire the hexagon (outbound adapter → use-case → inbound adapter) ──────
    // Swap InMemoryListingRepository for a PostgresListingRepository here when
    // the DB adapter lands — nothing else changes.
    val listingRepository = InMemoryListingRepository()
    val getListingUseCase = GetListingUseCase(repository = listingRepository)
    val listingGrpcService = ListingGrpcService(getListing = getListingUseCase)

    // Health + reflection (grpc-services). Health reports SERVING for the whole
    // server; reflection lets grpcurl/clients discover methods without protos.
    val health = HealthStatusManager()

    val server: Server = ServerBuilder
        .forPort(port)
        // Interceptor order: tracing OUTERMOST (starts the span / request-id),
        // then auth (resolves Principal into the Context) before the handler.
        .intercept(AuthInterceptor())
        .intercept(TracingInterceptor())
        .addService(listingGrpcService)
        .addService(health.healthService)
        .addService(ProtoReflectionService.newInstance())
        .build()

    server.start()
    // Mark serving AFTER a successful start so probes only pass once we're up.
    health.setStatus("", io.grpc.health.v1.HealthCheckResponse.ServingStatus.SERVING)
    println("team-service gRPC listening on :$port")

    // ── Graceful shutdown ─────────────────────────────────────────────────────
    Runtime.getRuntime().addShutdownHook(
        Thread {
            println("shutting down team-service …")
            health.setStatus("", io.grpc.health.v1.HealthCheckResponse.ServingStatus.NOT_SERVING)
            server.shutdown()
            server.awaitTermination(SHUTDOWN_GRACE_SECONDS, java.util.concurrent.TimeUnit.SECONDS)
            println("team-service stopped")
        },
    )

    server.awaitTermination()
}

private const val SHUTDOWN_GRACE_SECONDS = 30L
