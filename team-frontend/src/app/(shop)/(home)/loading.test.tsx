import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import HomeLoading from "./loading";

describe("home loading", () => {
  it("renders hero, hubs, category and grid skeletons", () => {
    const { container } = render(<HomeLoading />);
    expect(container.firstElementChild).toHaveAttribute("aria-busy", "true");
    // hero + hubs + category + feed heading, plus 12 card skeletons
    expect(
      container.querySelectorAll('[aria-busy="true"]').length,
    ).toBeGreaterThan(12);
    expect(container.querySelector(".aspect-square")).not.toBeNull();
    expect(container.querySelector(".lg\\:grid-cols-6")).not.toBeNull();
  });
});
