import { render, screen, within } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  listCollectionItems,
  listCollections,
  listFavoriteIds,
} from "@/lib/gateway/engagement";
import { getListing } from "@/lib/gateway/listings";
import { getPrincipal } from "@/lib/gateway/session";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/engagement", () => ({
  listCollectionItems: vi.fn(),
  listCollections: vi.fn(),
  listFavoriteIds: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", () => ({ getListing: vi.fn() }));
vi.mock("@/features/engagement/actions", () => ({
  createCollectionResultAction: vi.fn(),
}));
// ListingGrid belongs to ui-phase-discovery; the page must hand it only `listings`.
const gridProps = vi.hoisted(() => vi.fn());
vi.mock("@/features/listing/ListingGrid", () => ({
  ListingGrid: (props: { listings: { id: string; title: string }[] }) => {
    gridProps(props);
    return (
      <ul data-testid="grid">
        {props.listings.map((l) => (
          <li key={l.id}>
            <a href={`/listing/${l.id}`}>{l.title}</a>
          </li>
        ))}
      </ul>
    );
  },
}));

import { createCollectionResultAction } from "@/features/engagement/actions";
import FavoritesPage from "./page";

const collection = { id: "c1", userId: "u", name: "Mua sau", itemCount: 5 };

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(listCollections).mockResolvedValue([collection] as never);
  vi.mocked(listFavoriteIds).mockResolvedValue({
    ids: ["l1", "l2"],
    nextCursor: "",
    total: 2,
  });
  vi.mocked(listCollectionItems).mockResolvedValue({
    ids: ["l1"],
    nextCursor: "",
    total: 1,
  });
  vi.mocked(getListing).mockImplementation(
    async (id: string) => ({ id, title: `SP ${id}` }) as never,
  );
});

describe("/favorites", () => {
  it("redirects an anonymous visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await FavoritesPage({});
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("shows the total in a Badge and hands ListingGrid only the listings", async () => {
    render(await FavoritesPage({}));
    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Sản phẩm yêu thích của tôi",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("2 sản phẩm")).toBeInTheDocument();
    expect(gridProps).toHaveBeenCalledWith({
      listings: [
        { id: "l1", title: "SP l1" },
        { id: "l2", title: "SP l2" },
      ],
    });
  });

  it("an active collection filters by id and shows its name", async () => {
    render(await FavoritesPage({ searchParams: { collection: "c1" } }));
    expect(listCollectionItems).toHaveBeenCalledWith("c1", "");
    expect(listFavoriteIds).not.toHaveBeenCalled();
    expect(
      screen.getByRole("heading", { level: 1, name: "Bộ sưu tập: Mua sau" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "← Xem tất cả sản phẩm yêu thích" }),
    ).toHaveAttribute("href", "/favorites");
  });

  it("an unknown collection renders a 404 Result with a way back", async () => {
    render(
      await FavoritesPage({ searchParams: { collection: "does-not-exist" } }),
    );
    expect(screen.getByText("Không tìm thấy bộ sưu tập")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Xem tất cả sản phẩm yêu thích" }),
    ).toHaveAttribute("href", "/favorites");
    expect(listFavoriteIds).not.toHaveBeenCalled();
    expect(listCollectionItems).not.toHaveBeenCalled();
  });

  it("shows collection-specific Empty text with a link to search", async () => {
    vi.mocked(listCollectionItems).mockResolvedValue({
      ids: [],
      nextCursor: "",
      total: 0,
    });
    render(await FavoritesPage({ searchParams: { collection: "c1" } }));
    expect(
      screen.getByText("Bộ sưu tập này chưa có sản phẩm."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Khám phá sản phẩm ngay" }),
    ).toHaveAttribute("href", "/search");
  });

  it("walks the cursor to the requested ?page= and paginates with links", async () => {
    vi.mocked(listFavoriteIds)
      .mockResolvedValueOnce({ ids: ["a"], nextCursor: "c2", total: 60 })
      .mockResolvedValueOnce({ ids: ["b"], nextCursor: "c3", total: 60 })
      .mockResolvedValueOnce({ ids: ["c"], nextCursor: "", total: 60 });
    render(await FavoritesPage({ searchParams: { page: "2" } }));
    expect(listFavoriteIds).toHaveBeenNthCalledWith(1, "");
    expect(listFavoriteIds).toHaveBeenNthCalledWith(2, "c2");
    expect(listFavoriteIds).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("link", { name: "SP b" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(nav).getByRole("link", { name: "Trang 1" })).toHaveAttribute(
      "href",
      "/favorites",
    );
    expect(within(nav).getByRole("link", { name: "Trang 3" })).toHaveAttribute(
      "href",
      "/favorites?page=3",
    );
  });
});

describe("CollectionsManager", () => {
  it("create is pending/disabled, then toasts and lists the new collection", async () => {
    const user = setupUser();
    vi.mocked(createCollectionResultAction).mockResolvedValue({
      ok: true,
      data: { id: "c9", userId: "u", name: "Quà tặng", itemCount: 0 } as never,
    });
    render(await FavoritesPage({}));
    const submit = screen.getByRole("button", { name: "Tạo mới" });
    expect(submit).toBeDisabled();
    await user.type(screen.getByLabelText("Tên bộ sưu tập"), "Quà tặng");
    expect(submit).toBeEnabled();
    await user.click(submit);

    expect(createCollectionResultAction).toHaveBeenCalledWith("Quà tặng");
    expect(toast.success).toHaveBeenCalledWith('Đã tạo bộ sưu tập "Quà tặng".');
    const rows = screen.getAllByTestId("collection-row");
    expect(rows[0]).toHaveAttribute("data-name", "Quà tặng");
    expect(screen.getByLabelText("Tên bộ sưu tập")).toHaveValue("");
  });

  it("a failed create toasts the error and keeps the typed name", async () => {
    const user = setupUser();
    vi.mocked(createCollectionResultAction).mockResolvedValue({
      ok: false,
      error: "Tạo bộ sưu tập thất bại.",
    });
    render(await FavoritesPage({}));
    await user.type(screen.getByLabelText("Tên bộ sưu tập"), "Quà tặng");
    await user.click(screen.getByRole("button", { name: "Tạo mới" }));
    expect(toast.error).toHaveBeenCalledWith("Tạo bộ sưu tập thất bại.");
    expect(screen.getByLabelText("Tên bộ sưu tập")).toHaveValue("Quà tặng");
    expect(screen.getAllByTestId("collection-row")).toHaveLength(1);
  });

  it("keeps the e2e hooks: form label, row test id and name", async () => {
    render(await FavoritesPage({}));
    expect(
      screen.getByRole("form", { name: "Tạo bộ sưu tập" }),
    ).toBeInTheDocument();
    const row = screen.getByTestId("collection-row");
    expect(row).toHaveAttribute("data-name", "Mua sau");
    expect(within(row).getByRole("link", { name: "Mua sau" })).toHaveAttribute(
      "href",
      "/favorites?collection=c1",
    );
  });
});
