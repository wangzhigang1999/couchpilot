"use client";

import { useMemo, useState } from "react";

type Mapping = {
  control: string;
  action: string;
  shortcut?: string;
  note?: string;
};

type AppGuide = {
  id: string;
  name: string;
  apps: string;
  category: string;
  featured?: boolean;
  mark: string;
  summary: string;
  process: string;
  mappings: Mapping[];
  caution?: string;
};

const globalMappings: Mapping[] = [
  { control: "左摇杆", action: "移动鼠标", note: "直接、连续移动" },
  { control: "右摇杆", action: "滚动页面", note: "上下滚动" },
  { control: "A", action: "鼠标左键", note: "按住可拖拽、框选" },
  { control: "X", action: "鼠标右键", note: "按住可右键拖拽" },
  { control: "Y", action: "语音输入", shortcut: "Right Alt" },
  { control: "十字键", action: "方向键", shortcut: "↑ ↓ ← →" },
  { control: "LT", action: "精准移动", note: "降低鼠标速度" },
  { control: "RT", action: "快速移动", note: "提高鼠标速度" },
  { control: "LT + LB", action: "上一个窗口", shortcut: "Alt + Shift + Tab" },
  { control: "LT + RB", action: "下一个窗口", shortcut: "Alt + Tab" },
  { control: "Back + Start", action: "紧急退出", note: "按住 1.5 秒" },
];

