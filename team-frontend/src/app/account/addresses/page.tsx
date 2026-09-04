import { redirect } from "next/navigation";

import { AddressManager } from "@/features/address/AddressManager";
import { listAddresses } from "@/lib/gateway/addresses";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function AccountAddressesPage() {
  const me = getPrincipal();
  if (!me) redirect("/login");

  const addresses = await listAddresses();

  return (
    <section className="mx-auto max-w-2xl py-2">
      <AddressManager addresses={addresses} />
    </section>
  );
}
