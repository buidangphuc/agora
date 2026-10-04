import { redirect } from "next/navigation";

import { AccountShell } from "@/features/account/AccountShell";
import { AddressManager } from "@/features/address/AddressManager";
import { listAddresses } from "@/lib/gateway/addresses";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Sổ địa chỉ | Marketplace",
};

export default async function AccountAddressesPage() {
  const me = getPrincipal();
  if (!me) redirect("/login");

  const addresses = await listAddresses();

  return (
    <AccountShell
      current="addresses"
      title="Sổ địa chỉ"
      description="Quản lý danh sách địa chỉ giao nhận hàng của bạn."
    >
      <AddressManager addresses={addresses} />
    </AccountShell>
  );
}
