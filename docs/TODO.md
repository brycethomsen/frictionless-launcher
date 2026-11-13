# TODO

## Done

- [x] Steam launch via protocol handler (`steam://rungameid/APPID`)
- [x] Epic Games launch via protocol handler (`com.epicgames.launcher://apps/...`)
- [x] Direct executable launch
- [x] Multi-game config with per-game schedules
- [x] Continuous schedule monitoring (checks every minute, not just on boot)
- [x] Boot-time launch if already within a schedule window
- [x] Launch tracking — `hasLaunchedInCurrentWindow` prevents re-launching in same window
- [x] Game process detection — `isGameRunning` for `direct` method games only (Steam/Epic use launch tracking instead)
- [x] Foreground app detection — skip launch entirely if an app is in the foreground
- [x] Countdown popup — standalone window with cancel button, closes game manager stays hidden
- [x] Config hot-reload via fsnotify
- [x] Fyne UI — add/edit/delete games, no YAML editing required
- [x] Steam + Epic game auto-discovery from manifests
- [x] Game picker — shows discovered games, falls back to manual entry
- [x] Schedule overlap validation — UI blocks saving two games with overlapping day/time windows
- [x] macOS dock icon hidden — `NSApplicationActivationPolicyAccessory` via CGo
- [x] Cross-platform build stubs (`dock_darwin.go` / `dock_other.go`)

## Bugs

- [ ] Steam launches to foreground on macOS despite `-g` flag
- [ ] macOS release binary isn't a real `.app` bundle — Finder shows it as
      "Unix Executable File" and double-click opens it through Terminal.app
      instead of launching silently into the tray. Blocks any normal user
      from just downloading and double-clicking the release. Un-released
      v0.3.0 over this on 2026-07-17; needs fixing before re-cutting it.
      Needs: `Contents/MacOS/Frictionless-Launcher`, `Contents/Info.plist`
      (with `LSUIElement` to declare "no dock icon" properly instead of only
      the runtime `NSApplicationActivationPolicyAccessory` call),
      `Contents/Resources/icon.icns`, and a `release.yml` packaging change to
      build/zip the bundle instead of the bare binary. Also the prerequisite
      for native notifications noted below.

## Up Next

- [x] Warn on overlapping schedules loaded from YAML (UI blocks creation; load now logs warnings)
- [x] Idle time detection — skipped; foreground app check covers the active-user case adequately
- [x] Multi-schedule per game in the UI editor — each window has independent day checkboxes + times, overlap validated within and across games

## Future / Lower Priority

- [ ] App blocklist — don't launch if a specific app is frontmost (e.g. Zoom, Plex). Consider: name-based text entry vs running process discovery vs file browser. Also reconsider whether current "any foreground app blocks" behavior should become "only blocked apps block" — more useful in practice.

- [ ] Native notifications (`UNUserNotificationCenter`) — blocked on the macOS app bundle fix above; tray cancel label is the current fallback

- [x] Import/export — export game schedules as YAML via file-save dialog, import via file-open dialog with replace confirmation

- [ ] GOG Galaxy support (discovery + protocol handler)
- [ ] System load guard — skip launch if CPU/memory above threshold
- [ ] Launch args auto-detection via PCGamingWiki or SKIF database
- [ ] Google Calendar integration for scheduling
- [ ] Auto-updater (check GitHub releases)
