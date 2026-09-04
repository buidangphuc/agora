import Link from "next/link";
import { redirect } from "next/navigation";

import { ListingForm } from "@/features/listing/ListingForm";
import { listCategories } from "@/lib/gateway/listings";
import { hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function SellerNewPage() {
  if (!hasScope("listing.write")) redirect("/login");
  const categories = await listCategories();

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between border-b border-gray-200 bg-white p-5 rounded-xl shadow-2xs">
        <div>
          <h1 className="text-lg font-bold text-gray-900">Đăng sản phẩm mới</h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Điền đầy đủ thông tin để sản phẩm của bạn tiếp cận hàng triệu khách
            hàng
          </p>
        </div>
        <Link
          href="/seller"
          className="text-xs font-medium text-brand hover:underline"
        >
          ← Quay lại danh sách
        </Link>
      </div>

      <ListingForm categories={categories} submitLabel="Đăng bán ngay" />
    </div>
  );
}
