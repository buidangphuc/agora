import { describe, expect, expectTypeOf, it } from "vitest";

import { type ActionResult, fail, ok } from "./action-result";

function place(stock: number): ActionResult<{ orderId: string }> {
  return stock > 0 ? ok({ orderId: "o-1" }) : fail("Hết hàng");
}

describe("ActionResult", () => {
  it("fail() narrows to an error and exposes no data", () => {
    const res = place(0);
    expect(res).toEqual({ ok: false, error: "Hết hàng" });
    if (!res.ok) {
      expectTypeOf(res.error).toBeString();
      expect(res.error).toBe("Hết hàng");
      // @ts-expect-error `data` is not accessible on the failure branch
      expect(res.data).toBeUndefined();
    } else {
      throw new Error("expected failure branch");
    }
  });

  it("ok() narrows to success and carries optional data", () => {
    const res = place(3);
    if (res.ok) {
      expectTypeOf(res.data).toEqualTypeOf<{ orderId: string } | undefined>();
      expect(res.data?.orderId).toBe("o-1");
    } else {
      throw new Error("expected success branch");
    }
    expect(ok()).toEqual({ ok: true });
  });
});
