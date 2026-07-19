# Windows icon

`couchpilot.ico` is generated from the tracked `couchpilot-tray.svg`. The tray
source is intentionally separate from the website favicon: it has a transparent
background, a bright controller body for dark taskbars, and a dark keyline for
light taskbars. Its controls and silhouette are simplified to remain legible at
the native 16-pixel notification-area size.

The ICO contains PNG-compressed 32-bit images at 16, 20, 24, 32, 48, 64, 128,
and 256 pixels so Windows can select a native-size image at common DPI scales.

Regenerate it from the repository root with:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\generate-windows-icon.ps1
```

The command also writes `.cache/tray-icon-preview.png`, which shows the mark on
transparent, light, and dark surfaces at both native and enlarged sizes. Pass
`-PreviewPath <path>` to place the preview elsewhere.

The generator is isolated in its own Go module with pinned, pure-Go
dependencies. Normal application builds consume the checked-in ICO and use a
pinned `go-winres` release to embed icon resource ID 1, a CLI manifest, and
version information into the executable.
