import { PaymentMethod } from "@fe-platform/api-client";

const LABELS: Partial<Record<PaymentMethod, string>> = {
  [PaymentMethod.COD]: "Cash on delivery",
  [PaymentMethod.MOCK_MOMO]: "MoMo (test)",
  [PaymentMethod.MOCK_BANK]: "Bank transfer (test)",
  [PaymentMethod.MOCK_CARD]: "Card (test)",
};

export function paymentOptions(): { value: string; label: string }[] {
  return Object.values(PaymentMethod)
    .filter(
      (v): v is PaymentMethod =>
        typeof v === "number" && v !== PaymentMethod.UNSPECIFIED,
    )
    .map((v) => ({
      value: String(v),
      label: LABELS[v] ?? PaymentMethod[v] ?? String(v),
    }));
}

export function parsePaymentMethod(
  raw: FormDataEntryValue | null,
): PaymentMethod {
  const n = Number(raw);
  return Number.isInteger(n) && n in PaymentMethod
    ? (n as PaymentMethod)
    : PaymentMethod.UNSPECIFIED;
}
