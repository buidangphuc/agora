export const DELIVERY_COOKIE = "shop_delivery";

export type Delivery = {
  name: string;
  address: string;
  city: string;
  phone: string;
};

export type DeliveryField = keyof Delivery;

export type DeliveryResult =
  | { ok: true; value: Delivery }
  | { ok: false; fields: DeliveryField[] };

const FIELDS: DeliveryField[] = ["name", "address", "city", "phone"];

function clean(v: FormDataEntryValue | null | undefined): string {
  return typeof v === "string" ? v.trim() : "";
}

export function validateDelivery(
  get: (key: string) => FormDataEntryValue | null | undefined,
): DeliveryResult {
  const value: Delivery = {
    name: clean(get("name")),
    address: clean(get("address")),
    city: clean(get("city")),
    phone: clean(get("phone")),
  };
  const digits = value.phone.replace(/\D/g, "");
  const bad: Record<DeliveryField, boolean> = {
    name: value.name.length < 2,
    address: value.address.length < 5,
    city: value.city.length < 2,
    phone: !/^[0-9+()\-\s]+$/.test(value.phone) || digits.length < 8,
  };
  const fields = FIELDS.filter((f) => bad[f]);
  return fields.length === 0 ? { ok: true, value } : { ok: false, fields };
}

export function parseDeliveryCookie(raw: string | undefined): Delivery | null {
  if (!raw) return null;
  try {
    const data = JSON.parse(raw) as Partial<Delivery>;
    const result = validateDelivery((k) => data[k as DeliveryField]);
    return result.ok ? result.value : null;
  } catch {
    return null;
  }
}

export function parseErrorFields(
  raw: string | string[] | undefined,
): DeliveryField[] {
  const text = Array.isArray(raw) ? raw[0] : raw;
  return FIELDS.filter((f) => (text ?? "").split(",").includes(f));
}
