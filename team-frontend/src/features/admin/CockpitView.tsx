"use client";

import React, { useEffect, useState } from "react";

// Every figure on this page comes from the gateway (Prometheus via
// GET /api/admin/metrics, or the ops:orders SSE room). When a source has no
// data the widget says so — it never falls back to a placeholder number.

const GATEWAY_URL =
  process.env.NEXT_PUBLIC_GATEWAY_URL?.trim() || "http://localhost:8080";

const JAEGER_URL = "http://localhost:16686";
const KAFKA_UI_URL = "http://localhost:8088";
const GRAFANA_URL = "http://localhost:3001";

const NO_DATA = "—";

export interface ServiceHealth {
  name: string;
  port: number;
  status: string;
  rps: number;
  p95_latency_ms: number | null;
  p99_latency_ms: number | null;
  error_rate: number | null;
}

export interface TraceSummary {
  trace_id: string;
  operation: string;
  duration: string;
  status: string;
  jaeger_url: string;
}

export interface CockpitData {
  timestamp: string;
  prometheus_available?: boolean;
  total_rps: number;
  avg_latency_ms: number | null;
  total_orders_24h: number | null;
  total_revenue_24h: number | null;
  services: ServiceHealth[] | null;
  recent_traces: TraceSummary[] | null;
}

interface LiveOrder {
  id: string;
  user: string | null;
  amount: number | null;
  time: string;
}

function num(v: number | null | undefined, digits: number, unit = ""): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  return `${v.toFixed(digits)}${unit}`;
}

function count(v: number | null | undefined): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  return v.toLocaleString();
}

const STATUS_STYLE: Record<string, string> = {
  HEALTHY: "bg-emerald-950 text-emerald-400 border-emerald-800",
  DEGRADED: "bg-red-950 text-red-400 border-red-800",
  IDLE: "bg-slate-800 text-slate-300 border-slate-600",
  NO_DATA: "bg-slate-900 text-slate-500 border-slate-700",
  UNKNOWN: "bg-slate-900 text-slate-500 border-slate-700",
};

