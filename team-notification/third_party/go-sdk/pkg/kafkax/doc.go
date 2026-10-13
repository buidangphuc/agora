// Package kafkax builds franz-go Kafka clients from config.Kafka. Producers write
// synchronously and stamp each record with the W3C traceparent of the caller's
// span and the request id from reqctx; both clients are instrumented with kotel.
package kafkax
