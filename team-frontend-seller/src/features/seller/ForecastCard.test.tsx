import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ViewDemandForecast } from "@/lib/gateway/analytics";
import { ForecastCard } from "./ForecastCard";

const points = Array.from({ length: 14 }, (_, i) => ({
  date: `2026-11-${String(15 + i).padStart(2, "0")}`,
  p10: 1,
  p50: 2.5,
  p90: 4,
}));

const warm: ViewDemandForecast = {
  sellerId: "me",
  listingId: "l1",
  dailyForecasts: points,
  suggestedReorderPoint: 12.2,
  safetyStock: 3,
  modelVersion: "baseline-v1",
  isColdStart: false,
  generatedAt: "2026-11-13T18:30:00.000Z",
};

const ok = (value: ViewDemandForecast) =>
  ({
    status: "fulfilled",
    value,
  }) as PromiseFulfilledResult<ViewDemandForecast>;
const failed = { status: "rejected", reason: new Error("down") } as const;

function show(
  result: PromiseSettledResult<ViewDemandForecast>,
  listingId = "l1",
) {
  render(
    <ForecastCard
      items={[{ listingId, sku: "SKU-1", result }]}
      retryHref="/seller/analytics?range=7d"
    />,
  );
  return screen.getByTestId("forecast-section");
}

describe("ForecastCard", () => {
  it("exposes the card hook that holds the title and every listing section", () => {
    const section = show(failed);
    const card = screen.getByTestId("forecast-card");
    expect(
      within(card).getByText("Dự báo nhu cầu 14 ngày"),
    ).toBeInTheDocument();
    expect(card).toContainElement(section);
    expect(
      within(card).getByText("Dự báo tạm thời không khả dụng"),
    ).toBeInTheDocument();
  });

  it("Seller with history sees the forecast: 14 rows, reorder point rounded up, model, run date in UTC+7", () => {
    const section = show(ok(warm));
    expect(screen.getByText("Dự báo nhu cầu 14 ngày")).toBeInTheDocument();
    expect(within(section).getAllByRole("row")).toHaveLength(15);
    expect(
      within(section).getByText("Điểm đặt hàng lại: 13 đơn vị"),
    ).toBeInTheDocument();
    expect(within(section).getByText(/baseline-v1/)).toBeInTheDocument();
    expect(within(section).getByText(/14\/11\/2026/)).toBeInTheDocument();
  });

  it("Values are rendered exactly as served: renders served quantiles unchanged", () => {
    const section = show(
      ok({
        ...warm,
        dailyForecasts: [{ date: "2026-11-15", p10: 1.0, p50: 2.5, p90: 4.0 }],
      }),
    );
    const row = within(section).getAllByRole("row")[1];
    expect(within(row).getByText("1.0")).toBeInTheDocument();
    expect(within(row).getByText("2.5")).toBeInTheDocument();
    expect(within(row).getByText("4.0")).toBeInTheDocument();
  });

  it("Cold start with stray points is not rendered: cold start never renders points", () => {
    const section = show(
      ok({
        ...warm,
        isColdStart: true,
        dailyForecasts: [
          { date: "2026-11-15", p10: 71.1, p50: 77.7, p90: 88.8 },
        ],
      }),
    );
    expect(
      within(section).getByText("Chưa đủ dữ liệu để dự báo"),
    ).toBeInTheDocument();
    expect(within(section).queryByRole("table")).toBeNull();
    expect(within(section).queryByText(/77\.7/)).toBeNull();
    expect(within(section).queryByText(/Điểm đặt hàng lại/)).toBeNull();
    expect(within(section).queryByText(/baseline-v1/)).toBeNull();
    expect(within(section).queryByText(/14\/11\/2026/)).toBeNull();
  });

  it("Listing without a forecast key: cold start without points shows no numbers", () => {
    const section = show(
      ok({
        ...warm,
        dailyForecasts: [],
        isColdStart: true,
        suggestedReorderPoint: 0,
        modelVersion: "",
        generatedAt: null,
      }),
    );
    expect(
      within(section).getByText("Chưa đủ dữ liệu để dự báo"),
    ).toBeInTheDocument();
    expect(within(section).queryByRole("table")).toBeNull();
    expect(within(section).queryByText(/Điểm đặt hàng lại/)).toBeNull();
  });

  it("Warm response with no points is an error state: empty warm response is treated as an error", () => {
    const section = show(ok({ ...warm, dailyForecasts: [] }));
    expect(
      within(section).getByText("Dự báo tạm thời không khả dụng"),
    ).toBeInTheDocument();
    expect(within(section).queryByRole("table")).toBeNull();
    expect(within(section).queryByText(/Điểm đặt hàng lại/)).toBeNull();
    expect(within(section).queryByText(/baseline-v1/)).toBeNull();
  });

  it("Store unavailable: a rejected call shows the error text and no numbers", () => {
    const section = show(failed);
    expect(
      within(section).getByText("Dự báo tạm thời không khả dụng"),
    ).toBeInTheDocument();
    expect(within(section).queryByRole("table")).toBeNull();
    expect(within(section).queryByText(/Điểm đặt hàng lại/)).toBeNull();
  });
});
