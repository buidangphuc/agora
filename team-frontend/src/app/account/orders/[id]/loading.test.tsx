import { render } from "@testing-library/react";
import React from "react";
import { describe, expect, it } from "vitest";

import OrderDetailLoading from "./loading";

describe("order detail loading", () => {
  it("renders a busy skeleton", () => {
    const { container } = render(<OrderDetailLoading />);
    expect(container.querySelector('section[aria-busy="true"]')).not.toBeNull();
  });
});
