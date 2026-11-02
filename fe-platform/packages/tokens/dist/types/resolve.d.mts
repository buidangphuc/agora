/**
 * @param {string} theme
 * @returns {Record<string, string>}
 */
export function resolveTheme(theme: string): Record<string, string>;
export function semanticNames(): string[];
/** Tailwind colour value for a semantic name that accepts an opacity modifier (`bg-surface-card/80`). */
export function colorVar(name: string): string;
export function px(n: number): string;
/**
 * @param {{ x: number, y: number, blur: number, spread: number, color: string }[]} layers
 */
export function shadowToCss(layers: {
    x: number;
    y: number;
    blur: number;
    spread: number;
    color: string;
}[]): string;
/**
 * @param {string} theme
 * @returns {Record<string, string>}
 */
export function resolveShadows(theme: string): Record<string, string>;
export function shadowNames(): string[];
export function toCss(): string;
export function toJson(): string;
export const THEMES: string[];
