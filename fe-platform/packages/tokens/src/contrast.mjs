/** @type {Record<string, number>} */
export const THRESHOLD = { normal: 4.5, large: 3, ui: 3 };

/** @param {string} hex */
function channels(hex) {
  const h = hex.replace("#", "");
  return [0, 2, 4].map((i) => Number.parseInt(h.slice(i, i + 2), 16) / 255);
}

/** @param {number} c */
function linear(c) {
  return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/** @param {string} hex */
export function luminance(hex) {
  const [r, g, b] = channels(hex).map(linear);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/**
 * @param {string} a
 * @param {string} b
 */
export function contrastRatio(a, b) {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/**
 * @param {{ fg: string, bg: string, size: string }} pair
 * @param {Record<string, string>} values
 */
export function checkPair(pair, values) {
  const fg = values[pair.fg];
  const bg = values[pair.bg];
  if (!fg || !bg)
    throw new Error(`unknown token in pair ${pair.fg}/${pair.bg}`);
  const ratio = contrastRatio(fg, bg);
  const min = THRESHOLD[pair.size];
  return { ...pair, ratio, min, pass: ratio >= min };
}
