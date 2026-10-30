export type CasesTab = "reports" | "queue";
export type CasesStatus = "OPEN" | "ACTIONED" | "DISMISSED";

export interface CasesParams {
  tab: CasesTab;
  status: CasesStatus;
  cursor: string;
}

type Raw = Record<string, string | string[] | undefined>;

const MAX_CURSOR = 256;

function first(v: string | string[] | undefined): string {
  const s = Array.isArray(v) ? v[0] : v;
  return typeof s === "string" ? s.trim() : "";
}

// Never throws: unknown tab or status fall back to reports / OPEN.
export function parseCasesParams(sp: Raw): CasesParams {
  const tab = first(sp.tab);
  const status = first(sp.status);
  return {
    tab: tab === "queue" ? "queue" : "reports",
    status: status === "ACTIONED" || status === "DISMISSED" ? status : "OPEN",
    cursor: first(sp.cursor).slice(0, MAX_CURSOR),
  };
}
const VN_OFFSET_MS = 7 * 60 * 60 * 1000;
const pad = (n: number) => String(n).padStart(2, "0");

// YYYY-MM-DD HH:mm:ss in UTC+07:00 (fixed, so server and browser agree).
export function formatVnSeconds(iso: string): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const d = new Date(t + VN_OFFSET_MS);
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}`;
}
