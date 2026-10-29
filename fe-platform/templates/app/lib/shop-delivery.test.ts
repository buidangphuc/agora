import { describe, expect, it } from "vitest";
import {
  parseDeliveryCookie,
  parseErrorFields,
  validateDelivery,
} from "./shop-delivery";

const good = {
  name: "An Nguyen",
  address: "1 Main Street",
  city: "HCMC",
  phone: "+84 900 000 000",
};
const getter = (o: Record<string, string>) => (k: string) => o[k];

describe("validateDelivery", () => {
  it("accepts a complete form and trims values", () => {
    const r = validateDelivery(getter({ ...good, name: "  An Nguyen " }));
    expect(r).toEqual({ ok: true, value: good });
  });

  it("reports every invalid field", () => {
    const r = validateDelivery(
      getter({ name: "", address: "x", city: "", phone: "abc" }),
    );
    expect(r).toEqual({
      ok: false,
      fields: ["name", "address", "city", "phone"],
    });
  });

  it("rejects a phone with too few digits", () => {
    const r = validateDelivery(getter({ ...good, phone: "12-34" }));
    expect(r).toEqual({ ok: false, fields: ["phone"] });
  });

  it("treats missing and non-string values as empty", () => {
    const r = validateDelivery(() => undefined);
    expect(r.ok).toBe(false);
  });
});

describe("cookie and error params", () => {
  it("round-trips a valid cookie and drops a tampered one", () => {
    expect(parseDeliveryCookie(JSON.stringify(good))).toEqual(good);
    expect(parseDeliveryCookie("not json")).toBeNull();
    expect(parseDeliveryCookie(JSON.stringify({ name: "x" }))).toBeNull();
    expect(parseDeliveryCookie(undefined)).toBeNull();
  });

  it("parses only known error field codes", () => {
    expect(parseErrorFields("phone,evil,name")).toEqual(["name", "phone"]);
    expect(parseErrorFields(undefined)).toEqual([]);
  });
});
