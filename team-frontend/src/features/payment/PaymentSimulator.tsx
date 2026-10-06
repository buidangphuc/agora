"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Result } from "@/components/ui/Result";
import { useToast } from "@/components/ui/ToastProvider";
import {
  linkButtonOutline,
  linkButtonPrimary,
} from "@/features/cart/linkButton";
import { processMockPaymentAction } from "@/features/order/actions";

export type SimulatorPhase = "idle" | "success" | "failed";

/**
 * Mock payment simulator. The two buttons are mutually exclusive and
 * pending-aware (both disabled while one runs). The outcome is shown as a
 * Result; a failed attempt can be retried.
 */
export function PaymentSimulator({
  orderId,
  transactionId,
  initialPhase,
}: {
  orderId: string;
  transactionId: string;
  initialPhase: SimulatorPhase;
}) {
  const toast = useToast();
  const [phase, setPhase] = useState<SimulatorPhase>(initialPhase);
  const [pending, setPending] = useState<"success" | "failed" | null>(null);
  const [failure, setFailure] = useState("");

  async function simulate(success: boolean) {
    if (pending) return;
    setPending(success ? "success" : "failed");
    try {
      const res = await processMockPaymentAction(transactionId, success);
      if (res.ok) {
        toast.success("Thanh toán giả lập thành công.");
        setPhase("success");
      } else {
        setFailure(res.error);
        toast.error(res.error);
        setPhase("failed");
      }
    } catch {
      const msg = "Có lỗi xảy ra khi xử lý thanh toán giả lập.";
      setFailure(msg);
      toast.error(msg);
      setPhase("failed");
    } finally {
      setPending(null);
    }
  }

  if (phase === "success") {
    return (
      <Result
        status="success"
        title="Thanh toán thành công"
        subTitle="Đơn hàng của bạn đã được thanh toán."
        extra={
          <div className="flex flex-wrap justify-center gap-3">
            <Link href="/account/orders" className={linkButtonPrimary}>
              Xem đơn hàng
            </Link>
            <Link href="/" className={linkButtonOutline}>
              Tiếp tục mua sắm
            </Link>
          </div>
        }
      />
    );
  }

  if (phase === "failed") {
    return (
      <Result
        status="error"
        title="Thanh toán thất bại"
        subTitle={failure || "Giao dịch chưa được thanh toán."}
        extra={
          <div className="flex flex-wrap justify-center gap-3">
            <Button onClick={() => setPhase("idle")}>Thử lại</Button>
            <Link
              href={`/account/orders/${orderId}`}
              className={linkButtonOutline}
            >
              Đổi phương thức
            </Link>
          </div>
        }
      />
    );
  }

  return (
    <Card aria-busy={pending ? "true" : undefined}>
      <CardHeader>
        <CardTitle>Mô phỏng thanh toán</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <Button
          size="lg"
          className="w-full"
          isLoading={pending === "success"}
          disabled={pending !== null}
          onClick={() => simulate(true)}
        >
          Thanh toán thành công
        </Button>
        <Button
          size="lg"
          variant="outline"
          className="w-full"
          isLoading={pending === "failed"}
          disabled={pending !== null}
          onClick={() => simulate(false)}
        >
          Thanh toán thất bại
        </Button>
        <p className="text-center text-xs text-text-secondary">
          Môi trường demo: không có khoản tiền thật nào bị trừ.
        </p>
        <div className="text-center">
          <Link
            href="/account/orders"
            className="text-xs text-text-secondary hover:text-action-primary"
          >
            Bỏ qua và thanh toán sau
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}
