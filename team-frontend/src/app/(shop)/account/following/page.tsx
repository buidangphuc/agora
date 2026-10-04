import Link from "next/link";
import { redirect } from "next/navigation";
import { Suspense } from "react";

import { Avatar } from "@/components/ui/Avatar";
import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Statistic } from "@/components/ui/Statistic";
import { Tabs } from "@/components/ui/Tabs";
import { focusRing } from "@/components/ui/focus";
import { AccountShell } from "@/features/account/AccountShell";
import {
  FollowedFeed,
  FollowedFeedSkeleton,
} from "@/features/account/FollowedFeed";
import { LinkButton } from "@/features/account/LinkButton";
import { listFollowedSellers } from "@/lib/gateway/engagement";
import { getPrincipal } from "@/lib/gateway/session";
import { shopLabel } from "@/lib/gateway/shops";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Shop đang theo dõi | Marketplace",
};

type FollowingTab = "shops" | "items";

function parseTab(value: string | undefined): FollowingTab {
  return value === "items" ? "items" : "shops";
}

export default async function FollowingPage({
  searchParams,
}: {
  searchParams?: { tab?: string };
}) {
  if (!getPrincipal()) redirect("/login");

  const tab = parseTab(searchParams?.tab);
  const followed = await listFollowedSellers();

  return (
    <AccountShell
      current="following"
      title="Shop đang theo dõi"
      description="Các gian hàng và sản phẩm bạn đang theo dõi."
    >
      <Statistic title="Gian hàng đang theo dõi" value={followed.length} />
      <Tabs
        items={[
          { id: "shops", label: "Gian hàng", badge: followed.length },
          { id: "items", label: "Sản phẩm" },
        ]}
        activeId={tab}
        hrefFor={(id) => `/account/following?tab=${id}`}
      />

      {tab === "shops" ? (
        followed.length === 0 ? (
          <Card>
            <Empty
              description="Bạn chưa theo dõi gian hàng nào."
              action={
                <LinkButton href="/search">Khám phá gian hàng</LinkButton>
              }
            />
          </Card>
        ) : (
          <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {followed.map(({ sellerId, displayName }) => {
              const label = shopLabel(sellerId, displayName);
              return (
                <li key={sellerId}>
                  <Link
                    href={`/shop/${sellerId}`}
                    className={`block rounded-xl ${focusRing}`}
                  >
                    <Card
                      hoverable
                      className="flex min-h-18 items-center gap-3 p-4"
                    >
                      <Avatar name={label} />
                      <span className="min-w-0 truncate text-sm font-medium text-text-primary">
                        {label}
                      </span>
                    </Card>
                  </Link>
                </li>
              );
            })}
          </ul>
        )
      ) : (
        <Suspense fallback={<FollowedFeedSkeleton />}>
          <FollowedFeed sellerIds={followed.map((f) => f.sellerId)} />
        </Suspense>
      )}
    </AccountShell>
  );
}
