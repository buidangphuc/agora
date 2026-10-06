import { Card, CardContent } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import type { ViewAddress } from "@/lib/gateway/addresses";
import { AddAddressButton, AddressActions } from "./AddressActions";

function fullAddress(addr: ViewAddress): string {
  return [addr.street, addr.ward, addr.district, addr.city]
    .filter(Boolean)
    .join(", ");
}

/**
 * Address book: a server-rendered card list. Only the buttons that open
 * modals or mutate (AddAddressButton, AddressActions) are client islands.
 */
export function AddressManager({ addresses }: { addresses: ViewAddress[] }) {
  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <AddAddressButton className="w-full sm:w-auto" />
      </div>

      {addresses.length === 0 ? (
        <Card>
          <Empty
            description="Bạn chưa có địa chỉ nhận hàng nào."
            action={
              <AddAddressButton label="Thêm địa chỉ ngay" variant="outline" />
            }
          />
        </Card>
      ) : (
        <ul className="space-y-3">
          {addresses.map((addr) => (
            <li key={addr.id} data-testid="address-card">
              <Card>
                <CardContent className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-base font-semibold text-text-primary">
                        {addr.recipientName}
                      </span>
                      <span className="text-sm text-text-secondary">
                        {addr.phone}
                      </span>
                      {addr.isDefault && <Tag color="primary">Mặc định</Tag>}
                    </div>
                    <p className="text-sm text-text-primary">
                      {fullAddress(addr)}
                    </p>
                  </div>
                  <AddressActions address={addr} />
                </CardContent>
              </Card>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
