import { Timestamp } from "@bufbuild/protobuf";
import {
  Code,
  ConnectError,
  createPromiseClient,
  createRouterTransport,
} from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { VoucherService } from "@/generated/platform/promotion/v1/promotion_connect.js";
import {
  type CreateVoucherRequest,
  DiscountType,
  type GetVoucherRequest,
  type ListVouchersRequest,
  Voucher,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import { makeClients } from "./client.js";
import {
  createPlatformVoucher,
  getVoucher,
  listPlatformVouchers,
} from "./vouchers.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({ getToken: vi.fn(() => "test-token") }));

type Impl = {
  listVouchers?: (req: ListVouchersRequest) => unknown;
  getVoucher?: (req: GetVoucherRequest) => unknown;
  createVoucher?: (req: CreateVoucherRequest) => unknown;
};

const unimplemented = () => {
  throw new ConnectError("not used", Code.Unimplemented);
};

function useFakeGateway(impl: Impl) {
  const transport = createRouterTransport(({ service }) => {
    service(VoucherService, {
      listVouchers: (req: ListVouchersRequest) =>
        (impl.listVouchers ?? unimplemented)(req),
      getVoucher: (req: GetVoucherRequest) =>
        (impl.getVoucher ?? unimplemented)(req),
      createVoucher: (req: CreateVoucherRequest) =>
        (impl.createVoucher ?? unimplemented)(req),
    } as never);
  });
  vi.mocked(makeClients).mockReturnValue({
    voucher: createPromiseClient(VoucherService, transport),
  } as never);
}

const STARTS = new Date("2026-11-01T00:00:00.000Z");
const ENDS = new Date("2026-11-30T17:00:00.000Z");

const pbVoucher = (over: Partial<Voucher> = {}) =>
  new Voucher({
    id: "v-1",
    code: "SALE11",
    scope: VoucherScope.PLATFORM,
    discountType: DiscountType.PERCENT,
    discountValue: 10n,
    minSpend: 100000n,
    maxDiscount: 50000n,
    quota: 1000n,
    used: 12n,
    startsAt: Timestamp.fromDate(STARTS),
    endsAt: Timestamp.fromDate(ENDS),
    ...over,
  });

const createInput = {
  code: "SALE11",
  discountType: "percent" as const,
  discountValue: 10,
  minSpend: 100000,
  maxDiscount: 50000,
  quota: 1000,
  startsAt: STARTS,
  endsAt: ENDS,
};

const LEAK = "audit at 10.0.0.7:50066 refused";

beforeEach(() => vi.clearAllMocks());

describe("listPlatformVouchers over a fake transport", () => {
  it("asks for platform vouchers with an empty seller id and page size 20", async () => {
    const seen: ListVouchersRequest[] = [];
    useFakeGateway({
      listVouchers: (req) => {
        seen.push(req);
        return { vouchers: [pbVoucher()], page: { nextCursor: "c2" } };
      },
    });
    const res = await listPlatformVouchers("c1");
    expect(seen[0].sellerId).toBe("");
    expect(seen[0].page?.cursor).toBe("c1");
    expect(seen[0].page?.pageSize).toBe(20);
    expect(res).toEqual({
      status: "ok",
      nextCursor: "c2",
      vouchers: [
        {
          id: "v-1",
          code: "SALE11",
          scope: "platform",
          sellerId: "",
          discountType: "percent",
          discountValue: 10,
          minSpend: 100000,
          maxDiscount: 50000,
          quota: 1000,
          used: 12,
          startsAt: STARTS.toISOString(),
          endsAt: ENDS.toISOString(),
        },
      ],
    });
  });

  it("maps an empty page and missing times", async () => {
    useFakeGateway({
      listVouchers: () => ({
        vouchers: [pbVoucher({ startsAt: undefined, endsAt: undefined })],
      }),
    });
    const res = await listPlatformVouchers("");
    expect(res.status).toBe("ok");
    if (res.status !== "ok") return;
    expect(res.nextCursor).toBe("");
    expect(res.vouchers[0].startsAt).toBe("");
    expect(res.vouchers[0].endsAt).toBe("");
  });

  it.each([
    [Code.Unavailable, "unavailable"],
    [Code.PermissionDenied, "forbidden"],
    [Code.Unauthenticated, "unauthenticated"],
    [Code.DeadlineExceeded, "timeout"],
    [Code.Internal, "failed"],
  ])("maps error code %s to %s without the message", async (code, status) => {
    useFakeGateway({
      listVouchers: () => {
        throw new ConnectError(LEAK, code);
      },
    });
    const res = await listPlatformVouchers("");
    expect(res).toEqual({ status });
    expect(JSON.stringify(res)).not.toContain("10.0.0.7");
  });
});

describe("getVoucher over a fake transport", () => {
  it("reads one voucher by code", async () => {
    const seen: GetVoucherRequest[] = [];
    useFakeGateway({
      getVoucher: (req) => {
        seen.push(req);
        return { voucher: pbVoucher({ code: req.code }) };
      },
    });
    const res = await getVoucher("SALE11");
    expect(seen[0].code).toBe("SALE11");
    expect(res.status).toBe("ok");
    if (res.status === "ok") expect(res.voucher.code).toBe("SALE11");
  });

  it("maps NotFound and an empty response to not_found", async () => {
    useFakeGateway({
      getVoucher: () => {
        throw new ConnectError("voucher not found", Code.NotFound);
      },
    });
    expect(await getVoucher("NOPE")).toEqual({ status: "not_found" });
    useFakeGateway({ getVoucher: () => ({}) });
    expect(await getVoucher("NOPE")).toEqual({ status: "not_found" });
  });

  it("maps Unavailable to unavailable", async () => {
    useFakeGateway({
      getVoucher: () => {
        throw new ConnectError(LEAK, Code.Unavailable);
      },
    });
    expect(await getVoucher("SALE11")).toEqual({ status: "unavailable" });
  });
});

describe("createPlatformVoucher over a fake transport", () => {
  it("sends scope PLATFORM with the typed amounts and times, and returns the stored voucher", async () => {
    const seen: CreateVoucherRequest[] = [];
    useFakeGateway({
      createVoucher: (req) => {
        seen.push(req);
        return { voucher: pbVoucher({ id: "v-new", used: 0n }) };
      },
    });
    const res = await createPlatformVoucher(createInput);
    const req = seen[0];
    expect(req.code).toBe("SALE11");
    expect(req.scope).toBe(VoucherScope.PLATFORM);
    expect(req.discountType).toBe(DiscountType.PERCENT);
    expect(req.discountValue).toBe(10n);
    expect(req.minSpend).toBe(100000n);
    expect(req.maxDiscount).toBe(50000n);
    expect(req.quota).toBe(1000n);
    expect(req.startsAt?.toDate().toISOString()).toBe(STARTS.toISOString());
    expect(req.endsAt?.toDate().toISOString()).toBe(ENDS.toISOString());
    expect(res.status).toBe("ok");
    if (res.status === "ok") {
      expect(res.voucher.id).toBe("v-new");
      expect(res.voucher.used).toBe(0);
    }
  });

  it("sends a fixed discount as FIXED", async () => {
    const seen: CreateVoucherRequest[] = [];
    useFakeGateway({
      createVoucher: (req) => {
        seen.push(req);
        return { voucher: pbVoucher({ discountType: DiscountType.FIXED }) };
      },
    });
    await createPlatformVoucher({
      ...createInput,
      discountType: "fixed",
      discountValue: 20000,
      maxDiscount: 0,
    });
    expect(seen[0].discountType).toBe(DiscountType.FIXED);
    expect(seen[0].discountValue).toBe(20000n);
  });

  it("reports audit-down (Unavailable) as unavailable without the backend message", async () => {
    useFakeGateway({
      createVoucher: () => {
        throw new ConnectError(LEAK, Code.Unavailable);
      },
    });
    const res = await createPlatformVoucher(createInput);
    expect(res).toEqual({ status: "unavailable" });
    expect(JSON.stringify(res)).not.toContain("audit at");
  });

  it.each([
    [Code.InvalidArgument, "invalid"],
    [Code.PermissionDenied, "forbidden"],
    [Code.Unauthenticated, "unauthenticated"],
    [Code.DeadlineExceeded, "timeout"],
    [Code.Internal, "failed"],
  ])("maps error code %s to %s", async (code, status) => {
    useFakeGateway({
      createVoucher: () => {
        throw new ConnectError(LEAK, code);
      },
    });
    expect(await createPlatformVoucher(createInput)).toEqual({ status });
  });

  it("treats a response without a voucher as failed", async () => {
    useFakeGateway({ createVoucher: () => ({}) });
    expect(await createPlatformVoucher(createInput)).toEqual({
      status: "failed",
    });
  });
});
