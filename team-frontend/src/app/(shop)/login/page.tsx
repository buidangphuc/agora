import type { Metadata } from "next";

import { AuthPanel } from "@/features/auth/AuthPanel";
import { LoginForm } from "@/features/auth/LoginForm";
import { safeReturnUrl } from "@/features/auth/returnUrl";

export const metadata: Metadata = {
  referrer: "no-referrer",
};

export default async function LoginPage({
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
      title="Đăng nhập"
      description="Chào mừng trở lại. Đăng nhập để tiếp tục mua sắm."
    >
      <LoginForm returnUrl={returnUrl} />
    </AuthPanel>
  );
}
