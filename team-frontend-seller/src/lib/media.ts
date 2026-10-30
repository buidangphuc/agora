const MEDIA_BASE_URL =
  process.env.NEXT_PUBLIC_MEDIA_BASE_URL ||
  "http://localhost:9000/listing-images";

/**
 * Resolves a stored image key to a full public URL for rendering.
 */
export function getImageUrl(key: string): string {
  if (!key) return "";
  if (key.startsWith("http://") || key.startsWith("https://")) return key;
  return `${MEDIA_BASE_URL.replace(/\/$/, "")}/${key}`;
}
