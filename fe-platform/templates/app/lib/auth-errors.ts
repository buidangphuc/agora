export function authErrorCode(kind: string): string {
  if (kind === "forbidden") return "blocked";
  if (kind === "unavailable" || kind === "internal") return "unavailable";
  return "credentials";
}
