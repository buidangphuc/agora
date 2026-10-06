import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { PriceTag } from "@/components/ui/PriceTag";
import { Statistic } from "@/components/ui/Statistic";
import { Table, type TableColumn } from "@/components/ui/Table";
import { formatPrice } from "@/components/ui/format";
import { LinkButton } from "@/features/seller/LinkButton";
import { PayoutButton } from "@/features/seller/PayoutButton";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { SellerPagination } from "@/features/seller/SellerPagination";
import {
  type SearchParams,
  buildListHref,
  parseListParams,
  slicePage,
} from "@/features/seller/listParams";
import {
  type ViewWalletEntry,
  getWalletBalance,
  listLedgerEntries,
} from "@/lib/gateway/payment";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Ví người bán | Kênh người bán" };

const LEDGER_PAGE_SIZE = 20;

const columns: TableColumn<ViewWalletEntry>[] = [
  { key: "type", title: "Loại", render: (e) => e.type || "—" },
  {
    key: "amount",
    title: "Số tiền",
    align: "right",
    render: (e) => <PriceTag price={e.amount} size="md" />,
  },
  { key: "status", title: "Trạng thái", render: (e) => e.status || "—" },
  { key: "createdAt", title: "Ngày", dataIndex: "createdAt" },
];

/** Wallet: balance Statistic, payout confirm Modal, ledger Table + ?page= Pagination. */
export default async function SellerWalletPage({
  searchParams = {},
}: { searchParams?: SearchParams }) {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const { page } = parseListParams(searchParams);
  const [balance, entries] = await Promise.allSettled([
    getWalletBalance(me.id, { throwOnError: true }),
    listLedgerEntries(me.id, { throwOnError: true }),
  ]);
  const balanceValue = balance.status === "fulfilled" ? balance.value : null;
  const ledger = entries.status === "fulfilled" ? entries.value : null;
  const here = buildListHref("/seller/wallet", { page });

  return (
    <>
      <SellerPageHeader
        title="Ví người bán"
        description="Số dư khả dụng và lịch sử giao dịch ví."
      />

      <Card>
        <CardContent className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          {balanceValue === null ? (
            <Alert
              type="error"
              description="Không tải được số dư ví."
              className="flex-1"
              action={
                <LinkButton href={here} size="sm">
                  Thử lại
                </LinkButton>
              }
            />
          ) : (
            <>
              <div className="sm:w-72">
                <Statistic
                  title="Số dư khả dụng"
                  value={formatPrice(balanceValue)}
                />
              </div>
              <PayoutButton sellerId={me.id} balance={balanceValue} />
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            Lịch sử giao dịch{ledger ? ` (${ledger.length})` : ""}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {ledger === null ? (
            <Alert
              type="error"
              description="Không tải được lịch sử giao dịch."
              action={
                <LinkButton href={here} size="sm">
                  Thử lại
                </LinkButton>
              }
            />
          ) : (
            <>
              <Table
                caption="Lịch sử giao dịch ví"
                columns={columns}
                dataSource={slicePage(ledger, page, LEDGER_PAGE_SIZE)}
                rowKey="id"
                emptyText="Chưa có giao dịch nào"
              />
              <SellerPagination
                current={page}
                total={ledger.length}
                pageSize={LEDGER_PAGE_SIZE}
                hrefFor={(p) => buildListHref("/seller/wallet", { page: p })}
              />
            </>
          )}
        </CardContent>
      </Card>
    </>
  );
}
