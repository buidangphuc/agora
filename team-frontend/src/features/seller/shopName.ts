export const SHOP_NAME_MAX = 80;

/**
 * Shop display-name rule (team-domain: trimmed, 1-80 characters, no control
 * characters). Returns the error text, or null when valid.
 */
export function validateShopName(value: string): string | null {
  const name = value.trim();
  if (name === "") return "Nhập tên gian hàng.";
  if ([...name].length > SHOP_NAME_MAX) {
    return `Tên gian hàng tối đa ${SHOP_NAME_MAX} ký tự.`;
  }
  const hasControl = [...name].some((ch) => {
    const code = ch.codePointAt(0) ?? 0;
    return code < 32 || code === 127;
  });
  if (hasControl) {
    return "Tên gian hàng không được chứa ký tự điều khiển.";
  }
  return null;
}
