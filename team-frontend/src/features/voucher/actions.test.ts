import { describe, expect, it, vi } from "vitest";

vi.mock("@/lib/gateway/promotion", () => ({
  createVoucher: vi.fn(),
  previewVoucher: vi.fn(),
}));

import * as actions from "./actions";

describe("buyer voucher actions", () => {
  it("Seller visiting the buyer voucher page sees no create form: createVoucherAction is not exported", () => {
    expect(Object.keys(actions)).not.toContain("createVoucherAction");
    expect(Object.keys(actions)).toContain("previewVoucherAction");
  });
});
