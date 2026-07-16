import assert from "node:assert/strict";
import { access, readFile, readdir } from "node:fs/promises";
import test from "node:test";

const appSlugs = ["codex", "browsers", "raycast", "typeless", "notes", "vscode", "jetbrains", "chat", "assistants", "media", "documents", "terminal"];

test("builds English-first bilingual pages for GitHub Pages", async () => {
  const [english, chinese] = await Promise.all([
    readFile(new URL("../dist/index.html", import.meta.url), "utf8"),
    readFile(new URL("../dist/zh-cn/index.html", import.meta.url), "utf8"),
  ]);

  assert.match(english, /Pick up the gamepad\. Take over the desktop\./);
  assert.match(english, /<html lang="en"/);
  assert.match(english, /\/couchpilot\/zh-cn\//);
  assert.match(chinese, /拿起手柄，接管桌面/);
  assert.match(chinese, /<html lang="zh-CN"/);
  assert.match(chinese, /\/couchpilot\/_astro\//);
  assert.doesNotMatch(english, /chatgpt\.site|vinext|wrangler/i);
  await access(new URL("../dist/pagefind/pagefind.js", import.meta.url));
});

test("publishes every app profile in English and Simplified Chinese", async () => {
  const [englishFiles, chineseFiles] = await Promise.all([
    readdir(new URL("../src/content/docs/apps/", import.meta.url)),
    readdir(new URL("../src/content/docs/zh-cn/apps/", import.meta.url)),
  ]);

  assert.equal(englishFiles.filter((file) => file !== "index.mdx" && /\.mdx?$/.test(file)).length, 12);
  assert.equal(chineseFiles.filter((file) => file !== "index.mdx" && /\.mdx?$/.test(file)).length, 12);

  for (const slug of appSlugs) {
    const [english, chinese] = await Promise.all([
      readFile(new URL(`../dist/apps/${slug}/index.html`, import.meta.url), "utf8"),
      readFile(new URL(`../dist/zh-cn/apps/${slug}/index.html`, import.meta.url), "utf8"),
    ]);
    assert.match(english, /CouchPilot/);
    assert.match(chinese, /CouchPilot/);
  }
});

test("keeps the safety rules in both languages", async () => {
  const [englishCodex, englishChat, englishSafety, chineseCodex, chineseChat, chineseSafety] = await Promise.all([
    readFile(new URL("../src/content/docs/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/apps/chat.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/guide/safety.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/chat.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/guide/safety.md", import.meta.url), "utf8"),
  ]);

  assert.match(englishCodex, /X always remains right click/);
  assert.match(englishChat, /A is never mapped to Enter/);
  assert.match(englishSafety, /Never steals input focus/);
  assert.match(chineseCodex, /X 永远保持右键/);
  assert.match(chineseChat, /A 不映射 Enter/);
  assert.match(chineseSafety, /不会自动抢输入框/);
});