export function CockpitView() {
  const [data, setData] = useState<CockpitData | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [liveOrders, setLiveOrders] = useState<LiveOrder[]>([]);

  useEffect(() => {
    let cancelled = false;
    const fetchMetrics = async () => {
      try {
        const res = await fetch(`${GATEWAY_URL}/api/admin/metrics`);
        if (!res.ok) throw new Error(`status ${res.status}`);
        const json = (await res.json()) as CockpitData;
        if (cancelled) return;
        setData(json);
        setFetchFailed(false);
      } catch {
        if (cancelled) return;
        setData(null);
        setFetchFailed(true);
      } finally {
        if (!cancelled) setLoaded(true);
      }
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 3000);

    // Live orders: ops:orders SSE room, fed from the order.events topic. Rows
    // are shown only for real OrderPlaced events — no placeholders.
    let evtSource: EventSource | null = null;
    try {
      evtSource = new EventSource(
        `${GATEWAY_URL}/api/events/live?room=ops:orders`,
      );
      evtSource.onmessage = (e) => {
        try {
          const parsed = JSON.parse(e.data);
          const d = parsed?.data;
          if (parsed?.event !== "OrderPlaced" || !d?.order_id) return;
          setLiveOrders((prev) => [
            {
              id: String(d.order_id),
              user: typeof d.buyer === "string" ? d.buyer : null,
              amount: typeof d.amount === "number" ? d.amount : null,
              time: new Date().toLocaleTimeString(),
            },
            ...prev.slice(0, 7),
          ]);
        } catch {}
      };
    } catch {}

    return () => {
      cancelled = true;
      clearInterval(interval);
      if (evtSource) evtSource.close();
    };
  }, []);

  const promUp = data?.prometheus_available !== false && data !== null;
  const services = data?.services ?? [];
  const traces = data?.recent_traces ?? [];

  const banner = fetchFailed
    ? "Không kết nối được gateway — chưa có số liệu."
    : loaded && data && data.prometheus_available === false
      ? "Prometheus không khả dụng — chưa có số liệu telemetry."
      : null;

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 p-6 font-sans">
      {/* Top Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between pb-6 border-b border-slate-800 gap-4">
        <div>
          <div className="flex items-center gap-3">
            <span
              className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold border ${
                promUp
                  ? "bg-emerald-950 text-emerald-400 border-emerald-800"
                  : "bg-slate-900 text-slate-400 border-slate-700"
              }`}
            >
              {promUp
                ? "● Prometheus kết nối"
                : loaded
                  ? "○ Chưa có số liệu"
                  : "○ Đang tải"}
            </span>
            <h1 className="text-2xl font-bold tracking-tight text-white flex items-center gap-2">
              🛰️ Platform Operations HUD & SRE Cockpit
            </h1>
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Telemetry thật từ Prometheus (RPS, Latency, Error Rate theo gRPC
            service) & liên kết Distributed Tracing
          </p>
        </div>

        <div className="flex items-center gap-3">
          <a
            href={JAEGER_URL}
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-blue-900/40 hover:bg-blue-800/60 border border-blue-700 text-blue-300 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5"
          >
            🕵️ Open Jaeger (:16686)
          </a>
          <a
            href={KAFKA_UI_URL}
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-red-900/40 hover:bg-red-800/60 border border-red-700 text-red-300 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5"
          >
            🎛️ Open Kafka UI (:8088)
          </a>
          <a
            href={GRAFANA_URL}
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-orange-900/40 hover:bg-orange-800/60 border border-orange-700 text-orange-300 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5"
          >
            📈 Grafana (:3001)
          </a>
        </div>
      </div>

      {banner && (
        <output className="mt-4 block rounded-lg border border-amber-800 bg-amber-950/60 px-4 py-2 text-xs text-amber-300">
          {banner}
        </output>
      )}

      {/* Global Stat Cards */}
      <div
        data-testid="cockpit_metrics"
        className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mt-6"
      >
        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            Total Gateway Throughput
          </div>
          <div className="text-2xl font-black text-emerald-400 mt-1 flex items-baseline gap-2">
            {promUp ? num(data?.total_rps, 1) : NO_DATA}{" "}
            <span className="text-xs font-normal text-slate-500">req/sec</span>
          </div>
          <div className="text-xs text-slate-500 mt-1">
            Tổng gRPC client rate của gateway (cửa sổ 1 phút)
          </div>
        </div>

        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            Average gRPC P95 Latency
          </div>
          <div className="text-2xl font-black text-cyan-400 mt-1 flex items-baseline gap-2">
            {promUp ? num(data?.avg_latency_ms, 1) : NO_DATA}{" "}
            <span className="text-xs font-normal text-slate-500">ms</span>
          </div>
          <div className="text-xs text-slate-500 mt-1">
            Trung bình P95 các service đang có traffic (5 phút)
          </div>
        </div>

        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            24h Completed Orders
          </div>
          <div className="text-2xl font-black text-amber-400 mt-1 flex items-baseline gap-2">
            {count(data?.total_orders_24h)}{" "}
            <span className="text-xs font-normal text-slate-500">đơn</span>
          </div>
          {typeof data?.total_orders_24h !== "number" && (
            <div className="text-xs text-slate-500 mt-1">
              Chưa có dữ liệu (chưa có nguồn số liệu đơn hàng)
            </div>
          )}
        </div>

        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            24h Gross Revenue (GMV)
          </div>
          <div className="text-2xl font-black text-purple-400 mt-1 flex items-baseline gap-2">
            {count(data?.total_revenue_24h)}{" "}
            <span className="text-xs font-normal text-slate-500">₫</span>
          </div>
          {typeof data?.total_revenue_24h !== "number" && (
            <div className="text-xs text-slate-500 mt-1">
              Chưa có dữ liệu (chưa có nguồn doanh thu)
            </div>
          )}
        </div>
      </div>

      {/* Main Grid: Services Health Radar + Live Order Ticker */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mt-6">
        <div
          data-testid="cockpit_health"
          className="lg:col-span-2 bg-slate-900/90 border border-slate-800 p-5 rounded-xl"
        >
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
              🛡️ Microservices Health & RED Metrics Radar
            </h2>
            <span className="text-xs text-slate-500">
              {services.length > 0
                ? `${services.length} service theo dõi`
                : "Chưa có dữ liệu"}
            </span>
          </div>

          {services.length === 0 ? (
            <p className="py-6 text-center text-xs text-slate-500">
              Chưa có dữ liệu service.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="border-b border-slate-800 text-slate-400 font-medium">
                  <tr>
                    <th className="py-2.5 px-3">Service Name</th>
                    <th className="py-2.5 px-3">Port</th>
                    <th className="py-2.5 px-3">Status</th>
                    <th className="py-2.5 px-3 text-right">RPS</th>
                    <th className="py-2.5 px-3 text-right">P95 Latency</th>
                    <th className="py-2.5 px-3 text-right">P99 Latency</th>
                    <th className="py-2.5 px-3 text-right">Error Rate</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60 font-mono">
                  {services.map((s) => {
                    const hasStatus = s.status !== "UNKNOWN" && promUp;
                    return (
                      <tr
                        key={s.name}
                        className="hover:bg-slate-800/40 transition-colors"
                      >
                        <td className="py-2.5 px-3 font-semibold text-slate-200">
                          {s.name}
                        </td>
                        <td className="py-2.5 px-3 text-slate-400">
                          :{s.port}
                        </td>
                        <td className="py-2.5 px-3">
                          <span
                            className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-bold border ${STATUS_STYLE[s.status] ?? STATUS_STYLE.UNKNOWN}`}
                          >
                            {s.status}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 text-right text-emerald-400">
                          {hasStatus && s.status !== "NO_DATA"
                            ? num(s.rps, 1)
                            : NO_DATA}
                        </td>
                        <td className="py-2.5 px-3 text-right text-cyan-400">
                          {num(s.p95_latency_ms, 1, " ms")}
                        </td>
                        <td className="py-2.5 px-3 text-right text-amber-400">
                          {num(s.p99_latency_ms, 1, " ms")}
                        </td>
                        <td className="py-2.5 px-3 text-right text-slate-400">
                          {typeof s.error_rate === "number"
                            ? `${(s.error_rate * 100).toFixed(2)}%`
                            : NO_DATA}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
          <p className="mt-3 text-xs text-slate-500">
            Nguồn: gRPC client metrics của gateway. Error rate tính mọi mã gRPC
            khác OK (kể cả từ chối nghiệp vụ như hết hàng).
          </p>
        </div>

        {/* Live Order Ticker */}
        <div className="bg-slate-900/90 border border-slate-800 p-5 rounded-xl flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
                ⚡ Live Orders Stream (SSE)
              </h2>
              <span className="text-xs bg-slate-800 text-slate-300 border border-slate-600 px-2 py-0.5 rounded font-bold">
                ops:orders
              </span>
            </div>

            {liveOrders.length === 0 ? (
              <p className="py-6 text-center text-xs text-slate-500">
                Chưa có đơn hàng nào được ghi nhận.
              </p>
            ) : (
              <div className="space-y-2.5 max-h-80 overflow-y-auto pr-1">
                {liveOrders.map((o) => (
                  <div
                    key={o.id}
                    className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-xs hover:border-slate-700 transition-all"
                  >
                    <div>
                      <div className="font-semibold text-slate-200 flex items-center gap-1.5">
                        <span className="text-emerald-400 font-bold">
                          🛒 {o.id}
                        </span>
                        <span className="text-xs text-slate-500 font-mono">
                          ({o.time})
                        </span>
                      </div>
                      {o.user && (
                        <div className="text-xs text-slate-400">{o.user}</div>
                      )}
                    </div>
                    <div className="text-right font-mono font-bold text-amber-400">
                      {typeof o.amount === "number"
                        ? `${o.amount.toLocaleString()} ₫`
                        : NO_DATA}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="mt-4 pt-3 border-t border-slate-800 text-xs text-slate-500 text-center">
            Kafka Topic:{" "}
            <code className="text-slate-400 font-mono">order.events</code> →
            Edge SSE
          </div>
        </div>
      </div>

      {/* Tracing: real Jaeger deep-links only */}
      <div className="bg-slate-900/90 border border-slate-800 p-5 rounded-xl mt-6">
        <h2 className="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
          🕵️ Distributed Traces Inspection (W3C TraceContext)
        </h2>
        <p className="text-xs text-slate-400 mt-0.5">
          Mở Jaeger để xem span latency xuyên Gateway → gRPC Services → Kafka.
        </p>

        {traces.length > 0 && (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-4">
            {traces.map((t) => (
              <a
                key={t.trace_id}
                href={t.jaeger_url}
                target="_blank"
                rel="noreferrer"
                className="p-3.5 rounded-lg bg-slate-950 border border-slate-800 text-xs text-slate-300 hover:border-slate-600"
              >
                <div className="font-mono text-cyan-400 truncate">
                  {t.trace_id}
                </div>
                <div className="mt-1">{t.operation}</div>
                <div className="text-slate-500 font-mono">{t.duration}</div>
              </a>
            ))}
          </div>
        )}

        <div className="mt-4 flex flex-wrap gap-3">
          <a
            href={`${JAEGER_URL}/search?service=team-gateway`}
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-blue-600/30 hover:bg-blue-600/50 border border-blue-500/50 text-blue-300 text-xs font-semibold rounded transition-all"
          >
            🔍 Trace của team-gateway trên Jaeger ↗
          </a>
        </div>
        {traces.length === 0 && (
          <p className="mt-3 text-xs text-slate-500">
            Chưa có danh sách trace gần đây (chưa có nguồn) — dùng liên kết
            Jaeger ở trên.
          </p>
        )}
      </div>
    </div>
  );
}
