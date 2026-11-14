# Frictionless Launcher

**Your game is already open when you sit down.**

You finish work and want to play, but first you have to open Steam, scroll
through the library, wait for an update, and reconsider what you're in the
mood for. Half an hour later you're on your phone. The hard part of playing
games after a long day isn't the game, it's getting it started.

Frictionless Launcher removes that step. Decide once which game you play and
when, for example "Cyberpunk 2077, Saturdays from 8 PM". When that time comes,
the launcher counts down for a few seconds and starts the game for you, through
Steam or Epic, so cloud saves, achievements and playtime all work as usual. If
tonight isn't the night, click Cancel and it stays quiet until next time.

**[Download the latest release](https://github.com/brycethomsen/frictionless-launcher/releases/latest)**
for Windows, macOS or Linux / Steam Deck.

## How it works

1. **Put your games on the weekly board.** Open *Manage Games* from the tray
   icon, add a game (your Steam and Epic libraries are detected
   automatically), and give it one or more time windows.
2. **Leave the launcher running.** It sits in your system tray and starts
   with your computer.
3. **Sit down and play.** When a window opens, a short countdown appears and
   then your game starts. If you start your computer in the middle of a
   window, it launches then.

## Why it works

**It plans your week.** The schedule board shows your game nights as a
weekly calendar. Give a game as many windows as you like (weekday evenings,
weekend mornings), click a block to change it, and toggle a game off without
losing its schedule. Overlapping windows are caught before you save, so two
games never compete for the same evening.

**It finds your games.** Installed Steam and Epic Games titles show up in
a list when you add a game. Anything else, such as a GOG game, an emulator,
or any program at all, can be added by path.

**It launches games the right way.** Steam and Epic games start through their
own launchers, so cloud saves sync and achievements and playtime count like
they would if you'd clicked Play yourself. Direct-launch games can take
launch arguments to skip intros and splash screens.

**It doesn't nag.** Every launch starts with a countdown you can cancel.
Cancel once and that window stays quiet. Each window launches at most once,
and a direct-launch game that's already running won't be started twice. On
macOS, if you're working in another app, it won't put a window in front of
you: the tray icon pulses instead, and you can cancel from the menu.

**It stays out of the way.** The tray menu shows your next three game
nights; click one to play right now instead of waiting.

**It's light.** The part that runs all day is just the tray icon and a
timer. On macOS it idles at 0% CPU and about 20 MB of memory. The schedule
board only runs while you have it open.

**No setup files to edit.** Games and schedules are set up entirely on the
board, and changes take effect immediately. Your games are exported and imported in one click,
and under the hood they're kept in one readable file you can back up or carry
on a USB stick.

## Install

Releases are built by GitHub Actions from this repository. They aren't
code-signed yet, so your OS will warn you the first time you open the app.

### Windows

1. Download `Frictionless-Launcher_Windows_x86_64.zip` and extract
   `Frictionless-Launcher.exe` to any folder you like. There's no installer.
2. Run it. If SmartScreen says *Windows protected your PC*, click
   **More info**, then **Run anyway**.
3. **Start with Windows:** press `Win + R`, type `shell:startup`, and put a
   shortcut to the exe in the folder that opens.

### macOS

1. Download `Frictionless-Launcher_Darwin_universal.zip` (works on both Apple
   silicon and Intel), unzip it, and move **Frictionless Launcher** to
   Applications.
2. Open it. If macOS says it can't be opened, go to **System Settings**,
   then **Privacy & Security**, and click **Open Anyway**. It runs in the
   menu bar, with no Dock icon.
3. **Start at login:** add it under **System Settings**, **General**,
   **Login Items**.

### Linux and Steam Deck

1. Download `Frictionless-Launcher_Linux_x86_64.tar.gz` and extract it.
2. Run `Frictionless-Launcher` from a desktop session that has a system tray.
   On Steam Deck, that means Desktop Mode.
3. **Start at login:** add it to your desktop environment's autostart
   settings.

## Using it

On first run the launcher starts with no games and lives in your tray. Click
the tray icon to open its menu:

- **Your next game nights**: the next three scheduled games and when they
  start. Click one to launch it now.
- **Manage Games...**: the weekly schedule board, where you add, edit,
  enable or disable games, and import or export your list.
- **View Logs**: opens the log file, useful if a launch didn't do what you
  expected.
- **Quit**

During a countdown, the top of the menu changes to
*Cyberpunk 2077 launching in 7s... — Cancel*.

### Adding a game

1. Open **Manage Games...** from the tray and click **Add Game**.
2. Pick your game from the list of detected Steam and Epic titles, or choose
   **Enter manually...** for anything else and browse to its executable.
3. Add one or more time windows: pick the days, a start time and an end
   time. Use **+ Add Time Window** for, say, weekday evenings plus weekend
   afternoons.
4. Click **Save**. The game appears on the board and in the tray menu right
   away.

Click any block on the board to change or delete that game, or untick it to
stop auto-launching it without losing its schedule.

Steam and Epic games launch through Steam or Epic, so cloud saves,
achievements and playtime work as usual. To pass launch options to one, set
them in Steam or Epic itself: for Cyberpunk 2077, right-click it in Steam,
choose **Properties**, and put `--launcher-skip` under **Launch Options** to
go straight into the game. Games added by path launch directly, and take
their arguments from the **Launch Args** field.

### Tips

- **Late-night sessions:** a window can't cross midnight. For Friday
  22:00 to 01:00, use two windows: Friday 22:00 to 23:59 and Saturday 00:00
  to 01:00.
- **Skipping intros:** [PCGamingWiki](https://www.pcgamingwiki.com) lists
  launch options that skip intro videos for most games.
- **Missed the start?** If you turn your computer on, or start the launcher,
  partway through a window, the game still launches after the countdown.

## The settings file

Everything you set up on the board is saved to a single `config.yaml`. You
never need to open it, but it's there for three things:

- **The countdown length.** This is the one setting the board doesn't have
  yet. Set `boot_delay` to the number of seconds you want (default `10`).
- **Backups and moving machines.** Copy the file, or use **Export** and
  **Import** on the board.
- **Portable mode.** If a `config.yaml` sits next to the launcher itself, it
  uses that one instead, which is handy for a USB stick or a self-contained
  folder.

Edits to the file take effect immediately, without a restart. Otherwise it
lives here, next to the log file (`frictionless-launcher.log`):

| OS | Location |
|---|---|
| Windows | `%LOCALAPPDATA%\FrictionlessLauncher\config.yaml` |
| macOS | `~/Library/Application Support/FrictionlessLauncher/config.yaml` |
| Linux | `~/.config/FrictionlessLauncher/config.yaml` |

<details>
<summary>File format reference</summary>

```yaml
boot_delay: 10   # countdown length, in seconds

games:
  - game_name: "Cyberpunk 2077"
    game_path: "steam://rungameid/1091500"
    launch_method: "steam"
    schedules:
      - days: [Sat, Sun]
        start_time: "20:00"
        end_time: "23:59"
    enabled: true
```

| Field | Meaning |
|---|---|
| `boot_delay` | Seconds the countdown runs before a game launches. Default `10`. |
| `game_name` | Shown on the board and in the tray menu. |
| `game_path` | A `steam://rungameid/APPID` or `com.epicgames.launcher://` URL, or the full path to an executable. |
| `launch_method` | `steam`, `epic`, or `direct` (an executable). |
| `launch_args` | Command-line arguments. Used by `direct` games only; Steam and Epic games take theirs from Steam or Epic. |
| `schedules` | One or more windows, each with `days` (`Mon` to `Sun`), `start_time` and `end_time` in 24-hour `HH:MM`. A window includes its end minute. |
| `enabled` | `false` keeps the game on the board but never auto-launches it. |

</details>

## Roadmap

On the list: GOG Galaxy support, a blocklist so specific apps (say, a video
call) can hold off a launch, native notifications, and automatic updates.
See [docs/TODO.md](docs/TODO.md) for the full list and known issues.

## Building from source

You need Go (the version in [`go.mod`](go.mod)) and a C compiler, because the
Fyne UI toolkit uses cgo. On Linux, also install
`libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev`. Then:

```bash
git clone https://github.com/brycethomsen/frictionless-launcher.git
cd frictionless-launcher
make build        # builds ./frictionless-launcher for this machine
make test         # runs the test suite
make check        # formatting, vet, race-enabled tests: what CI runs
make help         # cross-compiling and the macOS .app bundle
```

Before contributing, read [AGENTS.md](AGENTS.md), which covers where code
goes, the rules that keep the always-on tray process light, and the checks
to run. [docs/DESIGN.md](docs/DESIGN.md) explains the architecture and the
decisions behind it.
