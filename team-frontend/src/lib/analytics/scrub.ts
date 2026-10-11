const reEmail = /[\w.+-]+@[\w-]+(\.[\w-]+)+/g;
const rePhone = /(?:\+84|\b0)(?:[\s.]?\d){9}\b/g;
const reLongDigit = /\d{13,}/g;

/** Mask emails, VN phone numbers and long digit runs (mirrors gateway scrubText). */
export function scrubText(s: string): string {
  return s
    .replace(reEmail, "[email]")
    .replace(rePhone, "[phone]")
    .replace(reLongDigit, "[number]");
}

/** Reduce a referrer to scheme://host/path; anything unparseable becomes "". */
export function scrubReferrer(ref: string): string {
  if (!ref) return "";
  try {
    const u = new URL(ref);
    if (!u.protocol || !u.host) return "";
    return `${u.protocol}//${u.host}${u.pathname}`;
  } catch {
    return "";
  }
}
