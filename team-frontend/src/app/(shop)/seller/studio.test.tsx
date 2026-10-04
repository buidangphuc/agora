import { render, screen } from "@testing-library/react";
import { notFound } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getListing, listCategories } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerEditPage from "./[id]/edit/page";
import SellerNewPage from "./new/page";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  hasScope: vi.fn(),
  getPrincipal: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
  listCategories: vi.fn(),
}));
vi.mock("@/features/listing/actions", () => ({
  saveListingAction: vi.fn(),
  magicListingAction: vi.fn(),
  getUploadUrlAction: vi.fn(),
}));

const listing = {
  id: "listing-12345678",
  sellerId: "me",
  title: "Phone",
  description: "",
  price: 100,
  currency: "VND",
  status: "published",
  imageKeys: [],
  categoryId: "cat1",
  stock: 4,
  variants: [],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(listCategories).mockResolvedValue([]);
  vi.mocked(getPrincipal).mockReturnValue({
    id: "me",
    name: "Me",
    scopes: ["listing.write"],
  });
});

describe("/seller/new", () => {
  it("renders the header and the listing form", async () => {
    render(await SellerNewPage());
    expect(
      screen.getByRole("heading", { name: "Đăng sản phẩm mới", level: 1 }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Đăng bán ngay" }),
    ).toBeInTheDocument();
  });
});

describe("/seller/[id]/edit", () => {
  it("shows a 403 Result and no form for someone else's listing", async () => {
    vi.mocked(getListing).mockResolvedValue({
      ...listing,
      sellerId: "other",
    } as never);
    render(await SellerEditPage({ params: { id: listing.id } }));
    expect(screen.getByText("Không có quyền chỉnh sửa")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại Kênh người bán" }),
    ).toHaveAttribute("href", "/seller");
    expect(screen.queryByRole("button", { name: "Lưu thay đổi" })).toBeNull();
  });

  it("an admin may edit any listing", async () => {
    vi.mocked(getListing).mockResolvedValue({
      ...listing,
      sellerId: "other",
    } as never);
    vi.mocked(getPrincipal).mockReturnValue({
      id: "admin",
      name: "A",
      scopes: ["listing.write", "admin"],
    });
    render(await SellerEditPage({ params: { id: listing.id } }));
    expect(
      screen.getByRole("button", { name: "Lưu thay đổi" }),
    ).toBeInTheDocument();
  });

  it("hands an unknown id to not-found", async () => {
    vi.mocked(getListing).mockResolvedValue(null);
    await SellerEditPage({ params: { id: "nope" } });
    expect(notFound).toHaveBeenCalled();
  });

  it("prefills the form for the owner", async () => {
    vi.mocked(getListing).mockResolvedValue(listing as never);
    render(await SellerEditPage({ params: { id: listing.id } }));
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue("Phone");
    expect(screen.getByLabelText(/Kho hàng/)).toHaveValue(4);
  });
});
