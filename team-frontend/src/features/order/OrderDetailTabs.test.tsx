import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { render, screen } from "@testing-library/react";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import { OrderDetailTabs } from "./OrderDetailTabs";
import { ReturnStateProvider, useReturnState } from "./ReturnState";
import { parseDetailTab } from "./detailTab";

afterEach(() => vi.restoreAllMocks());

describe("OrderDetailTabs", () => {
  it("starts on the tab from the URL and keeps both panels mounted", () => {
    render(
      <OrderDetailTabs
        initialTab="returns"
        timeline={<p>timeline body</p>}
        returns={<p>returns body</p>}
      />,
    );
    expect(
      screen.getByRole("tab", { name: "Trả hàng / Hoàn tiền" }),
    ).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("returns body")).toBeVisible();
    expect(
      screen.getByText("timeline body", { selector: "p" }),
    ).not.toBeVisible();
  });

  it("writes the selected tab to ?tab= and drops it for the default", async () => {
    const replace = vi.spyOn(window.history, "replaceState");
    render(
      <OrderDetailTabs
        initialTab="timeline"
        timeline={<p>timeline body</p>}
        returns={<p>returns body</p>}
      />,
    );
    const user = setupUser();
    await user.click(screen.getByRole("tab", { name: "Trả hàng / Hoàn tiền" }));
    expect(replace).toHaveBeenLastCalledWith(
      null,
      "",
      expect.stringContaining("tab=returns"),
    );
    expect(screen.getByText("returns body")).toBeVisible();

    await user.click(screen.getByRole("tab", { name: "Hành trình" }));
    expect(replace).toHaveBeenLastCalledWith(
      null,
      "",
      expect.not.stringContaining("tab="),
    );
  });

  it("brings the returns panel forward when a return is requested", async () => {
    function RequestButton() {
      const [, setRet] = useReturnState();
      return (
        <button
          type="button"
          onClick={() =>
            setRet({
              id: "r1",
              orderId: "o1",
              reason: "changed_mind",
              refundAmount: 1000,
              status: ReturnStatus.PENDING,
              statusText: "Chờ duyệt",
            })
          }
        >
          request
        </button>
      );
    }
    render(
      <ReturnStateProvider>
        <RequestButton />
        <OrderDetailTabs
          initialTab="timeline"
          timeline={<p>timeline body</p>}
          returns={<p>returns body</p>}
        />
      </ReturnStateProvider>,
    );
    expect(screen.getByText("returns body")).not.toBeVisible();
    await setupUser().click(screen.getByRole("button", { name: "request" }));
    expect(
      screen.getByRole("tab", { name: "Trả hàng / Hoàn tiền" }),
    ).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("returns body")).toBeVisible();
  });

  it("parses unknown tab values as timeline", () => {
    expect(parseDetailTab("returns")).toBe("returns");
    expect(parseDetailTab("x")).toBe("timeline");
    expect(parseDetailTab(undefined)).toBe("timeline");
  });
});
