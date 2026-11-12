# WittyBluray

Plays Blu-ray images and `BDMV` folders in WittyMediaServer the way a player does, with their menus, including 4K, HDR and 3D discs. You need it if your disc libraries hold Blu-rays.

<!-- pictures -->

![A Blu-ray playing in a browser](docs/images/bluray-menu.webp)

<sub>A Blu-ray image, in a browser. [Sita Sings the Blues](https://archive.org/details/SitaSingsTheBlues_201812), Nina Paley, [CC0](https://creativecommons.org/publicdomain/zero/1.0/).</sub>

![A Blu-ray's page](docs/images/bluray-item.webp)

<sub>A disc's page in WittyMediaServer. [Sita Sings the Blues](https://archive.org/details/SitaSingsTheBlues_201812), Nina Paley, [CC0](https://creativecommons.org/publicdomain/zero/1.0/).</sub>

![A message saying the disc is encrypted](docs/images/bluray-refused.webp)

<sub>An encrypted pressing gets a plain message, and nothing unlocks it. [Big Buck Bunny](https://peach.blender.org/), Blender Foundation, [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/), from a disc pressed by Imagion AG.</sub>

<!-- /pictures -->

## What it does

- **Plays the disc's own program:** first play, titles, interactive and pop-up menus with animated highlights, chapters and the navigation timer, through a virtual machine for the disc's commands.
- **Reads without mounting:** `.iso` and `.img` images and `BDMV` folders, through WittyDisc.
- **4K and HDR:** it reads 2160p HEVC streams with their HDR10, HDR10+ and Dolby Vision flags, so a player can choose what to do on an SDR screen.
- **Subtitles** from the disc's presentation graphics, read in windows so a large film is never scanned whole.
- **Java (BD-J) menus,** only if you install a Java and turn this on. None is shipped. They run in a default-deny sandbox, and without Java the disc plays its titles.
- **Finds the feature among decoys.** Every playlist is scored from the disc's unencrypted navigation alone: distinct versus total time; repeated, shuffled or missing clips; fake short clips; how each clip joins the next (the clock carrying on, the clip's packet map, where the clips lie on an image); chapters, streams and file sizes; and where the disc's HDMV program leads, read by following its registers without running it, or what its Java program names, read from its class files. So the real film is picked among the hundreds or thousands of decoy playlists some discs carry, each cut of a film with several is listed, a TV disc's episodes come in order, and the answer comes with its reasons and a confidence. See [docs/PLAYLIST-HEURISTICS.md](docs/PLAYLIST-HEURISTICS.md).
- **Reads copies that went wrong.** Every length in every file is checked, a damaged part of a file is left out and named, each structure falls back on its `BACKUP` copy when it is missing, unreadable or contradicts the rest of the disc, and playlists, clip information and stream file sizes are checked against each other. A copy with no `index.bdmv` or `MovieObject.bdmv` (a "movie only" copy) plays from its playlists.
- **Refuses encrypted discs.** AACS-encrypted discs are refused and BD+ is not run. Nothing is decrypted. The server makes its own check as well, whatever a module says.
- **Tells you what's on a disc.** The `wittybluray` command-line tool reports all of this, simulates the menus and draws menu pages.

It doesn't do these yet:

| Not yet | What happens instead |
|---|---|
| Secondary audio, picture in picture | The disc's choice is recorded, not mixed in |
| The second view of a 3D disc | The 2D view plays, and the second view is reported |

Drawing menus into pictures, and the session code that turns steps into streams, stay with the server.

How to use it is in **Help** inside WittyMediaServer: see Blu-rays.

## How to get it

An admin adds it from the module store inside WittyMediaServer: **Settings → Modules**, then **Browse the store**, then **Add** beside WittyBluray. That needs WittyMediaServer 0.6.0-alpha or later, and it works with nothing older. The range this version fits is in `module.json`: 0.6.0-alpha through 0.6.x.

## What it needs

- FFmpeg, which comes with the server.
- Java 8 or newer on the server, only for discs with Java menus, and only if you turn that on.
- [WittyDisc](https://github.com/wittywilla/WittyDisc), which reads the images, and [WittyModule](https://github.com/wittywilla/WittyModule), which is how a server talks to it. Both are built in.
- No media comes with it.

It's written in Go with no cgo, from the published disc structure, and contains no code from other players. If you have libbluray installed, it can be asked for a second opinion on a disc's playlists (see below); it is never needed.

## Platforms

It has only been tested on custom-authored discs, so retail discs may show problems not seen yet.

A program is built for each system below, and the server uses the right one.

| System | Where it stands |
|---|---|
| Linux (x64, arm64, 32-bit ARM, x86) | Its tests pass on x64, and a Blu-ray playlist has played from a test server with it installed, checked by script. The other CPUs are only built |
| Windows (x64, arm64) | Built. This version hasn't been run on Windows since it became its own program |
| macOS (Intel, Apple silicon) | Built by the release tool. It has not been run on a real Mac |
| FreeBSD (amd64, arm64) | Cross-compiled only. It has not been run |

On macOS and FreeBSD the server has no sandbox for a module's helper: it gets a small environment and a soft memory limit, and nothing more.

## Its part of every app

A Blu-ray's page in every WittyMediaServer app comes from this module's own `sdui.json` (module API 4), kept equal to
`UI()` in `sdui.go` by a test, so any app that draws the server's interface shows it in every look with no code of
its own. Between the title's header and its cast, in this order, each section left out when it has nothing to show:

| Section | What it shows |
|---|---|
| This disc can't be played | An encrypted disc: what happened, that nothing was changed or copied, and what the disc's index still says |
| Feature Presentation | Play the film (the whole of it, however the disc cuts it up) and how many scenes it has |
| Scenes | The film's chapters as pictures, each playing from its start |
| Episodes | A TV disc's episodes in viewing order |
| Other versions | Other cuts and camera angles than the one Play plays |
| Bonus material | The disc's other playlists worth watching, without its decoys |
| Disc menus | Shortcuts straight into the disc's own menus |
| Why this playlist | Which playlist is the film, why, how sure the engine is, and the other candidates |
| Diagnosis | What was wrong with this copy and what was mended, and for an administrator **Check this disc** |

Each section's source takes the page's title and answers for a Blu-ray only. The server answers them from what it keeps
about the disc (this module's ranking, an administrator's choice of film, the names given to its playlists); the helper
answers no `sdui.items`. **The player stays each app's own:** playing the disc, its menus and buttons, and the remote
are the app's native player screen, not SDUI. The module has no settings page: its settings are in the server's own
**Settings, Playback & discs**.

**Why these sections are SDUI, and the rest isn't.** A disc module doesn't need SDUI to work. These sections are SDUI
only because they are plain rows of a title's page that every app already knows how to draw: as SDUI, the iPhone,
Apple TV and Android apps show a Blu-ray's feature, scenes, episodes, versions and why this playlist with no code of their
own, and the same words as the web. What stays each app's own, on purpose:

- **Playing the disc, its menus and buttons, and the remote**: drawn frame by frame from the disc, with timing and
  focus no page description could carry.
- **Disc settings and naming titles and scenes**: an administrator's dialogs on the web's title page (**Manage this
  title**), writing to the server's own routes (`PUT /api/items/{id}`, `PUT /api/items/{id}/disc/labels`).
- **The server-wide disc settings**: the hand-built **Settings, Playback & discs**, as every setting is.
- **WittyMediaPlayer's disc screens**: the desktop player draws its own.

## Building and testing

You need Go 1.26 or newer. In a worktree, first run the copy of `dev-modules.mjs` kept above the repositories (`node D:/projects/dev-modules.mjs` on the development machine). It writes a `go.work` that git ignores, so the checkout builds against the repositories beside it. Never run `go work use`.

```bash
go vet ./...
```

```bash
go test ./... -count=1
```

```bash
go run github.com/wittywilla/wittymodule/cmd/wittymodule check .
```

The last one checks the manifest. On the same tool, `pack -targets linux/amd64 -out release .` builds a release for one system. CI also builds for every system.

The tests run disc code in Java, and skip where there's no Java. `WMS_REQUIRE_JAVA=1` makes a skip a failure, as CI does. `bdj/runtime.jar` is built by `bdj/java/build.sh`, which needs a JDK 11 or newer. You only need that to change the runtime.

To look at a disc by hand:

```bash
go run ./cmd/wittybluray info MY_DISC.iso
```

| Command | What it shows |
|---|---|
| `wittybluray info <disc>` | Titles, the disc's program, playlists, clips and streams, 3D and HDR flags |
| `wittybluray rank <disc> [json]` | Which playlist is the feature and why: every playlist scored, the confidence, editions, episodes, chapters, what in the copy disagrees, and libbluray's second opinion when it is installed |
| `wittybluray playlist <disc> <n>` | Every item, stream and chapter of one playlist |
| `wittybluray menu <disc> <playlist>` | A playlist's menu: pages, buttons, every command |
| `wittybluray walk <disc> [button...]` | Simulates inserting the disc, then pressing menu buttons by id |
| `wittybluray render <disc> <dir>` | The first menu's pages as PNG files |
| `wittybluray bdj <disc> [seconds [picture.png [key...]]]` | Java titles and what their code needs; with Java, runs them, pressing the keys named |

To use it as a library, `wittybluray.Open` opens an image or folder, and `wittybluray.NewNav` gives a navigator: `Start`, `PressButton`, `PlaylistEnded` and the rest return what to do next. Check `d.Encrypted` first. `d.MainPlaylist()` is the feature. `d.Analyze(wittybluray.RankOptions{})`, `d.RankPlaylists()`, `d.Episodes()` and `d.Explain()` give the whole ranking, with the same shape as WittyDVD's; `pl.Chapters()` gives a playlist's chapters on its playing time and `d.Consistency()` where the copy's structures disagree.

### libbluray as a second opinion (optional)

WittyBluray needs nothing installed, and no libbluray code is in it or shipped with it. If a libbluray shared library is on the machine, the `libbluray` package loads it when asked (`dlopen` or `LoadLibrary` through [purego](https://github.com/ebitengine/purego): no cgo, nothing at build time) and compares its list of playlists, their lengths, chapters and angles, and the title it would play as the feature, with WittyBluray's. Disagreements are listed and lower the ranking's confidence; WittyBluray's own answer is never replaced. `wittybluray rank`, the diagnoser, and the helper's `ranking` call with `shown: true` are what ask.

```bash
brew install libbluray        # or your system's package; nothing to rebuild
go run ./cmd/wittybluray rank MY_DISC.iso
```

- It is built for 64-bit macOS and Windows always, and for Linux with `-tags libbluray` (still without cgo). The release's Linux program is a static binary that runs on any distribution, and a loader would tie it to the machine's C library; build with the tag for your own machine if you want the comparison there. `-tags nolibbluray` leaves it out everywhere.
- `WITTYBLURAY_LIBBLURAY=off` turns loading off, and `WITTYBLURAY_LIBBLURAY=/path/to/libbluray.so.2` names the one file to load. `libbluray.Capability()` says what was found.
- It is never used to decrypt. A disc is only handed to libbluray when its streams are clear and it has no `AACS` and no `BDSVM` folder at all, so libbluray has no reason to load libaacs or libbdplus; `LIBAACS_PATH` and `LIBBDPLUS_PATH` are pointed at a path that doesn't exist besides. No key file is given, and `bd_read` isn't bound at all.

`wittyblurayplayer/` is the same engine as a program of its own for WittyMediaPlayer, with its own `go.mod` and manifest.

## Licence

GNU Affero General Public License v3.0 (or later): see [LICENSE](LICENSE) and [NOTICE](NOTICE). If you redistribute it or build on it, please keep the NOTICE attribution. It's an original implementation and contains no code from other players. `bdj/runtime.jar` is original code that re-implements the published interfaces disc applications link against, and contains no Java runtime. Each release archive carries a `THIRD-PARTY-NOTICES.txt` for what its programs link.

## Legal

WittyBluray is an independent open-source project. It isn't associated with, sponsored by or
endorsed by the Blu-ray Disc Association, any film studio or any other player project. “Blu-ray” and
“Blu-ray Disc” are trademarks of the Blu-ray Disc Association; WittyBluray isn't a licensed or
certified Blu-ray product and uses no Blu-ray logo. AACS-encrypted discs are refused, BD+ isn't run,
nothing is decrypted, and no media comes with it: you're responsible for having the rights to what
you open. Any third-party names here only describe what it works with, and belong to their owners.
It's provided “as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use
it. This isn't legal advice.

<!-- module-kit -->
## Releasing

A version tag builds a program for every platform, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.3.0 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. The signing step uses the `WMS_SIGNING_KEY` secret of the `release` environment, and nothing is published without it.

To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged `withdrawals`, which servers read.

Licensed under AGPL-3.0-or-later: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
