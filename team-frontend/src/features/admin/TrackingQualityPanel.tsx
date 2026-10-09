import React from "react";

import { Badge } from "@/components/ui/Badge";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
import { Table, type TableColumn } from "@/components/ui/Table";

// The `tracking_quality` section of GET /api/admin/metrics (team-analytics
// GetTrackingQualityReport over the last 24h, via the gateway). Null means
// team-analytics could not be reached: the panel then says so and shows no
// counts, never zeros.

export interface TrackingQualityType {
  event_type: string;
  events: number;
  visitors: number;
  /** 0..1; only meaningful when `listing_scoped`. */
  missing_listing_ratio: number;
  listing_scoped: boolean;
}

export interface TrackingQuality {
  status: "OK" | "DEGRADED";
  reasons: string[];
  last_ingested_at: string | null;
  lag_p50_seconds: number;
  lag_p95_seconds: number;
  decode_failures: number;
  duplicates_skipped: number;
  types: TrackingQualityType[];
}

const NO_DATA = "—";

function count(v: number | null | undefined): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  // Fixed locale: the server render and the browser must format identically.
  return v.toLocaleString("en-US");
}

function seconds(v: number | null | undefined): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  return `${v.toFixed(1)} s`;
}

function percent(v: number | null | undefined): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  return `${(v * 100).toFixed(1)}%`;
}

/** YYYY-MM-DD HH:MM:SS (UTC) of an RFC 3339 timestamp, or a dash. */
function stamp(ts: string | null | undefined): string {
  const d = ts ? new Date(ts) : null;
  return d && !Number.isNaN(d.getTime())
    ? `${d.toISOString().slice(0, 19).replace("T", " ")} UTC`
    : NO_DATA;
}

const COLUMNS: TableColumn<TrackingQualityType>[] = [
  {
    key: "event_type",
    title: "Event type",
    render: (t) => <span className="font-mono">{t.event_type}</span>,
  },
  {
    key: "events",
    title: "Events",
    align: "right",
    render: (t) => count(t.events),
  },
  {
    key: "visitors",
    title: "Visitors",
    align: "right",
    render: (t) => count(t.visitors),
  },
  {
    key: "missing_listing_ratio",
    title: "Missing listing id",
    align: "right",
    render: (t) =>
      t.listing_scoped ? percent(t.missing_listing_ratio) : NO_DATA,
  },
];

export function TrackingQualityPanel({
  quality,
}: {
  quality: TrackingQuality | null | undefined;
}) {
  if (!quality) {
    return (
      <Card data-testid="tracking-quality-panel" className="mt-6">
        <CardHeader>
          <CardTitle>Tracking data quality</CardTitle>
        </CardHeader>
        <Empty description="Tracking data quality is unavailable (team-analytics cannot be reached)." />
      </Card>
    );
  }

  const ok = quality.status === "OK";
  const reasons = quality.reasons ?? [];
  const types = quality.types ?? [];

  return (
    <Card data-testid="tracking-quality-panel" className="mt-6">
      <CardHeader>
        <CardTitle>Tracking data quality</CardTitle>
        <Badge
          data-testid="tracking-quality-status"
          variant={ok ? "success" : "danger"}
          pill
        >
          {quality.status}
        </Badge>
      </CardHeader>
      <div className="space-y-4 p-5">
        {!ok && (
          <ul
            data-testid="tracking-quality-reasons"
            className="flex flex-wrap gap-2"
          >
            {reasons.map((r) => (
              <li key={r}>
                <Badge variant="warning">{r}</Badge>
              </li>
            ))}
          </ul>
        )}
        <Descriptions
          column={3}
          items={[
            {
              key: "last",
              label: "Last ingested",
              children: stamp(quality.last_ingested_at),
            },
            {
              key: "p50",
              label: "Lag p50",
              children: seconds(quality.lag_p50_seconds),
            },
            {
              key: "p95",
              label: "Lag p95",
              children: seconds(quality.lag_p95_seconds),
            },
            {
              key: "decode",
              label: "Decode failures",
              children: count(quality.decode_failures),
            },
            {
              key: "dup",
              label: "Duplicates skipped",
              children: count(quality.duplicates_skipped),
            },
          ]}
        />
        <Table
          caption="Tracking events by type, last 24 hours"
          columns={COLUMNS}
          dataSource={types}
          rowKey="event_type"
          emptyText="No tracking events in the last 24 hours."
          rowProps={(t) => ({
            "data-testid": `tracking-quality-row-${t.event_type}`,
          })}
        />
      </div>
    </Card>
  );
}
