import { notFound } from "next/navigation";

import { Catalogue } from "./Catalogue";

// Evaluated per request so the environment gate is never frozen at build time.
export const dynamic = "force-dynamic";

export const metadata = { title: "UI catalogue" };

/** Development-only component catalogue; a 404 in production builds. */
export default function UiCataloguePage() {
  if (process.env.NODE_ENV === "production") notFound();
  return <Catalogue />;
}
