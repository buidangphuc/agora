import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { ADMIN_HOME, ADMIN_SCOPE } from "@/components/shell/nav";
import { LoginForm } from "@/features/auth/LoginForm";
import { safeReturnUrl } from "@/features/auth/returnUrl";
import { getPrincipal } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Đăng nhập quản trị",
  referrer: "no-referrer",
};

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
  if (getPrincipal()?.scopes.includes(ADMIN_SCOPE)) {
    redirect(returnUrl ?? ADMIN_HOME);
  }
  return (
    <main className="mx-auto w-full max-w-page space-y-6 px-4 py-10">
      <h1 className="text-center text-2xl font-bold text-text-primary">
        Đăng nhập
      </h1>
      <LoginForm returnUrl={returnUrl} />
    </main>
  );
}
