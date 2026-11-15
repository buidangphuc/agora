import type {
  GetRecommendationPerformanceResponse,
  GetSearchPerformanceResponse,
} from "@/generated/platform/analytics/v1/analytics_pb.js";
import type { ReportResult } from "@/lib/gateway/insights";

import { Note, UNAVAILABLE_TEXT, num, pct } from "./format";

type Rows = GetSearchPerformanceResponse["rows"];

function span(h: number): string {
  return h % 24 === 0 && h > 24 ? `${h / 24} ngày` : `${h} giờ`;
}

function PerfTable({ rows }: { rows: Rows }) {
  return (
    <table>
      <thead>
        <tr>
          <th>Placement</th>
          <th>Model version</th>
          <th>Impressions</th>
          <th>Item impressions</th>
          <th>Clicks</th>
          <th>Add to carts</th>
          <th>Purchases</th>
          <th>CTR</th>
          <th>Conversion</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={`${r.placementId}|${r.modelVersion}|${i}`}>
            <td>{r.placementId}</td>
            <td>{r.modelVersion}</td>
            <td>{num(r.impressions)}</td>
            <td>{num(r.itemImpressions)}</td>
            <td>{num(r.clicks)}</td>
            <td>{num(r.addToCarts)}</td>
            <td>{num(r.purchases)}</td>
            <td>{pct(r.ctr)}</td>
            <td>{pct(r.conversionRate)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function Block<T extends { rows: Rows }>({
  title,
  result,
  extra,
}: {
  title: string;
  result: ReportResult<T>;
  extra?: (d: T) => React.ReactNode;
}) {
  return (
    <section>
      <h3>{title}</h3>
      {result.status === "ok" ? (
        <>
          <PerfTable rows={result.data.rows} />
          {extra?.(result.data)}
        </>
      ) : result.status === "empty" ? (
        <Note>Chưa có dữ liệu trong khoảng này</Note>
      ) : (
        <Note>{UNAVAILABLE_TEXT}</Note>
      )}
    </section>
  );
}

export function PlacementsPanel({
  recommendation,
  search,
  windowHours,
}: {
  recommendation: ReportResult<GetRecommendationPerformanceResponse>;
  search: ReportResult<GetSearchPerformanceResponse>;
  windowHours: 24 | 72 | 168;
}) {
  return (
    <section data-testid="insights_placements">
      <h2>Placements</h2>
      <form method="get">
        <label>
          Khoảng thời gian
          <select name="window" defaultValue={String(windowHours)}>
            <option value="24">24 giờ</option>
            <option value="72">3 ngày</option>
            <option value="168">7 ngày</option>
          </select>
        </label>
        <button type="submit">Xem</button>
      </form>
      <p>Khoảng hiển thị: {span(windowHours)}</p>
      <Block
        title="Recommendation"
        result={recommendation}
        extra={(d) =>
          d.fallback.length > 0 ? (
            <ul>
              {d.fallback.map((f) => (
                <li key={f.placementId}>
                  {f.placementId}: fallback {pct(f.fallbackShare)}
                </li>
              ))}
            </ul>
          ) : null
        }
      />
      <Block title="Search" result={search} />
    </section>
  );
}
