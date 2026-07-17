import assert from "node:assert/strict";
import { access, readFile, readdir } from "node:fs/promises";
import test from "node:test";

const appSlugs = ["codex", "browsers", "raycast", "typeless", "notes", "vscode", "jetbrains", "chat", "assistants", "media", "documents", "terminal"];

test("builds English-first bilingual pages for GitHub Pages", async () => {
  const [english, chinese] = await Promise.all([
    readFile(new URL("../dist/index.html", import.meta.url), "utf8"),
    readFile(new URL("../dist/zh-cn/index.html", import.meta.url), "utf8"),
  ]);

  assert.match(english, /Leave the keyboard/);
  assert.match(english, /Desktop control · without the desk/);
  assert.match(english, /Voice editing/);
  assert.match(english, /Only active in whitelisted apps/);
  assert.match(english, /<html lang="en"/);
  assert.match(english, /\/couchpilot\/zh-cn\//);
  assert.match(chinese, /放下键盘/);
  assert.match(chinese, /离开桌子，也能掌控桌面/);
  assert.match(chinese, /语音编辑/);
  assert.match(chinese, /只在白名单 App 中启用/);
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

test("keeps the safety and voice-edit whitelist rules in both languages", async () => {
  const [englishCodex, englishChat, englishAssistant, englishSafety, chineseCodex, chineseChat, chineseAssistant, chineseSafety] = await Promise.all([
    readFile(new URL("../src/content/docs/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/apps/chat.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/apps/assistants.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/guide/safety.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/chat.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/assistants.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/guide/safety.md", import.meta.url), "utf8"),
  ]);

  assert.match(englishCodex, /X always remains right click/);
  assert.match(englishChat, /Outside the temporary voice-edit state, A remains the left mouse button/);
  assert.match(englishAssistant, /After <kbd>Y<\/kbd>, tap <kbd>A<\/kbd>/);
  assert.match(englishSafety, /Never steals input focus/);
  assert.match(englishSafety, /A sends only in an explicit voice-edit state/);
  assert.match(chineseCodex, /X 永远保持右键/);
  assert.match(chineseChat, /离开临时语音编辑状态后，A 仍然是鼠标左键/);
  assert.match(chineseAssistant, /按过 <kbd>Y<\/kbd> 后，轻按 <kbd>A<\/kbd>/);
  assert.match(chineseSafety, /不会自动抢输入框/);
  assert.match(chineseSafety, /A 只在明确的语音编辑状态发送/);
});

test("keeps the homepage mobile-first and free of the old blue gradient theme", async () => {
  const [css, mark, favicon] = await Promise.all([
    readFile(new URL("../src/styles/custom.css", import.meta.url), "utf8"),
    readFile(new URL("../src/assets/couchpilot-mark.svg", import.meta.url), "utf8"),
    readFile(new URL("../public/favicon.svg", import.meta.url), "utf8"),
  ]);

  assert.match(css, /--cp-accent:\s*#e4572e/i);
  assert.match(css, /@media \(max-width: 38rem\)/);
  assert.match(css, /\.cp-button\s*\{[^}]*width:\s*100%/s);
  assert.match(css, /overflow-x:\s*hidden/);
  assert.match(css, /\.cp-flow-line\s*\{[^}]*place-items:\s*center/s);
  assert.match(css, /\.cp-home \.cp-flow-line > span\s*\{[^}]*line-height:\s*1/s);
  assert.doesNotMatch(css, /\.cp-flow-line\s*\{[^}]*border-left/s);
  assert.doesNotMatch(css, /\.cp-command-deck::before/);
  assert.doesNotMatch(`${css}\n${mark}\n${favicon}`, /cyan|violet|linearGradient|radial-gradient|linear-gradient|backdrop-filter|blur\(/i);
});
