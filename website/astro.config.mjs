import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

export default defineConfig({
  site: "https://wangzhigang1999.github.io",
  base: "/couchpilot",
  integrations: [
    starlight({
      title: "CouchPilot",
      description: "Control your desktop from a gamepad with fast, predictable, app-aware mappings.",
      logo: {
        src: "./src/assets/couchpilot-mark.svg",
        alt: "CouchPilot",
      },
      favicon: "/couchpilot/favicon.svg",
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/wangzhigang1999/couchpilot",
        },
      ],
      locales: {
        root: {
          label: "English",
          lang: "en",
        },
        "zh-cn": {
          label: "简体中文",
          lang: "zh-CN",
        },
      },
      customCss: ["./src/styles/custom.css"],
      editLink: {
        baseUrl: "https://github.com/wangzhigang1999/couchpilot/edit/main/website/",
      },
      sidebar: [
        {
          label: "Getting started",
          translations: { "zh-CN": "开始使用" },
          items: [
            { slug: "guide/controls" },
            { slug: "guide/window-switching" },
            { slug: "guide/haptics" },
            { slug: "guide/safety" },
          ],
        },
        {
          label: "App mappings",
          translations: { "zh-CN": "应用映射" },
          items: [{ autogenerate: { directory: "apps" } }],
        },
      ],
    }),
  ],
});
