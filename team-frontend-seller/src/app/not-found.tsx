import { NotFoundResult } from "@/components/shell/NotFoundResult";
import { SellerAuthShell } from "@/components/shell/SellerAuthShell";

export default function NotFound() {
  return (
    <SellerAuthShell>
      <NotFoundResult />
    </SellerAuthShell>
  );
}
