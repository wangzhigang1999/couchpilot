import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("builds a GitHub Pages-ready static entry point", async () => {
  const html = await readFile(new URL("../dist/index.html", import.meta.url), "utf8");

  assert.match(html, /CouchPilot Field Guide/);
  assert.match(html, /\/couchpilot\/assets\/[^\"]+\.js/);
  assert.match(html, /\/couchpilot\/assets\/[^\"]+\.css/);
  assert.doesNotMatch(html, /chatgpt\.site|vinext|wrangler/i);
});

test("ships the real mappings and safety rules", async () => {
  const page = await readFile(new URL("../app/page.tsx", import.meta.url), "utf8");

  for (const app of ["Codex", "Raycast", "Typeless", "Typora", "Obsidian", "VS Code", "QQ", "微信", "QQ 音乐", "Windows Terminal"]) {
    assert.match(page, new RegExp(app));
  }
  assert.match(page, /LT \+ RB/);
  assert.match(page, /Back \+ Start/);
  assert.match(page, /不会用 A 自动发消息/);
  assert.match(page, /不会让 X 停止 Codex/);
});
