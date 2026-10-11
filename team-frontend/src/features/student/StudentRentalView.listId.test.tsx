import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  listIds,
  viewListing,
} from "@/test/listBeacons";

vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import { StudentRentalView } from "./StudentRentalView";

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
});
afterEach(endBeaconCapture);

describe("StudentRentalView item list id", () => {
  it("[Fan-out rows share a group id] beacons and view_item_list carry item list id student_rentals", async () => {
    render(
      <StudentRentalView
        initialListings={[viewListing("a"), viewListing("b")]}
      />,
    );
    await drain();
    expect(beaconsOf(fetchMock).length).toBeGreaterThan(0);
    expect(listIds(fetchMock)).toEqual({
      beacon: ["student_rentals"],
      layer: ["student_rentals"],
    });
  });
});
