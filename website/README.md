# CouchPilot Field Guide

Searchable Starlight documentation for CouchPilot's global gamepad controls, focused Codex and browser mappings, haptic feedback, and safety rules.

## Development

```powershell
npm install
npm run dev
```

## Validation

```powershell
npm test
```

Pull requests targeting `main` that change the website or its workflows run the same checks in **Docs check**, with a clean `npm ci` install. This read-only check does not deploy or cancel a Pages deployment; the existing **Docs** workflow publishes after merge.

English is the root and default locale. Simplified Chinese mirrors the same content structure under `src/content/docs/zh-cn` and is served at `/zh-cn/`.

The Codex and browser profiles live in `src/content/docs/apps` as standalone Markdown pages. Astro builds the site as static files and GitHub Pages publishes it whenever changes land on `main`.
