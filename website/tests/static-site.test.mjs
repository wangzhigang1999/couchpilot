import assert from "node:assert/strict";
import { access, readFile, readdir } from "node:fs/promises";
import test from "node:test";

const appSlugs = ["codex", "browsers"];
const removedAppSlugs = ["raycast", "typeless", "notes", "vscode", "jetbrains", "chat", "assistants", "media", "documents", "terminal"];

test("builds English-first bilingual pages for GitHub Pages", async () => {
  const [english, chinese] = await Promise.all([
    readFile(new URL("../dist/index.html", import.meta.url), "utf8"),
    readFile(new URL("../dist/zh-cn/index.html", import.meta.url), "utf8"),
  ]);

  assert.match(english, /Leave the keyboard/);
  assert.match(english, /Desktop control · without the desk/);
  assert.match(english, /Voice editing/);
  assert.match(english, /Y, speak, pause, then A sends in every app/);
  assert.match(english, /2<\/strong> focused app profiles/);
  assert.match(english, /<html lang="en"/);
  assert.match(english, /\/couchpilot\/zh-cn\//);
  assert.match(chinese, /放下键盘/);
  assert.match(chinese, /离开桌子，也能掌控桌面/);
  assert.match(chinese, /语音编辑/);
  assert.match(chinese, /所有 App 都支持 Y、说话、稍停后按 A 回车/);
  assert.match(chinese, /2<\/strong> 个专注的 App profile/);
  assert.match(chinese, /<html lang="zh-CN"/);
  assert.match(chinese, /\/couchpilot\/_astro\//);
  assert.doesNotMatch(english, /chatgpt\.site|vinext|wrangler/i);
  await access(new URL("../dist/pagefind/pagefind.js", import.meta.url));
});

test("publishes only Codex and browser profiles in both languages", async () => {
  const [englishFiles, chineseFiles] = await Promise.all([
    readdir(new URL("../src/content/docs/apps/", import.meta.url)),
    readdir(new URL("../src/content/docs/zh-cn/apps/", import.meta.url)),
  ]);

  assert.deepEqual(englishFiles.filter((file) => file !== "index.mdx" && /\.mdx?$/.test(file)).sort(), ["browsers.md", "codex.md"]);
  assert.deepEqual(chineseFiles.filter((file) => file !== "index.mdx" && /\.mdx?$/.test(file)).sort(), ["browsers.md", "codex.md"]);

  for (const slug of appSlugs) {
    const [english, chinese] = await Promise.all([
      readFile(new URL(`../dist/apps/${slug}/index.html`, import.meta.url), "utf8"),
      readFile(new URL(`../dist/zh-cn/apps/${slug}/index.html`, import.meta.url), "utf8"),
    ]);
    assert.match(english, /CouchPilot/);
    assert.match(chinese, /CouchPilot/);
  }

  const [englishIndex, chineseIndex] = await Promise.all([
    readFile(new URL("../dist/apps/index.html", import.meta.url), "utf8"),
    readFile(new URL("../dist/zh-cn/apps/index.html", import.meta.url), "utf8"),
  ]);
  for (const slug of removedAppSlugs) {
    assert.doesNotMatch(englishIndex, new RegExp(`/couchpilot/apps/${slug}/`));
    assert.doesNotMatch(chineseIndex, new RegExp(`/couchpilot/zh-cn/apps/${slug}/`));
  }
});

test("documents delayed global voice submit and Codex-only deletion in both languages", async () => {
  const [englishCodex, englishBrowser, englishControls, englishSafety, chineseCodex, chineseBrowser, chineseControls, chineseSafety] = await Promise.all([
    readFile(new URL("../src/content/docs/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/apps/browsers.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/guide/controls.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/guide/safety.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/codex.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/apps/browsers.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/guide/controls.md", import.meta.url), "utf8"),
    readFile(new URL("../src/content/docs/zh-cn/guide/safety.md", import.meta.url), "utf8"),
  ]);

  assert.match(englishCodex, /X always remains right click/);
  assert.match(englishCodex, /Codex additionally arms B as \*\*Backspace\*\*/);
  assert.match(englishBrowser, /Y<\/kbd>, then <kbd>A/);
  assert.match(englishControls, /Voice submit in every app/);
  assert.match(englishControls, /does not receive microphone or VAD events/);
  assert.match(englishControls, /Pressing A too early is safely ignored/);
  assert.match(englishSafety, /Never steals input focus/);
  assert.match(englishSafety, /A sends only in an explicit voice-edit state/);
  assert.match(englishSafety, /waits for the configured minimum delay/);
  assert.match(chineseCodex, /X 永远保持右键/);
  assert.match(chineseCodex, /Codex 还会额外把 B 变成 \*\*Backspace\*\*/);
  assert.match(chineseBrowser, /<kbd>Y<\/kbd>，然后 <kbd>A/);
  assert.match(chineseControls, /所有 App 都能语音后回车/);
  assert.match(chineseControls, /拿不到麦克风或 VAD 事件/);
  assert.match(chineseControls, /过早按 A 会被安全忽略/);
  assert.match(chineseSafety, /不会自动抢输入框/);
  assert.match(chineseSafety, /A 只在明确的语音编辑状态发送/);
  assert.match(chineseSafety, /先等待配置的最短时间/);
});

test("documents only the minimal privacy-safe local trace", async () => {
  const [english, chinese] = await Promise.all([
    readFile(new URL("../dist/guide/tracing/index.html", import.meta.url), "utf8"),
    readFile(new URL("../dist/zh-cn/guide/tracing/index.html", import.meta.url), "utf8"),
  ]);

  assert.match(english, /small local trace/i);
  assert.match(english, /trace\/trace\.jsonl/i);
  assert.match(english, /foreground executable base name/i);
  assert.match(english, /never includes typed text/i);
  assert.match(english, /stays on the device and is never uploaded/i);
  assert.match(chinese, /很小的本地 trace/);
  assert.match(chinese, /trace\/trace\.jsonl/i);
  assert.match(chinese, /前台程序的可执行文件名/);
  assert.match(chinese, /不会记录输入文字/);
  assert.match(chinese, /数据只留在本机，绝不会上传/);
  for (const page of [english, chinese]) {
    assert.doesNotMatch(page, /usage-v1-report\.html|couchpilot\.exe usage|local_usage_stats_enabled/i);
    assert.doesNotMatch(page, /open the report|查看按键报告|recommendation dashboard|推荐界面/i);
  }
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
