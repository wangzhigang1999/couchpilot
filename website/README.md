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

Each app profile lives in `src/content/docs/apps` as a standalone Markdown page. Astro builds the site as static files and GitHub Pages publishes it whenever changes land on `main`.
