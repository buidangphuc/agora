"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";
import { buyerAppUrl } from "@/lib/apps";

import { SellerAccessDenied } from "./SellerAccessDenied";
import { SellerLogoutButton } from "./SellerLogoutButton";
import { acceptSellerTerms } from "./onboarding";
import { SELLER_TERMS_VERSION } from "./onboardingTerms";
import { usePending } from "./usePending";

export function SellerOnboarding() {
  const [error, setError] = useState("");
  const [denied, setDenied] = useState(false);
  const { pending, run } = usePending();

  if (denied) return <SellerAccessDenied restricted />;

  async function accept() {
    setError("");
    const res = await run(() => acceptSellerTerms(SELLER_TERMS_VERSION));
    if (!res) return;
    if (res.error === "denied") setDenied(true);
    else setError(res.error);
  }

  return (
    <main className="mx-auto w-full max-w-3xl px-4 py-10">
      <Result
        status="info"
        title="Trở thành Người Bán"
        subTitle={`Điều khoản Người Bán, phiên bản ${SELLER_TERMS_VERSION}`}
        extra={
          <div className="space-y-3">
            <p>
              Bằng việc tiếp tục, bạn đồng ý với điều khoản dành cho Người Bán
              và bắt đầu đăng bán sản phẩm trên nền tảng.
            </p>
            {error && <Alert type="error" description={error} />}
            <Button onClick={accept} disabled={pending} isLoading={pending}>
              Đồng ý và trở thành Người Bán
            </Button>
            <div className="flex flex-wrap justify-center gap-3">
              <a
                href={buyerAppUrl("/")}
                className="text-sm text-text-secondary hover:text-text-primary"
              >
                Về trang mua sắm
              </a>
            </div>
            <SellerLogoutButton />
          </div>
        }
      />
    </main>
  );
}
