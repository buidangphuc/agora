import type { ViewBundle } from "@/lib/gateway/listings";

import { AddBundleButton } from "./AddBundleButton";
import { bundleAvailability } from "./bundleAvailability";

export function BundleCard({
  bundle,
  shopListings,
  shopId,
}: {
  bundle: ViewBundle;
  shopListings: { id: string; title: string; price: number }[];
  shopId: string;
}) {
  const availability = bundleAvailability(bundle, shopListings);
  const titles = bundle.listingIds.map(
    (id) => shopListings.find((l) => l.id === id)?.title,
  );
  return (
    <li className="rounded-xs border border-border-subtle p-3 text-xs">
      <p className="font-semibold text-text-primary">{bundle.title}</p>
      <p className="mt-0.5 text-text-disabled">
        {bundle.listingIds.length} sản phẩm
      </p>
      {availability.available ? (
        <ul className="mt-1 list-disc pl-4">
          {titles.map((t, i) => (
            <li key={bundle.listingIds[i]}>{t}</li>
          ))}
        </ul>
      ) : null}
      <p className="mt-1 font-bold text-action-primary">
        {bundle.bundlePrice.toLocaleString("vi-VN")} VND
      </p>
      {availability.available ? (
        <>
          {availability.saving !== null ? (
            <p className="text-success">
              Tiết kiệm {availability.saving.toLocaleString("vi-VN")} VND
            </p>
          ) : null}
          <AddBundleButton bundleId={bundle.id} shopId={shopId} />
        </>
      ) : (
        <p className="mt-2 text-text-disabled">Combo tạm không khả dụng</p>
      )}
    </li>
  );
}
