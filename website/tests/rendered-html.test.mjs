import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

async function render() {
  const workerUrl = new URL("../dist/server/index.js", import.meta.url);
  workerUrl.searchParams.set("test", `${process.pid}-${Date.now()}`);
  const { default: worker } = await import(workerUrl.href);

  return worker.fetch(
    new Request("http://localhost/", { headers: { accept: "text/html" } }),
    { ASSETS: { fetch: async () => new Response("Not found", { status: 404 }) } },
    { waitUntil() {}, passThroughOnException() {} },
  );
}

test("server-renders the CouchPilot field guide", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /CouchPilot Field Guide/);
  assert.match(html, /全局键位/);
  assert.match(html, /App 映射/);
  assert.match(html, /震动语言/);
  assert.match(html, /不会用 A 自动发消息/);
  assert.doesNotMatch(html, /codex-preview|react-loading-skeleton|Your site is taking shape/);
});

test("ships real mapping content without starter artifacts", async () => {
  const [page, layout, packageJson] = await Promise.all([
    readFile(new URL("../app/page.tsx", import.meta.url), "utf8"),
    readFile(new URL("../app/layout.tsx", import.meta.url), "utf8"),
    readFile(new URL("../package.json", import.meta.url), "utf8"),
  ]);

  for (const app of ["Codex", "Raycast", "Typeless", "Typora", "Obsidian", "VS Code", "QQ", "微信", "QQ 音乐", "Windows Terminal"]) {
    assert.match(page, new RegExp(app));
  }
  assert.match(page, /LT \+ RB/);
  assert.match(page, /Back \+ Start/);
  assert.match(layout, /CouchPilot Field Guide/);
  assert.doesNotMatch(packageJson, /react-loading-skeleton/);
});
