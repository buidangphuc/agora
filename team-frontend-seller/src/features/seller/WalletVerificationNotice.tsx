import { Alert } from "@/components/ui/Alert";
import { LinkButton } from "./LinkButton";

export function WalletVerificationNotice() {
  return (
    <Alert
      type="warning"
      description="Cần xác minh danh tính để rút tiền"
      action={
        <LinkButton href="/seller/verification" size="sm">
          Xác minh ngay
        </LinkButton>
      }
    />
  );
}
