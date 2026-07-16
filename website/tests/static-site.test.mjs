import assert from "node:assert/strict";
import { access, readFile, readdir } from "node:fs/promises";
import test from "node:test";

test("builds a GitHub Pages-ready Starlight site", async () => {
  const html = await readFile(new URL("../dist/index.html", import.meta.url), "utf8");

  assert.match(html, /CouchPilot Field Guide/);
  assert.match(html, /拿起手柄，接管桌面/);
  assert.match(html, /\/couchpilot\/_astro\//);
  assert.doesNotMatch(html, /chatgpt\.site|vinext|wrangler/i);
  await access(new URL("../dist/pagefind/pagefind.js", import.meta.url));
});

test("publishes one searchable page for every app profile", async () => {
  const appsDirectory = new URL("../src/content/docs/apps/", import.meta.url);
  const files = (await readdir(appsDirectory)).filter((file) => file !== "index.mdx" && /\.mdx?$/.test(file));

  assert.equal(files.length, 12);
  for (const slug of ["codex", "browsers", "raycast", "typeless", "notes", "vscode", "jetbrains", "chat", "assistants", "media", "documents", "terminal"]) {
    const html = await readFile(new URL(`../dist/apps/${slug}/index.html`, import.meta.url), "utf8");
    assert.match(html, /CouchPilot/);
  }
});

test("keeps the hard-won safety rules", async () => {
  const [codex, chat, safety] = await Promise.all([
    readFile(new URL("../src/content/docs/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/apps/chat.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/guide/safety.md", import.meta.url), "utf8"),
  ]);

  assert.match(codex, /X 永远保持右键/);
  assert.match(chat, /A 不映射 Enter/);
  assert.match(safety, /不会自动抢输入框/);
  assert.match(safety, /Back \+ Start/);
});
