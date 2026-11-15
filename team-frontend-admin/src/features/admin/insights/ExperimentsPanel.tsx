import type { GetExperimentReadoutResponse } from "@/generated/platform/analytics/v1/analytics_pb.js";
import type { ExperimentResult } from "@/lib/gateway/insights";

import { Note, UNAVAILABLE_TEXT, num, pct } from "./format";

function Body({
  result,
}: {
  result: ExperimentResult<GetExperimentReadoutResponse>;
}) {
  switch (result.status) {
    case "ok":
      return (
        <>
          <p>Cửa sổ chuyển đổi: {result.data.windowDays} ngày</p>
          <table>
            <thead>
              <tr>
                <th>Variant</th>
                <th>Exposed</th>
                <th>Converted</th>
                <th>Rate</th>
                <th>Interval</th>
              </tr>
            </thead>
            <tbody>
              {result.data.variants.map((v) => (
                <tr key={v.variant}>
                  <td>{v.variant}</td>
                  <td>{num(v.exposed)}</td>
                  <td>{num(v.converted)}</td>
                  <td>{pct(v.rate)}</td>
                  <td>
                    {pct(v.ciLow)} – {pct(v.ciHigh)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      );
    case "no_flag":
      return <Note>Nhập flag key để xem kết quả</Note>;
    case "bad_flag":
      return <Note>Flag key không hợp lệ</Note>;
    case "empty":
      return <Note>Chưa có lượt hiển thị cho flag này</Note>;
    case "invalid":
      return <Note>Tham số không hợp lệ</Note>;
    default:
      return <Note>{UNAVAILABLE_TEXT}</Note>;
  }
}

export function ExperimentsPanel({
  result,
  flag,
  rangeDays,
}: {
  result: ExperimentResult<GetExperimentReadoutResponse>;
  flag: string;
  rangeDays: 7 | 14 | 30;
}) {
  return (
    <section data-testid="insights_experiments">
      <h2>Experiments</h2>
      <form method="get">
        <label>
          Flag key
          <input type="text" name="flag" defaultValue={flag} />
        </label>
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
      <Body result={result} />
    </section>
  );
}
