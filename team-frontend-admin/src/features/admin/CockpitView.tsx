"use client";

import React, { useEffect, useState } from "react";

import { Badge, type BadgeVariant } from "@/components/ui/Badge";
import { Card, CardFooter, CardHeader } from "@/components/ui/Card";
import { Link } from "@/components/ui/Link";
import { Table } from "@/components/ui/Table";

import {
  type TrackingQuality,
  TrackingQualityPanel,
} from "./TrackingQualityPanel";

// Every figure on this page comes from the gateway's GET /api/admin/metrics
// (Prometheus telemetry, team-analytics order facts, Jaeger traces). The first
// payload is fetched server-side with the session token and passed in as
// `initial`; refreshes go through the same-origin route handler, which forwards
// the token (the browser never holds it). When a source has no data the widget
// says so — it never falls back to a placeholder number.

const METRICS_URL = "/api/admin/metrics";
const POLL_MS = 3000;

const JAEGER_URL = "http://localhost:16686";
const KAFKA_UI_URL = "http://localhost:8088";
const GRAFANA_URL = "http://localhost:3001";

const NO_DATA = "—";
const NO_DATA_TEXT = "Chưa có dữ liệu";

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
  span_count: number;
  duration_ms: number;
  started_at: string;
  jaeger_url: string;
}

export interface RecentOrder {
  order_id: string;
  seller_id: string;
  /** Minor units (₫). */
  total: number;
  paid_at: string;
}

export interface CockpitData {
  timestamp: string;
  prometheus_available?: boolean;
  total_rps: number;
  avg_latency_ms: number | null;
  total_orders_24h: number | null;
  total_revenue_24h: number | null;
  services: ServiceHealth[] | null;
  recent_orders: RecentOrder[] | null;
  recent_traces: TraceSummary[] | null;
  /** Null when team-analytics cannot be reached (or an older gateway). */
  tracking_quality?: TrackingQuality | null;
}

function num(v: number | null | undefined, digits: number, unit = ""): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  return `${v.toFixed(digits)}${unit}`;
}

function count(v: number | null | undefined): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  // Fixed locale: the server render and the browser must format identically.
  return v.toLocaleString("en-US");
}

const STATUS_TONE: Record<string, BadgeVariant> = {
  HEALTHY: "success",
  DEGRADED: "danger",
  IDLE: "primary",
  NO_DATA: "neutral",
  UNKNOWN: "neutral",
};

/** HH:MM:SS (UTC) of an RFC 3339 timestamp, or a dash if it does not parse. */
function clock(ts: string | undefined): string {
  const d = ts ? new Date(ts) : null;
  return d && !Number.isNaN(d.getTime())
    ? `${d.toISOString().slice(11, 19)} UTC`
    : NO_DATA;
}