const apps: AppGuide[] = [
  {
    id: "codex",
    name: "Codex",
    apps: "ChatGPT / Codex 桌面版",
    category: "高频",
    featured: true,
    mark: "CX",
    summary: "任务切换、命令菜单和终端都放在最顺手的位置。",
    process: "ChatGPT.exe · OpenAI.Codex",
    mappings: [
      { control: "B", action: "返回", shortcut: "Ctrl + [" },
      { control: "LB", action: "上一个任务", shortcut: "Ctrl + Shift + [" },
      { control: "RB", action: "下一个任务", shortcut: "Ctrl + Shift + ]" },
      { control: "L3", action: "命令菜单", shortcut: "Ctrl + K" },
      { control: "R3", action: "打开终端", shortcut: "Ctrl + `" },
      { control: "X", action: "鼠标右键", note: "不会发送 Escape，不会停止回答" },
    ],
    caution: "X 永远保持右键，避免误停正在生成的回答。",
  },
  {
    id: "browser",
    name: "浏览器",
    apps: "Chrome · Edge · Firefox",
    category: "浏览器",
    featured: true,
    mark: "WEB",
    summary: "肩键切标签，摇杆按下直达地址栏和新标签。",
    process: "chrome.exe · msedge.exe · firefox.exe",
    mappings: [
      { control: "B", action: "后退", shortcut: "Alt + Left" },
      { control: "LB", action: "上一个标签", shortcut: "Ctrl + Shift + Tab" },
      { control: "RB", action: "下一个标签", shortcut: "Ctrl + Tab" },
      { control: "L3", action: "聚焦地址栏", shortcut: "Ctrl + L" },
      { control: "R3", action: "新建标签", shortcut: "Ctrl + T" },
    ],
  },
  {
    id: "raycast",
    name: "Raycast",
    apps: "Raycast for Windows",
    category: "高频",
    featured: true,
    mark: "RY",
    summary: "把启动器当成游戏菜单：肩键选，A 确认，B 退出。",
    process: "Raycast.exe",
    mappings: [
      { control: "A", action: "确认当前项目", shortcut: "Enter" },
      { control: "B", action: "关闭 / 返回", shortcut: "Escape" },
      { control: "LB", action: "向上选择", shortcut: "Up" },
      { control: "RB", action: "向下选择", shortcut: "Down" },
    ],
  },
  {
    id: "typeless",
    name: "Typeless",
    apps: "Typeless 语音输入",
    category: "高频",
    featured: true,
    mark: "TY",
    summary: "Y 启动语音，B 关闭浮层，其余保持鼠标行为。",
    process: "Typeless.exe",
    mappings: [
      { control: "Y", action: "启动语音输入", shortcut: "Right Alt" },
      { control: "B", action: "关闭浮层", shortcut: "Escape" },
    ],
  },
  {
    id: "notes",
    name: "笔记与写作",
    apps: "Typora · Obsidian",
    category: "创作",
    featured: true,
    mark: "NOTE",
    summary: "切标签、查找、新建文档；不会主动抢输入焦点。",
    process: "Typora.exe · Obsidian.exe",
    mappings: [
      { control: "LB", action: "上一个标签", shortcut: "Ctrl + Shift + Tab" },
      { control: "RB", action: "下一个标签", shortcut: "Ctrl + Tab" },
      { control: "L3", action: "查找", shortcut: "Ctrl + F" },
      { control: "R3", action: "新建文档", shortcut: "Ctrl + N" },
    ],
    caution: "A 仍是鼠标左键，不会自动聚焦编辑区。",
  },
  {
    id: "vscode",
    name: "VS Code",
    apps: "Visual Studio Code",
    category: "开发",
    mark: "VS",
    summary: "肩键切编辑器，两个摇杆按键负责命令和文件跳转。",
    process: "Code.exe",
    mappings: [
      { control: "B", action: "关闭菜单 / 返回", shortcut: "Escape" },
      { control: "LB", action: "上一个标签", shortcut: "Ctrl + Shift + Tab" },
      { control: "RB", action: "下一个标签", shortcut: "Ctrl + Tab" },
      { control: "L3", action: "命令面板", shortcut: "Ctrl + Shift + P" },
      { control: "R3", action: "快速打开", shortcut: "Ctrl + P" },
    ],
  },
  {
    id: "jetbrains",
    name: "JetBrains IDE",
    apps: "PyCharm · IntelliJ IDEA · GoLand",
    category: "开发",
    mark: "JB",
    summary: "只加入最稳妥的查找和退出，避免覆盖 IDE 自己的复杂键位。",
    process: "pycharm64.exe · idea64.exe · goland64.exe",
    mappings: [
      { control: "B", action: "关闭菜单 / 返回", shortcut: "Escape" },
      { control: "L3", action: "当前文件查找", shortcut: "Ctrl + F" },
    ],
  },
  {
    id: "chat",
    name: "聊天",
    apps: "QQ · 微信",
    category: "沟通",
    mark: "CHAT",
    summary: "查找和关闭浮层；发送行为刻意留给语音或鼠标确认。",
    process: "QQ.exe · Weixin.exe · WeChat.exe",
    mappings: [
      { control: "B", action: "关闭浮层 / 返回", shortcut: "Escape" },
      { control: "L3", action: "查找", shortcut: "Ctrl + F" },
      { control: "A", action: "鼠标左键", note: "不会自动发送消息" },
    ],
    caution: "A 不映射 Enter，避免误发消息。",
  },
  {
    id: "assistants",
    name: "AI 助手",
    apps: "Claude · Cherry Studio",
    category: "沟通",
    mark: "AI",
    summary: "保留通用鼠标和语音，只增加查找与安全退出。",
    process: "Claude.exe · CherryStudio.exe",
    mappings: [
      { control: "B", action: "关闭浮层 / 返回", shortcut: "Escape" },
      { control: "L3", action: "查找", shortcut: "Ctrl + F" },
    ],
  },
  {
    id: "media",
    name: "音乐与视频",
    apps: "QQ 音乐 · Spotify · VLC",
    category: "媒体",
    mark: "PLAY",
    summary: "使用 Windows 全局媒体键，播放器不在前台也通常有效。",
    process: "QQMusic.exe · Spotify.exe · vlc.exe",
    mappings: [
      { control: "LB", action: "上一首", shortcut: "Media Previous" },
      { control: "RB", action: "下一首", shortcut: "Media Next" },
      { control: "L3", action: "静音 / 取消静音", shortcut: "Volume Mute" },
      { control: "R3", action: "播放 / 暂停", shortcut: "Media Play/Pause" },
    ],
  },
  {
    id: "documents",
    name: "文档与 PDF",
    apps: "Acrobat · Word · Excel · PowerPoint",
    category: "文档",
    mark: "DOC",
    summary: "肩键翻页，左摇杆按键查找，适合阅读和演示。",
    process: "Acrobat.exe · WINWORD.EXE · EXCEL.EXE · POWERPNT.EXE",
    mappings: [
      { control: "LB", action: "向上翻页", shortcut: "Page Up" },
      { control: "RB", action: "向下翻页", shortcut: "Page Down" },
      { control: "L3", action: "查找", shortcut: "Ctrl + F" },
    ],
  },
  {
    id: "terminal",
    name: "Windows Terminal",
    apps: "Windows Terminal",
    category: "开发",
    mark: "TERM",
    summary: "切终端标签、打开命令面板和新标签。",
    process: "WindowsTerminal.exe",
    mappings: [
      { control: "B", action: "关闭菜单 / 返回", shortcut: "Escape" },
      { control: "LB", action: "上一个标签", shortcut: "Ctrl + Shift + Tab" },
      { control: "RB", action: "下一个标签", shortcut: "Ctrl + Tab" },
      { control: "L3", action: "命令面板", shortcut: "Ctrl + Shift + P" },
      { control: "R3", action: "新建标签", shortcut: "Ctrl + T" },
    ],
  },
];

