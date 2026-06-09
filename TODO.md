# TODO

## Implemented
- [x] Steam launch via protocol handler (keeps cloud saves)
- [x] Epic Games launch via protocol handler (keeps cloud saves)
- [x] Direct executable launch
- [x] Game-running detection (don't launch if game is running)
- [x] Launch tracking (don't re-launch in same schedule window)
- [x] Continuous schedule monitoring (check every minute, launch on schedule)
- [x] Config hot-reload (changes take effect without restart)

## Bugs
- [ ] Steam launches to foreground on macOS when not already running (despite `-g` flag)

## Features (Future)
- [ ] Foreground app detection UI (popup with countdown + cancel button)
- [ ] User idle time detection (don't launch if AFK < 5 minutes)
- [ ] Game auto-discovery (scan Steam library)
- [ ] Launch args auto-detection (SKIF/PCGamingWiki)
- [ ] GOG Galaxy support
- [ ] Epic manifest parsing for auto-discovery
