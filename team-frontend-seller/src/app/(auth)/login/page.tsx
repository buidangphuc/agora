import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { LoginForm } from "@/features/auth/LoginForm";
import { safeReturnUrl } from "@/features/auth/returnUrl";
import { getPrincipal, hasSellerAccess } from "@/lib/gateway/session";

export const metadata: Metadata = {
  referrer: "no-referrer",
};

const SELLER_HOME = "/seller";

function safeParam(raw: string | string[] | undefined): string | undefined {
  const candidate = typeof raw === "string" ? raw : "";
  return candidate !== "" && safeReturnUrl(candidate) === candidate
    ? candidate
    : undefined;
}

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  const sp = await searchParams;
  const returnUrl =
    typeof sp.next === "string" ? safeParam(sp.next) : safeParam(sp.returnUrl);
  if (hasSellerAccess(getPrincipal())) redirect(returnUrl ?? SELLER_HOME);
  return (
    <section className="space-y-6 py-2">
      <h1 className="text-center text-2xl font-bold text-text-primary">
        Đăng nhập
      </h1>
      <LoginForm returnUrl={returnUrl} />
    </section>
  );
}
