#!/usr/bin/env node
// Read dist/.vite/manifest.json and replace asset placeholders in SSR templates.
// dist/.vite/manifest.json を読み、SSR テンプレート内のプレースホルダーを置換する。

import { readFileSync, writeFileSync, mkdirSync, readdirSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const distDir = join(__dirname, "..", "dist");
const manifest = JSON.parse(
  readFileSync(join(distDir, ".vite", "manifest.json"), "utf-8")
);

// Find the entry point in manifest.
const entry = manifest["index.html"];
if (!entry) {
  console.error("inject-assets: index.html not found in manifest");
  process.exit(1);
}

const buildHash = Date.now().toString(36);
const jsPath = entry.file ? `/${entry.file}?v=${buildHash}` : "";
const cssPath = entry.css?.[0] ? `/${entry.css[0]}?v=${buildHash}` : "";

// Process templates — output to handler/templates/ for Go embed.
// テンプレートを handler/templates/ に出力 (Go embed 用)。
const srcDir = join(__dirname, "..", "templates");
const outDir = join(__dirname, "..", "..", "handler", "templates");
mkdirSync(outDir, { recursive: true });

let count = 0;
for (const file of readdirSync(srcDir)) {
  if (!file.endsWith(".tmpl.html")) continue;
  let content = readFileSync(join(srcDir, file), "utf-8");
  content = content.replace(/__VITE_CSS__/g, cssPath);
  content = content.replace(/__VITE_JS__/g, jsPath);
  const outName = file.replace(".tmpl.html", ".tmpl");
  writeFileSync(join(outDir, outName), content);
  count++;
}

// Also patch dist/index.html with cache-busting query params.
// dist/index.html にもキャッシュバスティングのクエリパラメータを付与。
const indexPath = join(distDir, "index.html");
let indexHtml = readFileSync(indexPath, "utf-8");
indexHtml = indexHtml.replace(
  /\/(assets\/index\.(js|css))/g,
  `/$1?v=${buildHash}`
);
writeFileSync(indexPath, indexHtml);

console.log(`inject-assets: ${count} template(s) → ${outDir}`);
