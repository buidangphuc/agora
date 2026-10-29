import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { toCss } from "../src/resolve.mjs";

const out = join(dirname(fileURLToPath(import.meta.url)), "..", "dist");
mkdirSync(out, { recursive: true });
writeFileSync(join(out, "tokens.css"), toCss());
