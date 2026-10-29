/** @param {string} hex */
export function luminance(hex: string): number;
/**
 * @param {string} a
 * @param {string} b
 */
export function contrastRatio(a: string, b: string): number;
/**
 * @param {{ fg: string, bg: string, size: string }} pair
 * @param {Record<string, string>} values
 */
export function checkPair(pair: {
    fg: string;
    bg: string;
    size: string;
}, values: Record<string, string>): {
    ratio: number;
    min: number;
    pass: boolean;
    fg: string;
    bg: string;
    size: string;
};
/** @type {Record<string, number>} */
export const THRESHOLD: Record<string, number>;
