import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { enqueue, enqueueBatch } = vi.hoisted(() => ({
  enqueue: vi.fn(),
  enqueueBatch: vi.fn(),
}));
vi.mock("@/lib/analytics/queue", () => ({
  beaconQueue: { enqueue, enqueueBatch },
}));
vi.mock("@/lib/analytics/dataLayer", () => ({ pushDataLayer: vi.fn() }));

import { ExperimentExposure } from "./ExperimentExposure";

function queued(): Array<Record<string, unknown>> {
  return [
    ...enqueue.mock.calls.map((c) => c[0]),
    ...enqueueBatch.mock.calls.flatMap((c) => c[0]),
  ];
}

beforeEach(() => {
  enqueue.mockClear();
  enqueueBatch.mockClear();
});

// Scenario: Exposure is sent once per mount with only flag and variant
describe("ExperimentExposure", () => {
  it("sends one exposure per mount", () => {
    const view = render(
      <ExperimentExposure flag="home-feed-ranker" variant="b" />,
    );
    view.rerender(<ExperimentExposure flag="home-feed-ranker" variant="b" />);
    view.rerender(<ExperimentExposure flag="home-feed-ranker" variant="b" />);

    const beacons = queued();
    expect(beacons).toHaveLength(1);
    expect(beacons[0].type).toBe("experiment_exposure");
    expect(beacons[0].properties).toEqual({
      flag: "home-feed-ranker",
      variant: "b",
    });
  });

  // Scenario: No variant sends no exposure
  it("sends nothing without a variant", () => {
    render(<ExperimentExposure flag="home-feed-ranker" variant={null} />);
    render(<ExperimentExposure flag="home-feed-ranker" variant="" />);
    expect(queued()).toHaveLength(0);
  });
});
