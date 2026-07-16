# CouchPilot Field Guide

Searchable Starlight documentation for CouchPilot's global gamepad controls, per-app mappings, haptic feedback, and safety rules.

## Development

```powershell
npm install
npm run dev
```

## Validation

```powershell
npm test
```

English is the root and default locale. Simplified Chinese mirrors the same content structure under `src/content/docs/zh-cn` and is served at `/zh-cn/`.

Each app profile lives in `src/content/docs/apps` as a standalone Markdown page. Astro builds the site as static files and GitHub Pages publishes it whenever changes land on `main`.
