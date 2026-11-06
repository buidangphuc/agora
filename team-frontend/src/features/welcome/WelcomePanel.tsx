import { cookies } from "next/headers";

import { formatPrice } from "@/components/ui/format";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { listAddresses } from "@/lib/gateway/addresses";
import { listFollowedSellers } from "@/lib/gateway/engagement";
import { listBuyerOrders } from "@/lib/gateway/orders";
import {
  DiscountType,
  type ViewVoucher,
  VoucherKind,
  listMyVouchers,
} from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";

import { DismissButton } from "./DismissButton";
import { getAccountAgeMs } from "./accountAge";

const HOUR_MS = 60 * 60 * 1000;
const WINDOW_MS = 14 * 24 * HOUR_MS;
const READ_TIMEOUT_MS = 2000;
const BANNER_COOKIE = "agora_welcome_dismissed";
const CHECKLIST_COOKIE = "agora_checklist_dismissed";
const PENDING_TEXT = "Voucher đang được chuẩn bị";

function withTimeout<T>(read: Promise<T>, fallback: T): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<T>((resolve) => {
    timer = setTimeout(() => resolve(fallback), READ_TIMEOUT_MS);
  });
  return Promise.race([read.catch(() => fallback), timeout]).finally(() =>
    clearTimeout(timer),
  );
}

function expired(endsAt: string): boolean {
  const m = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/.exec(endsAt);
  if (!m) return false;
  const end = new Date(
    Number(m[3]),
    Number(m[2]) - 1,
    Number(m[1]),
    23,
    59,
    59,
  );
  return end.getTime() < Date.now();
}

function usedUp(v: ViewVoucher): boolean {
  return v.quota > 0 && v.used >= v.quota;
}

function amountText(v: ViewVoucher): string {
  return v.discountType === DiscountType.PERCENT
    ? `Giảm ${v.discountValue}%`
    : `Giảm ${formatPrice(v.discountValue)}`;
}

function Banner({
  voucher,
  pending,
}: {
  voucher?: ViewVoucher;
  pending: boolean;
}) {
  return (
    <div
      data-testid="welcome-banner"
      className="flex items-start justify-between gap-4 rounded-2xl bg-primary-50 p-4"
    >
      <div className="space-y-1">
        <p className="text-sm font-semibold text-text-primary">
          Chào mừng bạn đến với Agora
        </p>
        {voucher ? (
          <>
            <p className="text-base font-bold text-action-primary">
              {voucher.code}
            </p>
            <p className="text-sm text-text-secondary">{amountText(voucher)}</p>
            {voucher.endsAt && (
              <p className="text-xs text-text-secondary">
                HSD: {voucher.endsAt}
              </p>
            )}
          </>
        ) : (
          pending && (
            <p className="text-sm text-text-secondary">{PENDING_TEXT}</p>
          )
        )}
      </div>
      <DismissButton
        part="banner"
        label="Đóng"
        className="text-sm text-text-secondary"
      />
    </div>
  );
}

export async function WelcomePanel() {
  if (!getPrincipal()) return null;

  const jar = await cookies();
  const bannerDismissed = jar.get(BANNER_COOKIE) !== undefined;
  const checklistDismissed = jar.get(CHECKLIST_COOKIE) !== undefined;

  const [age, mine, followed, addresses, orders] = await Promise.all([
    withTimeout(getAccountAgeMs(), null),
    withTimeout<Awaited<ReturnType<typeof listMyVouchers>>>(listMyVouchers(), {
      ok: false,
    }),
    withTimeout<unknown[] | null>(listFollowedSellers(), null),
    withTimeout<unknown[] | null>(listAddresses(), null),
    withTimeout<{ status: OrderStatus }[] | null>(listBuyerOrders(), null),
  ]);

  if (age === null || age >= WINDOW_MS) return null;

  let showBanner = false;
  let voucher: ViewVoucher | undefined;
  let pending = false;
  if (!bannerDismissed && mine.ok) {
    const newUser = mine.vouchers.filter(
      (v) => v.kind === VoucherKind.NEW_USER,
    );
    if (newUser.length === 0) {
      pending = age < HOUR_MS;
      showBanner = pending;
    } else {
      voucher = newUser.find((v) => !expired(v.endsAt) && !usedUp(v));
      showBanner = voucher !== undefined;
    }
  }

  const steps = [
    {
      key: "follow",
      label: "Theo dõi một shop",
      href: "/account/following",
      done: (followed?.length ?? 0) > 0,
    },
    {
      key: "address",
      label: "Thêm địa chỉ giao hàng",
      href: "/account/addresses",
      done: (addresses?.length ?? 0) > 0,
    },
    {
      key: "order",
      label: "Đặt đơn đầu tiên",
      href: "/",
      done: (orders ?? []).some((o) => o.status !== OrderStatus.CANCELLED),
    },
  ];
  const showChecklist = !checklistDismissed && steps.some((s) => !s.done);

  if (!showBanner && !showChecklist) return null;

  return (
    <section aria-label="Chào mừng" className="space-y-3">
      {showBanner && <Banner voucher={voucher} pending={pending} />}
      {showChecklist && (
        <div
          data-testid="welcome-checklist"
          className="space-y-2 rounded-2xl border border-border-subtle p-4"
        >
          <ul className="space-y-1">
            {steps.map((s) => (
              <li
                key={s.key}
                data-testid="welcome-step"
                data-done={s.done ? "true" : "false"}
                className="text-sm"
              >
                {s.done ? (
                  <span className="text-text-secondary line-through">
                    {s.label}
                  </span>
                ) : (
                  <a href={s.href} className="text-action-primary underline">
                    {s.label}
                  </a>
                )}
              </li>
            ))}
          </ul>
          <DismissButton
            part="checklist"
            label="Ẩn danh sách"
            className="text-xs text-text-secondary"
          />
        </div>
      )}
    </section>
  );
}
