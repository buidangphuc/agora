import { AuthPanel } from "@/features/auth/AuthPanel";
import { RegisterForm } from "@/features/auth/RegisterForm";
import { safeReturnUrl } from "@/features/auth/returnUrl";

export default async function RegisterPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const raw = (await searchParams).returnUrl;
  const candidate = typeof raw === "string" ? raw : "";
  const safe = safeReturnUrl(candidate);
  const returnUrl = candidate !== "" && safe === candidate ? safe : undefined;
  return (
    <AuthPanel
      title="Tạo tài khoản"
      description="Chỉ mất một phút để bắt đầu mua sắm."
    >
      <RegisterForm returnUrl={returnUrl} />
    </AuthPanel>
  );
}
