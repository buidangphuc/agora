import type { GetAiSurfaceReportResponse } from "@/generated/platform/analytics/v1/analytics_pb.js";
import type { ReportResult } from "@/lib/gateway/insights";

import { NO_DATA, Note, UNAVAILABLE_TEXT, num, pct } from "./format";

function share(part: bigint, shown: bigint): string {
  if (shown <= 0n) return NO_DATA;
  return pct(Number(part) / Number(shown));
}

export function AiSurfacesPanel({
  result,
  rangeDays,
}: {
  result: ReportResult<GetAiSurfaceReportResponse>;
  rangeDays: 7 | 14 | 30;
}) {
  return (
    <section data-testid="insights_ai_surfaces">
      <h2>AI surfaces</h2>
      <form method="get">
        <label>
          Khoảng
          <select name="range" defaultValue={String(rangeDays)}>
            <option value="7">7 ngày</option>
            <option value="14">14 ngày</option>
            <option value="30">30 ngày</option>
          </select>
        </label>
        <button type="submit">Xem</button>
      </form>
      {result.status === "ok" ? (
        <table>
          <thead>
            <tr>
              <th>Surface</th>
              <th>Model version</th>
              <th>Shown</th>
              <th>Accepted</th>
              <th>Edited</th>
              <th>Dismissed</th>
              <th>Accepted share</th>
              <th>Dismissed share</th>
            </tr>
          </thead>
          <tbody>
            {result.data.rows.map((r, i) => (
              <tr key={`${r.surface}|${r.modelVersion}|${i}`}>
                <td>{r.surface}</td>
                <td>{r.modelVersion}</td>
                <td>{num(r.shown)}</td>
                <td>{num(r.accepted)}</td>
                <td>{num(r.edited)}</td>
                <td>{num(r.dismissed)}</td>
                <td>{share(r.accepted, r.shown)}</td>
                <td>{share(r.dismissed, r.shown)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : result.status === "empty" ? (
        <Note>Chưa có tương tác AI trong khoảng này</Note>
      ) : (
        <Note>{UNAVAILABLE_TEXT}</Note>
      )}
    </section>
  );
}
