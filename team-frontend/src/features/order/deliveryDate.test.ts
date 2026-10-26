import { afterEach, describe, expect, it } from "vitest";

import { formatDeliveryDate } from "./deliveryDate";

const originalTz = process.env.TZ;

afterEach(() => {
  if (originalTz === undefined) Reflect.deleteProperty(process.env, "TZ");
  else process.env.TZ = originalTz;
});

describe("formatDeliveryDate", () => {
  it("formats dd/mm in Asia/Ho_Chi_Minh (Checkout coherence: the date is formatted in Ho Chi Minh time)", () => {
    expect(formatDeliveryDate("2026-10-17T18:30:00Z")).toBe("18/10");
    expect(formatDeliveryDate("2026-12-31T16:59:59Z")).toBe("31/12");
    expect(formatDeliveryDate("2026-10-18T10:00:00+07:00")).toBe("18/10");
  });

  it("is independent of the process time zone (Checkout coherence: the date is formatted in Ho Chi Minh time)", () => {
    for (const tz of ["UTC", "America/Los_Angeles", "Pacific/Kiritimati"]) {
      process.env.TZ = tz;
      expect(formatDeliveryDate("2026-10-17T18:30:00Z")).toBe("18/10");
      expect(formatDeliveryDate("2026-12-31T16:59:59Z")).toBe("31/12");
    }
  });

  it("rolls over to the next day and year at 17:00Z (Checkout coherence: the date is formatted in Ho Chi Minh time)", () => {
    expect(formatDeliveryDate("2026-12-31T17:00:00Z")).toBe("01/01");
    expect(formatDeliveryDate("2026-10-17T16:59:59Z")).toBe("17/10");
  });

  it("zero-pads day and month (Checkout coherence: the date is formatted in Ho Chi Minh time)", () => {
    expect(formatDeliveryDate("2026-03-04T03:00:00Z")).toBe("04/03");
  });

  it("returns no date for a null, empty or unparsable value (Checkout coherence: a failed or unset delivery date is not invented)", () => {
    expect(formatDeliveryDate(null)).toBeFalsy();
    expect(formatDeliveryDate(undefined)).toBeFalsy();
    expect(formatDeliveryDate("")).toBeFalsy();
    expect(formatDeliveryDate("not-a-date")).toBeFalsy();
  });
});
