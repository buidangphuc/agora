export interface TokenViolation {
  line: number;
  token: string;
}
export function scanSource(text: string): TokenViolation[];
export function scanTree(
  srcDir: string,
  root?: string,
): (TokenViolation & { file: string })[];
