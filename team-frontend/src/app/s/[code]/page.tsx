import { redirect } from "next/navigation";

import { resolveShareLink } from "@/lib/gateway/sharing";

export const dynamic = "force-dynamic";

// Resolver for short share links (/s/<code>): looks up the target and redirects,
// incrementing the link's click_count as a side-effect of ResolveShareLink.
export default async function ShareRedirectPage({
  params,
}: {
  params: { code: string };
}) {
  const link = await resolveShareLink(params.code);
  if (!link) redirect("/");

  switch (link.targetType) {
    case "listing":
      redirect(`/listing/${link.targetId}`);
    case "shop":
    case "seller":
      redirect(`/shop/${link.targetId}`);
    default:
      redirect("/");
  }
}
