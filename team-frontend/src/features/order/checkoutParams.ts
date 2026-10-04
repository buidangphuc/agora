import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";

export const CHECKOUT_STEPS = [
  "address",
  "shipping",
  "payment",
  "confirm",
] as const;

export type CheckoutStep = (typeof CHECKOUT_STEPS)[number];

export const PAYMENT_METHODS: readonly PaymentMethod[] = [
  PaymentMethod.COD,
  PaymentMethod.MOCK_MOMO,
  PaymentMethod.MOCK_BANK,
  PaymentMethod.MOCK_CARD,
];

export type RawSearchParams = Record<string, string | string[] | undefined>;

export interface CheckoutState {
  step: CheckoutStep;
  /** Selected address id; only set when it exists in the buyer's list. */
  addr?: string;
  pay: PaymentMethod;
  voucher?: string;
}

export interface ResolvedCheckout extends CheckoutState {
  /** True when the URL asked for a step (or address) the state cannot satisfy. */
  needsRedirect: boolean;
}

function first(v: string | string[] | undefined): string | undefined {
  const value = Array.isArray(v) ? v[0] : v;
  return value === undefined || value === "" ? undefined : value;
}

function isStep(v: string | undefined): v is CheckoutStep {
  return CHECKOUT_STEPS.includes(v as CheckoutStep);
}

function parsePay(v: string | undefined): PaymentMethod {
  const n = Number(v);
  return PAYMENT_METHODS.includes(n) ? (n as PaymentMethod) : PaymentMethod.COD;
}

export interface AddressRef {
  id: string;
  isDefault?: boolean;
}

/**
 * Validate the checkout search params against the buyer's addresses.
 * - an unknown step becomes "address";
 * - every step after "address" needs a valid `addr`; a missing or stale one
 *   (deleted address) sends the buyer back to "address" (needsRedirect);
 * - on "address" the selection falls back to the default (else first) address
 *   so a stale `addr` never reaches the order.
 */
export function resolveCheckout(
  raw: RawSearchParams,
  addresses: readonly AddressRef[],
): ResolvedCheckout {
  const rawStep = first(raw.step);
  const requested: CheckoutStep = isStep(rawStep) ? rawStep : "address";
  const rawAddr = first(raw.addr);
  const valid = addresses.some((a) => a.id === rawAddr);
  const fallback = (addresses.find((a) => a.isDefault) ?? addresses[0])?.id;
  const pay = parsePay(first(raw.pay));
  const voucher = first(raw.voucher);

  if (requested !== "address" && !valid) {
    return {
      step: "address",
      addr: fallback,
      pay,
      voucher,
      needsRedirect: true,
    };
  }
  return {
    step: requested,
    addr: valid ? rawAddr : fallback,
    pay,
    voucher,
    needsRedirect: false,
  };
}

/** Canonical query string for a checkout state (only ids and a promo code). */
export function checkoutQuery(state: Partial<CheckoutState>): string {
  const q = new URLSearchParams();
  if (state.step) q.set("step", state.step);
  if (state.addr) q.set("addr", state.addr);
  if (state.pay !== undefined) q.set("pay", String(state.pay));
  if (state.voucher) q.set("voucher", state.voucher);
  return q.toString();
}

export function checkoutHref(state: Partial<CheckoutState>): string {
  return `/checkout?${checkoutQuery(state)}`;
}

export function stepIndex(step: CheckoutStep): number {
  return CHECKOUT_STEPS.indexOf(step);
}
