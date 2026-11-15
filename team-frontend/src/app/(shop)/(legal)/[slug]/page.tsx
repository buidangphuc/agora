import { notFound } from "next/navigation";

import { LEGAL_SLUGS, readLegalPage } from "@/features/legal/content";

export const dynamicParams = false;

export function generateStaticParams() {
  return LEGAL_SLUGS.map((slug) => ({ slug }));
}

export default async function LegalPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const page = readLegalPage(slug);
  if (!page) notFound();
  return (
    <article className="mx-auto max-w-3xl space-y-4">
      <h1 className="text-2xl font-bold">{page.title}</h1>
      <p className="text-sm text-text-secondary">Phiên bản: {page.version}</p>
      <div className="whitespace-pre-line">{page.body}</div>
    </article>
  );
}
