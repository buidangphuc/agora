export type StepState = "done" | "todo" | "unknown";

export interface GettingStartedStep {
  label: string;
  href: string;
  state: StepState;
}

export interface GettingStarted {
  steps: GettingStartedStep[];
  allDone: boolean;
}

type Page = { ok: boolean; items?: readonly unknown[] };

export interface GettingStartedSources {
  storefront: () => Promise<{ displayName: string; bannerUrl: string } | null>;
  listings: () => Promise<{ items: readonly unknown[] }>;
  vouchers: () => Promise<Page>;
  campaigns: () => Promise<Page>;
}

const TIMEOUT_MS = 3000;

type Settled<T> = { ok: true; value: T } | { ok: false };

function settle<T>(call: () => Promise<T>): Promise<Settled<T>> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => resolve({ ok: false }), TIMEOUT_MS);
    let p: Promise<T>;
    try {
      p = call();
    } catch {
      clearTimeout(timer);
      resolve({ ok: false });
      return;
    }
    p.then(
      (value) => {
        clearTimeout(timer);
        resolve({ ok: true, value });
      },
      () => {
        clearTimeout(timer);
        resolve({ ok: false });
      },
    );
  });
}

function promoState(a: Settled<Page>, b: Settled<Page>): StepState {
  const rows = (s: Settled<Page>) =>
    s.ok && s.value.ok && (s.value.items?.length ?? 0) > 0;
  if (rows(a) || rows(b)) return "done";
  const failed = (s: Settled<Page>) => !s.ok || !s.value.ok;
  return failed(a) || failed(b) ? "unknown" : "todo";
}

export async function deriveGettingStarted(
  src: GettingStartedSources,
): Promise<GettingStarted> {
  const [sf, ls, vs, cs] = await Promise.all([
    settle(src.storefront),
    settle(src.listings),
    settle(src.vouchers),
    settle(src.campaigns),
  ]);

  const field = (
    pick: (s: { displayName: string; bannerUrl: string }) => string,
  ): StepState => {
    if (!sf.ok) return "unknown";
    return sf.value && pick(sf.value).trim() !== "" ? "done" : "todo";
  };
  const listingState: StepState = !ls.ok
    ? "unknown"
    : ls.value.items.length > 0
      ? "done"
      : "todo";

  const steps: GettingStartedStep[] = [
    {
      label: "Đặt tên gian hàng",
      href: "/seller/shop",
      state: field((s) => s.displayName),
    },
    {
      label: "Thêm ảnh bìa gian hàng",
      href: "/seller/shop",
      state: field((s) => s.bannerUrl),
    },
    {
      label: "Đăng sản phẩm đầu tiên",
      href: "/seller/new",
      state: listingState,
    },
    {
      label: "Tạo voucher hoặc flash sale đầu tiên",
      href: "/seller/marketing",
      state: promoState(vs, cs),
    },
  ];
  return { steps, allDone: steps.every((s) => s.state === "done") };
}
