import { relative } from "node:path";
import { isMain, read, root, sourceFiles } from "./lib.mjs";

const IMPORT = /(?:\bfrom|\bimport)\s*\(?\s*["']([^"']+)["']/g;

export function scanImports(file, text) {
  const out = [];
  const path = file.replaceAll("\\", "/");
  const inPublic = path.includes("/(public)/");
  const inPackage = /(^|\/)packages\//.test(path);
  const inTemplate = /(^|\/)templates\//.test(path);
  const isClient = /^\s*["']use client["']/.test(text);
  const lines = text.split("\n");
  lines.forEach((line, i) => {
    for (const m of line.matchAll(IMPORT)) {
      const spec = m[1];
      if (
        inPublic &&
        (spec.includes("(internal)") || /(^|\/)internal(\/|$)/.test(spec))
      ) {
        out.push({ line: i + 1, rule: "public-imports-internal", spec });
      }
      if (
        inPackage &&
        (/(^|\/)templates\//.test(spec) || spec.startsWith("@fe-platform/app"))
      ) {
        out.push({ line: i + 1, rule: "package-imports-app", spec });
      }
      if (
        inTemplate &&
        (spec.includes("/generated/") ||
          spec.startsWith("@connectrpc/") ||
          spec.startsWith("@bufbuild/"))
      ) {
        out.push({ line: i + 1, rule: "app-imports-transport", spec });
      }
      if (
        isClient &&
        (spec === "@fe-platform/api-client" ||
          spec === "server-only" ||
          /(^|\/)lib\/session$/.test(spec))
      ) {
        out.push({ line: i + 1, rule: "client-imports-server-only", spec });
      }
    }
  });
  return out;
}

export function scanTree(files) {
  const found = [];
  for (const file of files) {
    for (const v of scanImports(file, read(file))) {
      found.push({ file: relative(root, file), ...v });
    }
  }
  return found;
}

if (isMain(import.meta.url)) {
  const violations = scanTree(sourceFiles());
  for (const v of violations)
    console.log(`${v.file}:${v.line}  ${v.rule}  ${v.spec}`);
  console.log(`\nboundary check: ${violations.length} violation(s)`);
  process.exit(violations.length > 0 ? 1 : 0);
}
