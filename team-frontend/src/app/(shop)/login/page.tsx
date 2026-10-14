import { LoginForm } from "@/features/auth/LoginForm";
import { safeReturnUrl } from "@/features/auth/returnUrl";

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
    <section className="space-y-6 py-2">
      <h1 className="text-center text-2xl font-bold text-text-primary">
        Đăng nhập
      </h1>
      <LoginForm returnUrl={returnUrl} />
    </section>
  );
}
