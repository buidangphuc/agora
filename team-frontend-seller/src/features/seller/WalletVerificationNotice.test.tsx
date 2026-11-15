import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { WalletVerificationNotice } from "./WalletVerificationNotice";

describe("WalletVerificationNotice", () => {
  it("Wallet shows a notice while not verified: text and link to the verification page", () => {
    render(<WalletVerificationNotice />);
    expect(
      screen.getByText("Cần xác minh danh tính để rút tiền"),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /xác minh/i })).toHaveAttribute(
      "href",
      "/seller/verification",
    );
  });
});
