# Changelog

What changed in WittyBluray, newest first. What changed before it had a repository of its own is in the
[WittyMediaServer changelog](https://github.com/wittywilla/WittyMediaServer/blob/master/CHANGELOG.md).

## Unreleased

- **WittyBluray 0.4.0:** built for module API 4: it carries `sdui.json` (its theme scope, for the theme editor), kept equal to `UI()` by a test. Its pages stay the server's own disc pages.
- **WittyBluray 0.4.0:** a part of a file an image doesn't record (WittyDisc 0.2.1 gives it no place on the image) is refused with a message rather than streamed from the wrong bytes. The quirks addendum is now part of the server's Blu-ray quirks reference, and is gone from here.
- **WittyBluray 0.4.0:** The playlist ranking engine, the static reading of a disc's programs, chapters, the cross-checks, BACKUP repair and the optional libbluray second opinion listed above, for WittyMediaServer up to 0.8.x. It needs WittyDisc 0.2.1, which reads small files UDF keeps inside their file entry, so the workaround for that is gone. purego (Apache-2.0) is credited in NOTICE. The synthetic discs can be written out for other projects' tests (`WITTYBLURAY_SYNTH_OUT`). Checked against images of pressed discs, which showed one mistake, now fixed: the playlist behind a menu, which loops one short clip under the menu's graphics, was called a decoy and the disc copy-protected or damaged. It is now classed `menu` (`ClassMenu`, `FlagMenu`).
- `wittyblurayplayer` 0.3.0: the player module answers two new calls, `guide` and `choose`: the ranked main playlist with why and how sure, its editions, a TV disc's episodes or the parts of its play-all playlist, the other playlists, warnings and inconsistencies. A playlist the viewer asks for plays as a title with the engine's chapters even when no index title names it, and a disc whose program plays nothing starts with the film. The libbluray second opinion is never asked for.

- The main feature is chosen by a playlist ranking engine (`Analyze`, `RankPlaylists`, `Explain`, `Episodes`, `DefaultWeights`), shaped like WittyDVD's title ranking. It reads only unencrypted navigation and metadata. See `docs/PLAYLIST-HEURISTICS.md`.
  - Among playlists of the same clips in different orders (playlist obfuscation), the film's order is found from how each clip joins the next: the clock carrying on, the clip's packet map, where the clips lie on an image, and last the clip numbers. Orders that follow on equally well are called ambiguous rather than guessed.
  - Decoys that repeat clips, shuffle a subset, pad with fake half-second clips, name missing or empty clips or run past a clip's end are told apart; they are grouped into clusters.
  - Each cut of a film with several (seamless branching) is listed in `Editions`; a TV disc's episodes come in viewing order, with where each starts in the play-all playlist (`PlayAllParts`).
  - Every answer has its reasons (`Candidate.Evidence`, `Ranking.Why`) and a confidence (`Confidence`, `ConfidenceLabel`).
  - A ranking no longer compares every playlist with every other: 2,000 playlists open and rank in about 50 ms on a desktop processor.
- `MovieObject.bdmv` is read statically by abstract interpretation (`ObjectTargets`, `StaticReach`, `FirstPlayReach`, `TitleReach`): registers are followed as sets of constants through arithmetic, comparisons, jumps and calls, with a budget of work, so a playlist chosen by a register is found. `Reach.Certain`, `Reach.Incomplete` and `NavTarget.Alternatives`, `Conditional`, `Dead` are new.
- BD-J discs: the JAR files' class constants are read for the playlists they name (`BDJStaticHints`, `ClassConstants`). Nothing is run.
- `Playlist.Chapters()` maps a playlist's entry marks onto its playing time across play items.
- `Disc.Consistency()` checks playlists, clip information and stream file sizes against each other; a copy cut short is flagged with how much of each clip can play.
- Parsers: every length is checked (including on 32-bit builds, where an offset could wrap); no room is made from a count a file gives; a damaged stream table, mark table, sub-path, extension block, program table or entry-point map is left out and named in `Playlist.Warnings` or `Clip.Warnings` instead of losing the file. The entry-point map's fine times were read one bit short. Playlists' extension data (3D stream table, extra sub-paths) is read. Play items with the same stream table share one parsed copy, and a disc has a budget of what it may hold in memory.
- BACKUP: a structure that parses but contradicts the disc (a playlist naming clips that are not there, an index naming movie objects that are not there) is taken from its `BACKUP` copy when that one does not. `Disc.FromBackup` and `Disc.Notes` say what was read from where.
- A copy with no `index.bdmv` and no `MovieObject.bdmv` (`Disc.MovieOnly`) is no longer reported as damaged. `OpenSource` opens a disc from any `Source`. Small files that UDF keeps inside their file entry are read. `Disc.Meta()` reads the disc library XML.
- libbluray: the optional cgo binding behind a build tag is replaced by a loader for a libbluray the user has installed, at run time and without cgo (macOS and Windows; Linux with `-tags libbluray`). It is only ever a second opinion (`Ranking.SecondOpinion`), never required, never given a disc that is encrypted or has an AACS or BD+ folder, and cannot read streams. `WITTYBLURAY_LIBBLURAY=off` turns it off.
- The helper answers `ranking`, `chapters`, `consistency` and `capabilities`; `wittybluray rank <disc>` prints the ranking with its reasons; the diagnoser shows the confidence, the editions and the cross-checks.
- Tests: a synthetic BDMV generator and UDF 2.50 image writer, every obfuscation pattern ranked blind, properties (adding decoys or renumbering never changes the feature), a fuzzer for each parser.
- No decryption, as before: encrypted discs are still refused.

- The copy for WittyMediaPlayer (`wittyblurayplayer`) goes to 0.2.0, on module API 3.
- The helper says which Java a server would use without running it (`javafind`), and a Java that was tried and failed is an answer rather than an error (`cantRun`), so a server can keep what it printed off the page. Which Java is only ever what the server's config file names.
- The helper is now the only way a server reaches WittyBluray, with no copy compiled in. A disc's Java programs run in the helper too, and it asks for permission to start programs and to write to a folder of its own.
- WittyBluray is now a project of its own, developed and released from this repository.
