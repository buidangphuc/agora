import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { primitives, semanticNames } from "@fe-platform/tokens";
import ts from "typescript";
import { isMain, root } from "./lib.mjs";

const uiRoot = join(root, "packages", "ui");
const entryFile = join(uiRoot, "src", "index.ts");
const outFile = join(root, "docs", "agents", "catalog.json");

function loadOptions(tsconfigPath) {
  const raw = ts.readConfigFile(tsconfigPath, ts.sys.readFile);
  return ts.parseJsonConfigFileContent(raw.config, ts.sys, uiRoot).options;
}

function propsFrom(node, checker, out) {
  if (ts.isParenthesizedTypeNode(node))
    return propsFrom(node.type, checker, out);
  if (ts.isIntersectionTypeNode(node)) {
    for (const t of node.types) propsFrom(t, checker, out);
    return;
  }
  if (ts.isTypeLiteralNode(node)) {
    for (const m of node.members) {
      if (!ts.isPropertySignature(m) || !m.type) continue;
      out.props.push({
        name: m.name.getText(),
        required: !m.questionToken,
        type: checker.typeToString(
          checker.getTypeAtLocation(m.type),
          undefined,
          ts.TypeFormatFlags.NoTruncation,
        ),
      });
    }
    return;
  }
  if (ts.isTypeReferenceNode(node)) {
    const name = node.typeName.getText();
    if (name === "NativeProps") {
      const arg = node.typeArguments?.[0];
      out.native = arg ? arg.getText().replaceAll('"', "") : undefined;
      return;
    }
    const symbol = checker.getSymbolAtLocation(node.typeName);
    const decl = symbol?.declarations?.find(ts.isTypeAliasDeclaration);
    if (decl) propsFrom(decl.type, checker, out);
  }
}

function describe(symbol, checker) {
  const target =
    symbol.flags & ts.SymbolFlags.Alias
      ? checker.getAliasedSymbol(symbol)
      : symbol;
  const decl = target.declarations?.find(
    (d) => ts.isFunctionDeclaration(d) || ts.isVariableDeclaration(d),
  );
  if (!decl) return null;
  const purpose = ts
    .displayPartsToString(target.getDocumentationComment(checker))
    .replace(/\s+/g, " ")
    .trim();
  const source = decl.getSourceFile().text;
  const entry = {
    name: symbol.getName(),
    kind: symbol.getName().startsWith("use") ? "hook" : "component",
    purpose,
    client: /^\s*["']use client["']/.test(source),
    props: [],
  };
  const tag = target.getJsDocTags(checker).find((t) => t.name === "deprecated");
  if (tag) {
    entry.deprecated =
      ts.displayPartsToString(tag.text).replace(/\s+/g, " ").trim() ||
      "deprecated";
  }
  const fn = ts.isFunctionDeclaration(decl) ? decl : undefined;
  const param = fn?.parameters[0];
  if (param?.type) propsFrom(param.type, checker, entry);
  return entry;
}

export function collectComponents(entry, options) {
  const program = ts.createProgram([entry], options);
  const checker = program.getTypeChecker();
  const source = program.getSourceFile(entry);
  const moduleSymbol = checker.getSymbolAtLocation(source);
  const exported = checker.getExportsOfModule(moduleSymbol);
  const found = [];
  for (const symbol of exported) {
    const entryItem = describe(symbol, checker);
    if (!entryItem) continue;
    if (!entryItem.purpose) {
      throw new Error(
        `catalog: export "${entryItem.name}" has no JSDoc purpose line`,
      );
    }
    found.push(entryItem);
  }
  return found.sort((a, b) => a.name.localeCompare(b.name));
}

export function buildCatalog() {
  const options = loadOptions(join(uiRoot, "tsconfig.json"));
  return {
    components: collectComponents(entryFile, options),
    tokens: {
      colors: [...semanticNames()].sort(),
      fontSize: Object.keys(primitives.fontSize),
      radius: Object.keys(primitives.radius),
      space: Object.keys(primitives.space),
    },
  };
}

export function render(catalog) {
  return `${JSON.stringify(catalog, null, 2)}\n`;
}

export function catalogDiff() {
  const fresh = render(buildCatalog());
  let committed = "";
  try {
    committed = readFileSync(outFile, "utf8");
  } catch {
    return ["docs/agents/catalog.json is missing"];
  }
  return fresh === committed ? [] : ["docs/agents/catalog.json is out of date"];
}

if (isMain(import.meta.url)) {
  if (process.argv.includes("--check")) {
    const problems = catalogDiff();
    for (const p of problems) console.log(p);
    console.log(`\ncatalog check: ${problems.length} problem(s)`);
    if (problems.length > 0) console.log("fix: npm run catalog, then commit");
    process.exit(problems.length > 0 ? 1 : 0);
  }
  writeFileSync(outFile, render(buildCatalog()));
  console.log(`wrote ${outFile}`);
}
