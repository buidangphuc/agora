import {
  FormCardSkeleton,
  HeaderSkeleton,
  TableCardSkeleton,
} from "@/features/seller/SellerSkeletons";

/** /seller/wallet skeleton: header, balance card and ledger table. */
export default function SellerWalletLoading() {
  return (
    <div className="space-y-6" aria-busy="true" data-testid="seller-skeleton">
      <HeaderSkeleton />
      <FormCardSkeleton lines={2} />
      <TableCardSkeleton rows={6} filter={false} />
    </div>
  );
}
