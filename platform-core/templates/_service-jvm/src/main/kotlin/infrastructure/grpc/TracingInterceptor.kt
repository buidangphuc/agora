package infrastructure.grpc

import io.grpc.Context
import io.grpc.Contexts
import io.grpc.Metadata
import io.grpc.ServerCall
import io.grpc.ServerCallHandler
import io.grpc.ServerInterceptor

/**
 * INBOUND cross-cutting adapter — observability seam (ADR-0004).
 *
 * Two jobs, both metadata-level:
 *   1. Bridge `x-request-id` — the human-facing correlation id from the existing
 *      product convention. Read it if present, else mint one, and stash it in the
 *      call Context so logs/spans can tag it.
 *   2. Propagate W3C `traceparent` — extract the incoming trace context so this
 *      service's spans join the caller's trace, and let it flow onward.
 *
 * SEED behaviour keeps only the metadata plumbing so the service builds without
 * a full OTel SDK wired in. TODO(ADR-0004): use the OpenTelemetry API to extract
 * the remote context (W3C `TextMapPropagator` over these metadata keys) and start
 * a SERVER span per call. When the `opentelemetry-grpc-1.6` instrumentation is on
 * the classpath, prefer its `GrpcTelemetry.newServerInterceptor()` and let this
 * interceptor own only the x-request-id bridge.
 */
class TracingInterceptor : ServerInterceptor {
    override fun <ReqT, RespT> interceptCall(
        call: ServerCall<ReqT, RespT>,
        headers: Metadata,
        next: ServerCallHandler<ReqT, RespT>,
    ): ServerCall.Listener<ReqT> {
        val requestId = headers.get(REQUEST_ID_KEY)?.takeIf { it.isNotBlank() }
            ?: generateRequestId()

        // traceparent is read here for propagation; the OTel propagator (TODO)
        // turns it into a parent SpanContext.
        val traceparent = headers.get(TRACEPARENT_KEY)

        var ctx = Context.current().withValue(REQUEST_ID_CTX, requestId)
        if (traceparent != null) {
            ctx = ctx.withValue(TRACEPARENT_CTX, traceparent)
        }
        return Contexts.interceptCall(ctx, call, headers, next)
    }

    private fun generateRequestId(): String = java.util.UUID.randomUUID().toString()

    companion object {
        val REQUEST_ID_KEY: Metadata.Key<String> =
            Metadata.Key.of("x-request-id", Metadata.ASCII_STRING_MARSHALLER)
        val TRACEPARENT_KEY: Metadata.Key<String> =
            Metadata.Key.of("traceparent", Metadata.ASCII_STRING_MARSHALLER)

        val REQUEST_ID_CTX: Context.Key<String> = Context.key("x-request-id")
        val TRACEPARENT_CTX: Context.Key<String> = Context.key("traceparent")

        fun requestIdFrom(context: Context = Context.current()): String? =
            REQUEST_ID_CTX.get(context)
    }
}
