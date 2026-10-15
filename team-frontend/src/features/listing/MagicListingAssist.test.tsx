import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { trackAiInteraction } from "@/lib/analytics/ai";
import { MagicListingAssist } from "./MagicListingAssist";
import { magicListingAction } from "./actions";

vi.mock("./actions", () => ({ magicListingAction: vi.fn() }));
vi.mock("@/lib/analytics/ai", () => ({ trackAiInteraction: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const suggestion = {
  generatedTitle: "Phone Pro - Chính hãng",
  generatedDescription: "Mô tả AI",
  suggestedCategoryId: "cat1",
  suggestedPriceMin: 4000000,
  suggestedPriceMax: 5000000,
  highlightTags: ["Chính Hãng"],
};

function events(action?: string) {
  return vi
    .mocked(trackAiInteraction)
    .mock.calls.map(([surface, a]) => ({ surface, action: a }))
    .filter((e) => action === undefined || e.action === action);
}

const generate = () =>
  fireEvent.click(screen.getByRole("button", { name: "Tạo gợi ý" }));

beforeEach(() => vi.clearAllMocks());

describe("MagicListingAssist ai_interaction", () => {
  // Scenario: A generated draft reports shown once
  it("reports shown once per draft", async () => {
    vi.mocked(magicListingAction).mockResolvedValue({
      ok: true,
      data: suggestion,
    });
    const onApply = vi.fn();
    const { rerender } = render(
      <MagicListingAssist title="Phone" categoryId="" onApply={onApply} />,
    );
    expect(events()).toEqual([]);

    generate();
    await screen.findByTestId("magic-suggestion");
    await waitFor(() => expect(events("shown")).toHaveLength(1));
    expect(events("shown")[0]).toEqual({
      surface: "magic_listing",
      action: "shown",
    });

    // Parent re-renders (typing elsewhere in the form) do not re-report.
    rerender(
      <MagicListingAssist title="Phone 2" categoryId="" onApply={onApply} />,
    );
    rerender(
      <MagicListingAssist title="Phone 23" categoryId="c" onApply={onApply} />,
    );
    expect(events("shown")).toHaveLength(1);
    expect(events("dismissed")).toEqual([]);
  });

  it("reports shown once per draft: a second generated draft reports a second shown", async () => {
    vi.mocked(magicListingAction).mockResolvedValue({
      ok: true,
      data: suggestion,
    });
    render(
      <MagicListingAssist title="Phone" categoryId="" onApply={vi.fn()} />,
    );
    generate();
    await screen.findByTestId("magic-suggestion");
    await waitFor(() => expect(events("shown")).toHaveLength(1));

    generate();
    await waitFor(() => expect(magicListingAction).toHaveBeenCalledTimes(2));
    await screen.findByTestId("magic-suggestion");
    await waitFor(() => expect(events("shown")).toHaveLength(2));
  });

  // Scenario: Applying reports accepted
  it("reports accepted on apply", async () => {
    vi.mocked(magicListingAction).mockResolvedValue({
      ok: true,
      data: suggestion,
    });
    const onApply = vi.fn();
    render(
      <MagicListingAssist title="Phone" categoryId="" onApply={onApply} />,
    );
    generate();
    await screen.findByTestId("magic-suggestion");
    expect(events("accepted")).toEqual([]);

    fireEvent.click(screen.getByRole("button", { name: "Áp dụng tất cả" }));
    expect(onApply).toHaveBeenCalledWith({
      title: "Phone Pro - Chính hãng",
      description: "Mô tả AI",
      price: 4000000,
    });
    expect(events("accepted")).toEqual([
      { surface: "magic_listing", action: "accepted" },
    ]);

    fireEvent.click(screen.getByRole("button", { name: "Áp dụng: Tiêu đề" }));
    expect(events("accepted")).toHaveLength(2);
  });

  // Scenario: A failed or empty generation reports nothing
  it("reports nothing on failed generation", async () => {
    vi.mocked(magicListingAction).mockResolvedValueOnce({
      ok: false,
      error: "AI tạm thời không phản hồi.",
    });
    render(
      <MagicListingAssist title="Phone" categoryId="" onApply={vi.fn()} />,
    );
    generate();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "AI tạm thời không phản hồi.",
    );
    expect(events()).toEqual([]);

    vi.mocked(magicListingAction).mockResolvedValueOnce({
      ok: true,
      data: undefined,
    } as never);
    generate();
    await waitFor(() => expect(magicListingAction).toHaveBeenCalledTimes(2));
    expect(
      await screen.findByText("AI chưa có gợi ý cho sản phẩm này."),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("magic-suggestion")).not.toBeInTheDocument();
    expect(events()).toEqual([]);
  });
});
