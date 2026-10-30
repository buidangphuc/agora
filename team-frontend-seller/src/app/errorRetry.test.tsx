import { fireEvent, render, screen } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh }),
}));

import RootError from "./error";
import SellerError from "./seller/error";

const boundaries = {
  root: RootError,
  seller: SellerError,
};

describe("route error boundaries: retry re-fetches server data", () => {
  beforeEach(() => refresh.mockClear());

  for (const [name, Boundary] of Object.entries(boundaries)) {
    it(`${name}: "Thử lại" calls router.refresh() and reset()`, () => {
      const reset = vi.fn();
      render(<Boundary error={new Error("boom")} reset={reset} />);
      fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
      expect(refresh).toHaveBeenCalledTimes(1);
      expect(reset).toHaveBeenCalledTimes(1);
    });
  }
});