const categories = ["全部", "高频", "浏览器", "创作", "沟通", "媒体", "开发", "文档"];

function MappingRows({ mappings }: { mappings: Mapping[] }) {
  return (
    <div className="mapping-list">
      {mappings.map((mapping) => (
        <div className="mapping-row" key={`${mapping.control}-${mapping.action}`}>
          <kbd>{mapping.control}</kbd>
          <div className="mapping-copy">
            <strong>{mapping.action}</strong>
            {mapping.note && <span>{mapping.note}</span>}
          </div>
          {mapping.shortcut && <code>{mapping.shortcut}</code>}
        </div>
      ))}
    </div>
  );
}

export default function Home() {
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("全部");

  const filteredApps = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return apps.filter((app) => {
      const categoryMatch = category === "全部" || (category === "高频" ? app.featured : app.category === category);
      const searchable = [
        app.name,
        app.apps,
        app.summary,
        app.process,
        ...app.mappings.flatMap((mapping) => [mapping.control, mapping.action, mapping.shortcut ?? "", mapping.note ?? ""]),
      ]
        .join(" ")
        .toLocaleLowerCase();
      return categoryMatch && (!needle || searchable.includes(needle));
    });
  }, [category, query]);

  return (
    <main>
      <header className="topbar">
        <a className="brand" href="#top" aria-label="CouchPilot 文档首页">
          <span className="brand-mark">CP</span>
          <span>CouchPilot</span>
          <small>Field Guide</small>
        </a>
        <nav aria-label="页面导航">
          <a href="#global">全局键位</a>
          <a href="#apps">App 映射</a>
          <a href="#haptics">震动</a>
          <a className="github-link" href="https://github.com/wangzhigang1999/couchpilot" target="_blank" rel="noreferrer">
            GitHub ↗
          </a>
        </nav>
      </header>

      <section className="hero" id="top">
        <div className="hero-grid" />
        <div className="eyebrow"><span /> COUCHPILOT / WINDOWS / XINPUT</div>
        <h1>拿起手柄，<br /><em>先看这里。</em></h1>
        <p className="hero-copy">所有全局键位和 App 专属映射都在一页里。输入 App、按键或动作，马上找到你要的操作。</p>
        <label className="search-box">
          <span aria-hidden="true">⌕</span>
          <input
            type="search"
            placeholder="搜索 App、按键或动作，例如：微信 / R3 / 下一首"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            aria-label="搜索映射"
          />
          {query && <button type="button" onClick={() => setQuery("")} aria-label="清除搜索">×</button>}
        </label>
        <div className="hero-stats" aria-label="项目摘要">
          <span><strong>12</strong> App profiles</span>
          <span><strong>120Hz</strong> Input polling</span>
          <span><strong>0</strong> Runtime deps</span>
        </div>
      </section>

      <section className="section global-section" id="global">
        <div className="section-heading">
          <div><span className="section-number">01</span><h2>全局键位</h2></div>
          <p>这些操作在所有 App 里都可用。App 专属映射只覆盖表中明确写出的按键。</p>
        </div>
        <div className="global-grid">
          {globalMappings.map((mapping) => (
            <article className="global-card" key={mapping.control}>
              <kbd>{mapping.control}</kbd>
              <strong>{mapping.action}</strong>
              <span>{mapping.shortcut ?? mapping.note}</span>
            </article>
          ))}
        </div>
        <div className="window-tip">
          <span className="pulse-dot" />
          <div><strong>多个窗口怎么切？</strong><p>一直按住 LT，反复轻点 RB / LB 浏览所有窗口，松开 LT 才确认。不会只在两个窗口之间来回跳。</p></div>
        </div>
      </section>

      <section className="section apps-section" id="apps">
        <div className="section-heading">
          <div><span className="section-number">02</span><h2>App 映射</h2></div>
          <p>优先做高频、明确、安全的操作。没有列出的按键继续使用全局行为。</p>
        </div>
        <div className="category-bar" aria-label="App 分类">
          {categories.map((item) => (
            <button key={item} className={category === item ? "active" : ""} onClick={() => setCategory(item)} type="button">
              {item}
            </button>
          ))}
        </div>
        <div className="result-line"><span>{filteredApps.length}</span> 个配置符合当前筛选</div>
        <div className="app-grid">
          {filteredApps.map((app) => (
            <article className="app-card" key={app.id} id={app.id}>
              <div className="app-card-head">
                <span className="app-mark">{app.mark}</span>
                <div><p>{app.category}</p><h3>{app.name}</h3><span>{app.apps}</span></div>
              </div>
              <p className="app-summary">{app.summary}</p>
              <MappingRows mappings={app.mappings} />
              {app.caution && <div className="caution">安全设计 · {app.caution}</div>}
              <div className="process-name">识别：{app.process}</div>
            </article>
          ))}
        </div>
        {filteredApps.length === 0 && (
          <div className="empty-state"><strong>没有找到对应映射</strong><p>换个 App 名、按键名，或切回“全部”试试。</p></div>
        )}
      </section>

      <section className="section feedback-section" id="haptics">
        <div className="section-heading">
          <div><span className="section-number">03</span><h2>震动语言</h2></div>
          <p>不是每次都同样震。反馈强弱会告诉你刚刚执行的是哪类操作。</p>
        </div>
        <div className="haptic-track">
          <div><span className="wave one" /><strong>轻触</strong><p>A / X 点击</p></div>
          <div><span className="wave two" /><strong>导航</strong><p>方向、标签、菜单</p></div>
          <div><span className="wave three" /><strong>确认</strong><p>语音输入启动</p></div>
          <div><span className="wave four" /><strong>强确认</strong><p>窗口切换与落定</p></div>
        </div>
        <div className="config-note"><code>haptics_enabled: true</code><code>haptic_strength: 1.0</code><p>强度范围 0.0–2.0；不想震可以完全关闭。</p></div>
      </section>

      <section className="section safety-section" id="safety">
        <div className="section-heading">
          <div><span className="section-number">04</span><h2>不会发生什么</h2></div>
          <p>这些约束来自真实使用中踩过的坑，因此被当作产品规则保留下来。</p>
        </div>
        <div className="safety-grid">
          <article><span>01</span><h3>不会自动抢输入框</h3><p>切换 App 或移动鼠标时，CouchPilot 不会擅自改变焦点。</p></article>
          <article><span>02</span><h3>不会用 A 自动发消息</h3><p>QQ、微信等聊天应用中，A 始终是鼠标左键。</p></article>
          <article><span>03</span><h3>不会让 X 停止 Codex</h3><p>Codex 中 X 保持右键，不发送 Escape。</p></article>
          <article><span>04</span><h3>随时可以紧急退出</h3><p>Back + Start 按住 1.5 秒，立即停止 CouchPilot。</p></article>
        </div>
      </section>

      <footer>
        <div><span className="brand-mark">CP</span><strong>CouchPilot</strong></div>
        <p>Pilot your desktop from a gamepad.</p>
        <a href="https://github.com/wangzhigang1999/couchpilot" target="_blank" rel="noreferrer">View source on GitHub ↗</a>
      </footer>
    </main>
  );
}
