import { readFileSync } from "node:fs";
import { join } from "node:path";

export const LEGAL_SLUGS = [
  "dieu-khoan-su-dung",
  "chinh-sach-bao-mat",
  "quy-che-hoat-dong",
  "chinh-sach-doi-tra",
  "hang-cam-kinh-doanh",
  "giai-quyet-khieu-nai",
  "dieu-khoan-nguoi-ban",
] as const;

export type LegalSlug = (typeof LEGAL_SLUGS)[number];

export interface LegalPage {
  slug: LegalSlug;
  title: string;
  version: string;
  body: string;
}

const DIR = join(process.cwd(), "content/legal");

export function readLegalPage(slug: string): LegalPage | null {
  if (!(LEGAL_SLUGS as readonly string[]).includes(slug)) return null;
  let raw: string;
  try {
    raw = readFileSync(join(DIR, `${slug}.md`), "utf8");
  } catch {
    return null;
  }
  const m = raw.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/);
  if (!m) return null;
  const meta: Record<string, string> = {};
  for (const line of m[1].split(/\r?\n/)) {
    const i = line.indexOf(":");
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  if (!meta.title || !meta.version) return null;
  return {
    slug: slug as LegalSlug,
    title: meta.title,
    version: meta.version,
    body: m[2].trim(),
  };
}
