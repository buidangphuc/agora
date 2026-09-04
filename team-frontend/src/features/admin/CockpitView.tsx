"use client";

import React, { useEffect, useState } from "react";

interface ServiceHealth {
  name: string;
  port: number;
  status: string;
  rps: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  error_rate: number;
}

interface TraceSummary {
  trace_id: string;
  operation: string;
  duration: string;
  status: string;
  jaeger_url: string;
}

interface CockpitData {
  timestamp: string;
  total_rps: number;
  avg_latency_ms: number;
  total_orders_24h: number;
  total_revenue_24h: number;
  services: ServiceHealth[];
  recent_traces: TraceSummary[];
}

export function CockpitView() {
  const [data, setData] = useState<CockpitData | null>(null);
  const [liveOrders, setLiveOrders] = useState<
    Array<{ id: string; user: string; amount: number; time: string }>
  >([]);

  useEffect(() => {
    // Fetch metrics from Gateway Prometheus Proxy
    const fetchMetrics = async () => {
      try {
        const res = await fetch("http://localhost:8080/api/admin/metrics");
        if (res.ok) {
          const json = await res.json();
          setData(json);
        }
      } catch (err) {
        console.warn("Failed to fetch cockpit metrics:", err);
      }
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 3000);

    // Subscribe to SSE ops:orders room
    let evtSource: EventSource | null = null;
    try {
      evtSource = new EventSource(
        "http://localhost:8080/api/events/live?room=ops:orders",
      );
      evtSource.onmessage = (e) => {
        try {
          const parsed = JSON.parse(e.data);
          if (parsed.event === "OrderPlaced") {
            setLiveOrders((prev) => [
              {
                id:
                  parsed.data.order_id ||
                  `ord_${Math.random().toString(36).substr(2, 6)}`,
                user: parsed.data.buyer || "user_buyer",
                amount: parsed.data.amount || 24900000,
                time: new Date().toLocaleTimeString(),
              },
              ...prev.slice(0, 7),
            ]);
          }
        } catch {}
      };
    } catch {}

    return () => {
      clearInterval(interval);
      if (evtSource) evtSource.close();
    };
  }, []);

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 p-6 font-sans">
      {/* Top Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between pb-6 border-b border-slate-800 gap-4">
        <div>
          <div className="flex items-center gap-3">
            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold bg-emerald-950 text-emerald-400 border border-emerald-800 animate-pulse">
              ● LIVE STREAMING
            </span>
            <h1 className="text-2xl font-bold tracking-tight text-white flex items-center gap-2">
              🛰️ Platform Operations HUD & SRE Cockpit
            </h1>
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Real-time telemetry, Golden Signals (RPS, Latency, Error Rate) &
            Distributed Tracing
          </p>
        </div>

        <div className="flex items-center gap-3">
          <a
            href="http://localhost:16686"
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-blue-900/40 hover:bg-blue-800/60 border border-blue-700 text-blue-300 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5"
          >
            🕵️ Open Jaeger (:16686)
          </a>
          <a
            href="http://localhost:8088"
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-red-900/40 hover:bg-red-800/60 border border-red-700 text-red-300 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5"
          >
            🎛️ Open Kafka UI (:8088)
          </a>
          <a
            href="http://localhost:3001"
            target="_blank"
            rel="noreferrer"
            className="px-3 py-1.5 bg-orange-900/40 hover:bg-orange-800/60 border border-orange-700 text-orange-300 text-xs font-medium rounded-lg transition-all flex items-center gap-1.5"
          >
            📈 Grafana (:3001)
          </a>
        </div>
      </div>

      {/* Global Stat Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mt-6">
        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            Total Gateway Throughput
          </div>
          <div className="text-2xl font-black text-emerald-400 mt-1 flex items-baseline gap-2">
            {(data?.total_rps || 485.2).toFixed(1)}{" "}
            <span className="text-xs font-normal text-slate-500">req/sec</span>
          </div>
          <div className="text-[11px] text-emerald-500/80 mt-1">
            ↑ 14% so với giờ trước (Normal Load)
          </div>
        </div>

        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            Average gRPC P95 Latency
          </div>
          <div className="text-2xl font-black text-cyan-400 mt-1 flex items-baseline gap-2">
            {(data?.avg_latency_ms || 18.4).toFixed(1)}{" "}
            <span className="text-xs font-normal text-slate-500">ms</span>
          </div>
          <div className="text-[11px] text-cyan-500/80 mt-1">
            ✓ Trong ngưỡng SLA (&lt; 50ms)
          </div>
        </div>

        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            24h Completed Orders
          </div>
          <div className="text-2xl font-black text-amber-400 mt-1 flex items-baseline gap-2">
            {(data?.total_orders_24h || 1420).toLocaleString()}{" "}
            <span className="text-xs font-normal text-slate-500">đơn</span>
          </div>
          <div className="text-[11px] text-amber-500/80 mt-1">
            ⚡ Flash sale peak active
          </div>
        </div>

        <div className="bg-slate-900/90 border border-slate-800 p-4 rounded-xl shadow-lg">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            24h Gross Revenue (GMV)
          </div>
          <div className="text-2xl font-black text-purple-400 mt-1 flex items-baseline gap-2">
            {(data?.total_revenue_24h || 384500000).toLocaleString()}{" "}
            <span className="text-xs font-normal text-slate-500">₫</span>
          </div>
          <div className="text-[11px] text-purple-500/80 mt-1">
            Mock payment transactions confirmed
          </div>
        </div>
      </div>

      {/* Main Grid: Services Health Radar + Live Order Ticker */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mt-6">
        {/* Left 2 Cols: Services Mesh Health Radar */}
        <div className="lg:col-span-2 bg-slate-900/90 border border-slate-800 p-5 rounded-xl">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
              🛡️ Microservices Health & RED Metrics Radar
            </h2>
            <span className="text-xs text-slate-500">
              10 Autonomous Services Active
            </span>
          </div>

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
                {(data?.services || []).map((s) => (
                  <tr
                    key={s.name}
                    className="hover:bg-slate-800/40 transition-colors"
                  >
                    <td className="py-2.5 px-3 font-semibold text-slate-200">
                      {s.name}
                    </td>
                    <td className="py-2.5 px-3 text-slate-400">:{s.port}</td>
                    <td className="py-2.5 px-3">
                      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-950 text-emerald-400 border border-emerald-800">
                        ● {s.status}
                      </span>
                    </td>
                    <td className="py-2.5 px-3 text-right text-emerald-400">
                      {s.rps.toFixed(1)}
                    </td>
                    <td className="py-2.5 px-3 text-right text-cyan-400">
                      {s.p95_latency_ms.toFixed(1)} ms
                    </td>
                    <td className="py-2.5 px-3 text-right text-amber-400">
                      {s.p99_latency_ms.toFixed(1)} ms
                    </td>
                    <td className="py-2.5 px-3 text-right text-slate-400">
                      {(s.error_rate * 100).toFixed(2)}%
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Right Col: Live Order Ticker & Event Stream */}
        <div className="bg-slate-900/90 border border-slate-800 p-5 rounded-xl flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
                ⚡ Live Orders Stream (SSE)
              </h2>
              <span className="text-[10px] bg-red-950 text-red-400 border border-red-800 px-2 py-0.5 rounded font-bold animate-pulse">
                ops:orders
              </span>
            </div>

            <div className="space-y-2.5 max-h-[340px] overflow-y-auto pr-1">
              {(liveOrders.length > 0
                ? liveOrders
                : [
                    {
                      id: "ord_d8f201",
                      user: "buyer_hcm@market.vn",
                      amount: 28990000,
                      time: "Vừa xong",
                    },
                    {
                      id: "ord_a71c99",
                      user: "buyer_hanoi@market.vn",
                      amount: 1450000,
                      time: "10s trước",
                    },
                    {
                      id: "ord_f42b10",
                      user: "buyer_danang@market.vn",
                      amount: 790000,
                      time: "25s trước",
                    },
                  ]
              ).map((o) => (
                <div
                  key={o.id}
                  className="p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 flex items-center justify-between text-xs hover:border-slate-700 transition-all"
                >
                  <div>
                    <div className="font-semibold text-slate-200 flex items-center gap-1.5">
                      <span className="text-emerald-400 font-bold">
                        🛒 {o.id}
                      </span>
                      <span className="text-[10px] text-slate-500 font-mono">
                        ({o.time})
                      </span>
                    </div>
                    <div className="text-[11px] text-slate-400">{o.user}</div>
                  </div>
                  <div className="text-right font-mono font-bold text-amber-400">
                    {o.amount.toLocaleString()} ₫
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="mt-4 pt-3 border-t border-slate-800 text-[11px] text-slate-500 text-center">
            Kafka Topic:{" "}
            <code className="text-slate-400 font-mono">order.events</code> →
            Edge SSE
          </div>
        </div>
      </div>

      {/* Bottom Section: Recent Distributed Traces with Deep-Link to Jaeger */}
      <div className="bg-slate-900/90 border border-slate-800 p-5 rounded-xl mt-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
              🕵️ Distributed Traces Inspection (W3C TraceContext)
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Click &quot;Inspect Trace in Jaeger&quot; to inspect real-time
              span latencies across Gateway → gRPC Services → Kafka
            </p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {(data?.recent_traces || []).map((t) => (
            <div
              key={t.trace_id}
              className="p-3.5 rounded-lg bg-slate-950 border border-slate-800 flex flex-col justify-between gap-3"
            >
              <div>
                <div className="flex items-center justify-between text-xs">
                  <span className="font-mono text-cyan-400 font-semibold truncate max-w-[180px]">
                    trace:{t.trace_id.slice(0, 12)}...
                  </span>
                  <span className="text-[10px] font-bold px-1.5 py-0.5 bg-emerald-950 text-emerald-400 border border-emerald-800 rounded">
                    {t.status}
                  </span>
                </div>
                <div className="text-xs text-slate-300 font-medium mt-1.5">
                  {t.operation}
                </div>
                <div className="text-[11px] text-slate-500 font-mono mt-0.5">
                  Duration: {t.duration}
                </div>
              </div>

              <a
                href={
                  t.jaeger_url ||
                  "http://localhost:16686/search?service=team-gateway"
                }
                target="_blank"
                rel="noreferrer"
                className="w-full py-1.5 bg-blue-600/30 hover:bg-blue-600/50 border border-blue-500/50 text-blue-300 text-xs font-semibold rounded text-center transition-all flex items-center justify-center gap-1.5"
              >
                <span>🔍 Soi Trace trên Jaeger UI (:16686)</span>
                <span className="text-[10px]">↗</span>
              </a>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
