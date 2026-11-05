import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { CampaignView } from "@/lib/gateway/campaigns";

import { CampaignRow } from "./CampaignRow";
import { cancelCampaignAction } from "./actions";

vi.mock("./actions", () => ({
  cancelCampaignAction: vi.fn(),
  updateCampaignAction: vi.fn(),
  createCampaignAction: vi.fn(),
}));

const base: CampaignView = {
  id: "fs-1",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 7,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
  status: "scheduled",
  cancelledAt: "",
};

const withStatus = (status: CampaignView["status"]): CampaignView => ({
  ...base,
  status,
  cancelledAt: status === "cancelled" ? "2026-11-01T00:00:00.000Z" : "",
});

function renderRow(campaign: CampaignView) {
  return render(
    <table>
      <tbody>
        <CampaignRow campaign={campaign} />
      </tbody>
    </table>,
  );
}

const statusOf = () =>
  screen.getByTestId("campaign-status").getAttribute("data-status");

beforeEach(() => vi.clearAllMocks());

describe("CampaignRow", () => {
  it("edit only when scheduled: enabled on a scheduled row", () => {
    renderRow(withStatus("scheduled"));
    const edit = screen.getByTestId("campaign-edit");
    expect(edit).toBeInTheDocument();
    expect(edit).not.toBeDisabled();
    expect(edit).not.toHaveAttribute("aria-disabled", "true");
  });

  it.each(["active", "ended", "cancelled"] as const)(
    "edit only when scheduled: absent or disabled on a %s row",
    (status) => {
      renderRow(withStatus(status));
      const edit = screen.queryByTestId("campaign-edit");
      if (edit) {
        const disabled =
          (edit as HTMLButtonElement).disabled ||
          edit.getAttribute("aria-disabled") === "true";
        expect(disabled).toBe(true);
      }
    },
  );

  it("shows the derived status of the row", () => {
    renderRow(withStatus("ended"));
    expect(screen.getByTestId("campaign-row")).toBeInTheDocument();
    expect(statusOf()).toBe("ended");
  });

  it("offers cancel only for scheduled or active rows", () => {
    const { unmount } = renderRow(withStatus("active"));
    expect(screen.getByTestId("campaign-cancel")).toBeInTheDocument();
    unmount();
    renderRow(withStatus("ended"));
    expect(screen.queryByTestId("campaign-cancel")).toBeNull();
  });

  it("filter and cancel from the list: cancel needs confirmation and the row then shows cancelled", async () => {
    vi.mocked(cancelCampaignAction).mockResolvedValue({
      ok: true,
      campaign: withStatus("cancelled"),
    });
    renderRow(withStatus("active"));
    fireEvent.click(screen.getByTestId("campaign-cancel"));
    expect(cancelCampaignAction).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("campaign-cancel-confirm"));
    await waitFor(() => expect(statusOf()).toBe("cancelled"));
    expect(cancelCampaignAction).toHaveBeenCalledTimes(1);
    expect(cancelCampaignAction).toHaveBeenCalledWith("fs-1");
    expect(screen.queryByTestId("campaign-cancel")).toBeNull();
  });

  it("filter and cancel from the list: a refused cancel keeps the status and shows the error", async () => {
    vi.mocked(cancelCampaignAction).mockResolvedValue({
      ok: false,
      error: "Hệ thống kiểm toán đang tạm ngưng.",
    });
    renderRow(withStatus("active"));
    fireEvent.click(screen.getByTestId("campaign-cancel"));
    fireEvent.click(screen.getByTestId("campaign-cancel-confirm"));
    await waitFor(() =>
      expect(
        screen.getByText("Hệ thống kiểm toán đang tạm ngưng."),
      ).toBeInTheDocument(),
    );
    expect(statusOf()).toBe("active");
  });
});