export function CockpitView({ initial }: { initial: CockpitData | null }) {
  const [data, setData] = useState<CockpitData | null>(initial);
  const [loaded, setLoaded] = useState(initial !== null);
  const [fetchFailed, setFetchFailed] = useState(false);
  const [denied, setDenied] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const fetchMetrics = async () => {
      try {
        const res = await fetch(METRICS_URL, { cache: "no-store" });
        if (res.status === 401 || res.status === 403) {
          if (cancelled) return;
          setData(null);
          setDenied(true);
          return;
        }
        if (!res.ok) throw new Error(`status ${res.status}`);
        const json = (await res.json()) as CockpitData;
        if (cancelled) return;
        setData(json);
        setDenied(false);
        setFetchFailed(false);
      } catch {
        if (cancelled) return;
        setData(null);
        setFetchFailed(true);
      } finally {
        if (!cancelled) setLoaded(true);
      }
    };

    // The server already rendered `initial`; the first poll waits one interval.
    if (initial === null) fetchMetrics();
    const interval = setInterval(fetchMetrics, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [initial]);

  const promUp = data?.prometheus_available !== false && data !== null;
  const services = data?.services ?? [];
  const traces = data?.recent_traces ?? [];
  const orders = data?.recent_orders ?? [];

  const banner = denied
    ? "Phiên đăng nhập hết hạn hoặc tài khoản không có quyền admin."
    : fetchFailed
      ? "Không kết nối được gateway — chưa có số liệu."
      : loaded && data && data.prometheus_available === false
        ? "Prometheus không khả dụng — chưa có số liệu telemetry."
        : null;

  return (
    <div className="space-y-6 md:space-y-8">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="min-w-0 space-y-2">
          <Badge variant={promUp ? "success" : "neutral"} pill>
            {promUp
              ? "● Prometheus kết nối"
              : loaded
                ? "○ Chưa có số liệu"
                : "○ Đang tải"}
          </Badge>
          <h1 className="text-2xl font-bold tracking-tight text-text-primary md:text-3xl">
            Platform Operations HUD & SRE Cockpit
          </h1>
          <p className="text-sm text-text-secondary md:text-base">
            Telemetry thật từ Prometheus (RPS, Latency, Error Rate theo gRPC
            service) & liên kết Distributed Tracing
          </p>
        </div>

        <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap lg:max-w-xl lg:justify-end">
          <Link href="/admin/audit" variant="secondary" size="sm">
            Nhật ký kiểm toán
          </Link>
          <Link href="/admin/users" variant="secondary" size="sm">
            Quản lý người dùng
          </Link>
          <Link href="/admin/listings" variant="secondary" size="sm">
            Kiểm duyệt tin đăng
          </Link>
          <Link href="/admin/cases" variant="secondary" size="sm">
            Hộp thư vụ việc
          </Link>
          <Link
            href={JAEGER_URL}
            target="_blank"
            rel="noreferrer"
            variant="ghost"
            size="sm"
          >
            Open Jaeger (:16686) ↗
          </Link>
          <Link
            href={KAFKA_UI_URL}
            target="_blank"
            rel="noreferrer"
            variant="ghost"
            size="sm"
          >
            Open Kafka UI (:8088) ↗
          </Link>
          <Link
            href={GRAFANA_URL}
            target="_blank"
            rel="noreferrer"
            variant="ghost"
            size="sm"
          >
            Grafana (:3001) ↗
          </Link>
        </div>
      </div>

      {banner && (
        <output className="block rounded-2xl border border-border-subtle bg-surface-muted px-4 py-3 text-sm font-medium text-text-warning md:px-6">
          {banner}
        </output>
      )}

      <div
        data-testid="cockpit_metrics"
        className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 md:gap-6"
      >
        <Card padded>
          <div className="text-xs font-semibold uppercase tracking-wider text-text-secondary">
            Total Gateway Throughput
          </div>
          <div className="mt-3 flex items-baseline gap-2 text-2xl font-bold text-text-primary md:text-3xl">
            {promUp ? num(data?.total_rps, 1) : NO_DATA}{" "}
            <span className="text-sm font-medium text-text-secondary">
              req/sec
            </span>
          </div>
          <div className="mt-2 text-sm text-text-secondary">
            Tổng gRPC client rate của gateway (cửa sổ 1 phút)
          </div>
        </Card>

        <Card padded>
          <div className="text-xs font-semibold uppercase tracking-wider text-text-secondary">
            Average gRPC P95 Latency
          </div>
          <div className="mt-3 flex items-baseline gap-2 text-2xl font-bold text-text-primary md:text-3xl">
            {promUp ? num(data?.avg_latency_ms, 1) : NO_DATA}{" "}
            <span className="text-sm font-medium text-text-secondary">ms</span>
          </div>
          <div className="mt-2 text-sm text-text-secondary">
            Trung bình P95 các service đang có traffic (5 phút)
          </div>
        </Card>

        <Card padded>
          <div className="text-xs font-semibold uppercase tracking-wider text-text-secondary">
            24h Paid Orders
          </div>
          <div className="mt-3 flex items-baseline gap-2 text-2xl font-bold text-text-primary md:text-3xl">
            {count(data?.total_orders_24h)}{" "}
            <span className="text-sm font-medium text-text-secondary">đơn</span>
          </div>
          {typeof data?.total_orders_24h !== "number" && (
            <div className="mt-2 text-sm text-text-secondary">
              Chưa có dữ liệu (team-analytics chưa khả dụng)
            </div>
          )}
        </Card>

        <Card padded>
          <div className="text-xs font-semibold uppercase tracking-wider text-text-secondary">
            24h Gross Revenue (GMV)
          </div>
          <div className="mt-3 flex items-baseline gap-2 text-2xl font-bold text-text-primary md:text-3xl">
            {count(data?.total_revenue_24h)}{" "}
            <span className="text-sm font-medium text-text-secondary">₫</span>
          </div>
          {typeof data?.total_revenue_24h !== "number" && (
            <div className="mt-2 text-sm text-text-secondary">
              Chưa có dữ liệu (team-analytics chưa khả dụng)
            </div>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div data-testid="cockpit_health" className="min-w-0 lg:col-span-2">
          <Card>
            <CardHeader>
              <h2 className="text-base font-semibold text-text-primary">
                Microservices Health & RED Metrics Radar
              </h2>
              <span className="shrink-0 text-sm text-text-secondary">
                {services.length > 0
                  ? `${services.length} service theo dõi`
                  : "Chưa có dữ liệu"}
              </span>
            </CardHeader>

            {services.length === 0 ? (
              <p className="px-6 py-10 text-center text-sm text-text-secondary">
                Chưa có dữ liệu service.
              </p>
            ) : (
              <Table<ServiceHealth>
                caption="Microservices Health & RED Metrics Radar"
                columns={[
                  {
                    key: "name",
                    title: "Service Name",
                    sticky: true,
                    render: (s) => (
                      <span className="whitespace-nowrap font-mono font-semibold">
                        {s.name}
                      </span>
                    ),
                  },
                  {
                    key: "port",
                    title: "Port",
                    hideBelow: "md",
                    render: (s) => (
                      <span className="font-mono text-text-secondary">
                        :{s.port}
                      </span>
                    ),
                  },
                  {
                    key: "status",
                    title: "Status",
                    render: (s) => (
                      <Badge
                        variant={STATUS_TONE[s.status] ?? STATUS_TONE.UNKNOWN}
                        pill
                      >
                        {s.status}
                      </Badge>
                    ),
                  },
                  {
                    key: "rps",
                    title: "RPS",
                    align: "right",
                    render: (s) => (
                      <span className="whitespace-nowrap font-mono">
                        {s.status !== "UNKNOWN" &&
                        promUp &&
                        s.status !== "NO_DATA"
                          ? num(s.rps, 1)
                          : NO_DATA}
                      </span>
                    ),
                  },
                  {
                    key: "p95",
                    title: "P95 Latency",
                    align: "right",
                    render: (s) => (
                      <span className="whitespace-nowrap font-mono">
                        {num(s.p95_latency_ms, 1, " ms")}
                      </span>
                    ),
                  },
                  {
                    key: "p99",
                    title: "P99 Latency",
                    align: "right",
                    hideBelow: "sm",
                    render: (s) => (
                      <span className="whitespace-nowrap font-mono">
                        {num(s.p99_latency_ms, 1, " ms")}
                      </span>
                    ),
                  },
                  {
                    key: "error_rate",
                    title: "Error Rate",
                    align: "right",
                    render: (s) => (
                      <span className="whitespace-nowrap font-mono">
                        {typeof s.error_rate === "number"
                          ? `${(s.error_rate * 100).toFixed(2)}%`
                          : NO_DATA}
                      </span>
                    ),
                  },
                ]}
                dataSource={services}
                rowKey="name"
              />
            )}
            <CardFooter>
              <span>
                Nguồn: gRPC client metrics của gateway. Error rate tính mọi mã
                gRPC khác OK (kể cả từ chối nghiệp vụ như hết hàng).
              </span>
            </CardFooter>
          </Card>
        </div>

        <div data-testid="cockpit_orders" className="min-w-0">
          <Card>
            <CardHeader>
              <h2 className="text-base font-semibold text-text-primary">
                Đơn hàng gần đây
              </h2>
              <Badge variant="neutral" pill>
                order_facts
              </Badge>
            </CardHeader>

            {orders.length === 0 ? (
              <p className="px-6 py-10 text-center text-sm text-text-secondary">
                {NO_DATA_TEXT}
              </p>
            ) : (
              <ul className="max-h-96 space-y-2 overflow-y-auto p-4 md:p-6">
                {orders.map((o) => (
                  <li
                    key={o.order_id}
                    className="flex items-center justify-between gap-3 rounded-xl border border-border-subtle bg-surface-muted px-4 py-3 text-sm"
                  >
                    <div className="min-w-0">
                      <div className="truncate font-mono font-semibold text-text-primary">
                        {o.order_id}
                      </div>
                      <div className="font-mono text-xs text-text-secondary">
                        {clock(o.paid_at)}
                      </div>
                    </div>
                    <div className="shrink-0 whitespace-nowrap font-mono font-semibold text-text-primary">
                      {count(o.total)} ₫
                    </div>
                  </li>
                ))}
              </ul>
            )}

            <CardFooter>
              <span>
                Nguồn:{" "}
                <code className="font-mono text-text-primary">
                  order.events
                </code>{" "}
                → team-analytics (làm mới mỗi {POLL_MS / 1000}s)
              </span>
            </CardFooter>
          </Card>
        </div>
      </div>

      <TrackingQualityPanel quality={data?.tracking_quality} />

      <Card>
        <CardHeader>
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-text-primary">
              Distributed Traces Inspection (W3C TraceContext)
            </h2>
            <p className="mt-1 text-sm text-text-secondary">
              Trace gần đây của team-gateway (1 giờ qua) từ Jaeger.
            </p>
          </div>
        </CardHeader>
        <div className="space-y-4 p-4 md:p-6">
          {traces.length > 0 && (
            <div
              data-testid="cockpit_traces"
              className="grid grid-cols-1 gap-4 md:grid-cols-3"
            >
              {traces.map((t) => (
                <a
                  key={t.trace_id}
                  href={t.jaeger_url}
                  target="_blank"
                  rel="noreferrer"
                  className="block min-w-0 rounded-2xl border border-border-subtle bg-surface-muted p-4 text-sm text-text-primary transition duration-150 hover:border-border-strong focus-visible:ring-2 focus-visible:ring-focus-ring"
                >
                  <div className="truncate font-mono text-text-link">
                    {t.trace_id}
                  </div>
                  <div className="mt-1 break-words font-semibold">
                    {t.operation}
                  </div>
                  <div className="mt-1 font-mono text-xs text-text-secondary">
                    {t.span_count} spans · {num(t.duration_ms, 1, " ms")} ·{" "}
                    {clock(t.started_at)}
                  </div>
                </a>
              ))}
            </div>
          )}

          <Link
            href={`${JAEGER_URL}/search?service=team-gateway`}
            target="_blank"
            rel="noreferrer"
            variant="secondary"
            size="sm"
            fullWidth="mobile"
          >
            Trace của team-gateway trên Jaeger ↗
          </Link>

          {traces.length === 0 && (
            <p className="text-sm text-text-secondary">
              {NO_DATA_TEXT} — dùng liên kết Jaeger ở trên.
            </p>
          )}
        </div>
      </Card>
    </div>
  );
}
