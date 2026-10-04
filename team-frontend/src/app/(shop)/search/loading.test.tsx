import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import SearchLoading from "./loading";

describe("search loading", () => {
  it("renders the header, filter column and 24 card skeletons in the real grid", () => {
    const { container } = render(<SearchLoading />);
    const grid = container.querySelector(".lg\\:grid-cols-6") as HTMLElement;
    expect(grid).not.toBeNull();
    expect(grid.children).toHaveLength(24);
    // filter column stays: mobile button placeholder + desktop card
    expect(container.querySelector(".lg\\:w-64")).not.toBeNull();
  });
});
