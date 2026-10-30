import { range } from "@/components/ui/range";

/**
 * Route loading skeletons for the account screens. Each one reserves the same
 * footprint as the page it stands in for (shell header, menu, card heights),
 * so content replaces it without layout shift.
 */

const block = "animate-pulse bg-neutral-200";

function CardBlock({ height }: { height: string }) {
  return <div className={`w-full rounded-xl ${block} ${height}`} />;
}

function Header() {
  return (
    <>
      <div className={`h-4 w-40 rounded-xs ${block}`} />
      <div className="space-y-1">
        <div className={`h-8 w-1/3 rounded-lg ${block}`} />
        <div className={`h-5 w-2/3 rounded-xs ${block}`} />
      </div>
    </>
  );
}

/** Shell + menu + `sections` card placeholders (the five /account/* settings pages). */
export function AccountPageSkeleton({
  sections = 2,
}: {
  sections?: number;
}) {
  return (
    <div
      className="space-y-4 py-2"
      aria-busy="true"
      aria-live="polite"
      data-testid="page-skeleton"
    >
      <Header />
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:gap-6">
        <div className="flex gap-1 overflow-hidden lg:w-60 lg:shrink-0 lg:flex-col">
          {range(8).map((i) => (
            <div
              key={i}
              className={`h-10 w-24 shrink-0 rounded-lg lg:w-full ${block}`}
            />
          ))}
        </div>
        <div className="min-w-0 max-w-3xl flex-1 space-y-4">
          {range(sections).map((i) => (
            <CardBlock key={i} height="h-48" />
          ))}
        </div>
      </div>
    </div>
  );
}

/** Standalone wide pages (/favorites, /notifications): header card + blocks. */
export function StandalonePageSkeleton({ cards = 3 }: { cards?: number }) {
  return (
    <div
      className="space-y-4 py-2"
      aria-busy="true"
      aria-live="polite"
      data-testid="page-skeleton"
    >
      <Header />
      {range(cards).map((i) => (
        <CardBlock key={i} height="h-40" />
      ))}
    </div>
  );
}

/** Centred auth card (/login, /register). */
export function AuthCardSkeleton() {
  return (
    <div
      className="mx-auto w-full max-w-sm space-y-4 py-2"
      aria-busy="true"
      aria-live="polite"
      data-testid="page-skeleton"
    >
      <div className={`mx-auto h-8 w-1/2 rounded-lg ${block}`} />
      <CardBlock height="h-96" />
    </div>
  );
}
