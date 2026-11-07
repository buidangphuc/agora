import type { Metadata } from "next";

const DEFAULT_SITE_URL = "http://localhost:3000";
const DESCRIPTION_MAX = 160;

export const SITE_NAME = "Agora";

export function siteUrl(): string {
  const raw = process.env.NEXT_PUBLIC_SITE_URL || DEFAULT_SITE_URL;
  return raw.replace(/\/+$/, "");
}

export function absoluteUrl(path: string): string {
  return `${siteUrl()}${path.startsWith("/") ? path : `/${path}`}`;
}

function clip(text: string): string {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length <= DESCRIPTION_MAX
    ? flat
    : flat.slice(0, DESCRIPTION_MAX).trimEnd();
}

export function pageMetadata(input: {
  title: string;
  description: string;
  path: string;
  type?: "website" | "product";
  image?: string;
}): Metadata {
  const description = clip(input.description);
  const url = absoluteUrl(input.path);
  return {
    title: input.title,
    description,
    alternates: { canonical: url },
    openGraph: {
      type: (input.type ?? "website") as "website",
      siteName: SITE_NAME,
      title: input.title,
      description,
      url,
      ...(input.image ? { images: [input.image] } : {}),
    },
  };
}
