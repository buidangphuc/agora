import type { GetRetentionCohortsResponse } from "@/generated/platform/analytics/v1/analytics_pb.js";
import type { ReportResult } from "@/lib/gateway/insights";

import { Note, UNAVAILABLE_TEXT, num, pct } from "./format";

export function RetentionPanel({
  result,
  weeks,
}: {
  result: ReportResult<GetRetentionCohortsResponse>;
  weeks: 4 | 12 | 26;
}) {
  return (
    <section data-testid="insights_retention">
      <h2>Retention</h2>
      <form method="get">
        <label>
          Số tuần
          <select name="weeks" defaultValue={String(weeks)}>
            <option value="4">4</option>
            <option value="12">12</option>
            <option value="26">26</option>
          </select>
        </label>
        <button type="submit">Xem</button>
      </form>
      {result.status === "ok" ? (
        <table>
          <thead>
            <tr>
              <th>Cohort week</th>
              <th>Size</th>
              <th>D1</th>
              <th>D7</th>
              <th>D30</th>
              <th>Repeat purchase 30d</th>
              <th>Weekly active</th>
            </tr>
          </thead>
          <tbody>
            {result.data.cohorts.map((c) => (
              <tr key={c.cohortWeek}>
                <td>{c.cohortWeek}</td>
                <td>{num(c.cohortSize)}</td>
                <td>{pct(c.d1)}</td>
                <td>{pct(c.d7)}</td>
                <td>{pct(c.d30)}</td>
                <td>{pct(c.repeatPurchase30d)}</td>
                <td>{c.weeklyActive.map(pct).join(" · ")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : result.status === "empty" ? (
        <Note>Chưa có nhóm đăng ký nào</Note>
      ) : (
        <Note>{UNAVAILABLE_TEXT}</Note>
      )}
    </section>
  );
}
