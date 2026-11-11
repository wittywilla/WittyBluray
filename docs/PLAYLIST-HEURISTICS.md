# Playlist heuristics

How WittyBluray decides which playlist is the main feature, which are episodes of a TV disc, and which are decoys. The code is in `heuristics.go`, `episodes.go` and `mobj_static.go`. The API mirrors WittyDVD's title ranking (`docs/TITLE-HEURISTICS.md` there), so a server can treat DVDs and Blu-rays alike.

## No decryption

The engine reads only what a Blu-ray keeps unencrypted: the playlists (`PLAYLIST/*.mpls`), the clip information (`CLIPINF/*.clpi`), `index.bdmv`, `MovieObject.bdmv`, whether BD-J objects are present, and the sizes of the `.m2ts` files. It never reads stream content for ranking, and it has no AACS or BD+ code, no key database, and no libaacs or libbdplus. An encrypted disc is still refused by `Open`'s callers through `Disc.Encrypted`, `AACSFolder` and `BDPlus`, exactly as before. A ranking says nothing about whether a disc can play.

## API

| Call | What it gives |
|---|---|
| `d.RankPlaylists() []Candidate` | Every playlist scored, the pick first, then best first |
| `d.Analyze(RankOptions) *Ranking` | The full answer: candidates, `Main`, `PlayAll`, `Episodes`, `Decoys`, `FirstPlay`, `TitleOne`, `BDJ`, `Warnings` |
| `d.Explain() string` / `r.Explain()` | The ranking in words, with each candidate's reasons |
| `d.Episodes() []Episode` | A TV disc's episodes in viewing order, or nil |
| `d.MainPlaylist() *Playlist` | Unchanged signature: the ranking's `Main`, worked out once per opened disc |
| `DefaultWeights` | The weights `MainPlaylist` uses |
| `d.StaticReach`, `FirstPlayReach`, `TitleReach`, `ObjectTargets` | The static reading of the HDMV program |

A `Candidate` has `Score`, `Class`, `Flags` and `Reasons`, the same as WittyDVD's, plus playlist facts: `Duration`, `Distinct`, `Chapters`, `Items`, `Clips`, `Size`, `Audio`, `Subtitles`, `Languages` and `Angles`. Flags serialize to JSON as a list of names, for example `["main","firstPlay"]`. Playlist numbers start at 0, so "none" is `-1` in `Ranking`.

`RankOptions.ClipSizes` passes `.m2ts` sizes in. Use `-1` for a missing file. Without it, the sizes are read from the disc's source once. `RankOptions.Weights` replaces `DefaultWeights`.

Classes are `main`, `playAll`, `episode`, `decoy`, `duplicate`, `alternate`, `intro`, `stub` (under 15 s), `trailer` (under 4 min), `featurette` (under 45 min) and `extra`.

## Signals and weights

Scores are in "minutes of distinct video". A weight of 15 is worth a quarter of an hour of unique segments.

| Signal | Weight | Flag |
|---|---|---|
| Distinct, readable video: each `(clip, time base, in, out)` counted once | +1 per minute | |
| A play item's clip has no `.clpi`, or its `.m2ts` is missing | -40 | `missingClip` |
| A play item's `.m2ts` is empty | -40 | `zeroSize` |
| In/out times outside the clip's time base (`STC` range in CLPI) | -10, then -20 × share of items | `outOfRange` |
| Play time spent replaying segments (at least 20 %) | -30 × share | `repeats` |
| Joins that go back: a lower clip number, or earlier in the same clip, without continuing in time | -5, then -35 × share of joins | |
| Joins that continue: the next item starts where the last ended in time, or at the same entry point of the clip's packet map (SPN continuity), or a whole clip follows a whole clip of a higher number | +5 × share of joins | |
| Same segment set as a sibling, in a less natural order | -25 | `shuffled` |
| Mostly another playlist's segments, with more back-jumps | -25 | `shuffled` |
| Same segments in the same order as a lower-numbered playlist | -0.5 | `duplicate` |
| Segment spam: 10+ items under 1 s making half the items, or 20+ items with half under 10 s | -30 | `segmentSpam` |
| Chapter marks naming play items that don't exist | -10 | `badChapters` |
| 3+ chapters of even length (spread under 0.8 of the mean, mean 1–20 min) | +3 | |
| Most audio streams among the long playlists | +2 | |
| Most PG subtitle streams among the long playlists | +1 | |
| Largest total `.m2ts` size among the long playlists | +5 | |
| A 3D (MVC) dependent view: the 2D twin is preferred, as 3D plays here as 2D | -1 | `stereo3D` |
| Multi-angle item | 0 | `multiAngle` |
| First play leads to it (static reading) | +15 | `firstPlay`, `menuTarget` |
| Title 1 leads to it (static reading) | +10 | `titleOne`, `menuTarget` |
| Some title's HDMV program plays it | +4 | `menuTarget` |
| The BD-J object of first play or title 1 lists it among 10 or fewer playlists | +4 | `menuTarget` |

"Long playlists" are readable ones with at least half the longest distinct time.

**The pick.** Playlists are ordered in tiers before score: readable ones with video, then decoys with video, then audio-only ones, then audio-only decoys. Unreadable ones come last and are never picked. "Unreadable" means no items, no distinct time, a clip without information, or an item past its clip's end. A playlist whose `.m2ts` is missing or empty is still described, but it counts as a decoy. Ties go to more distinct time, then longer, then the lower number.

**A decoy** is `shuffled`, `segmentSpam`, `missingClip` or `zeroSize`, runs past a clip's end, or spends half its time on repeats.

**The most natural order.** Obfuscated discs carry many siblings of the same length that differ only in segment order. All playlists with the same set of segments are compared by the share of their joins that continue (time or SPN) and go forward. The highest share stands, and the rest are marked `shuffled`.

## The HDMV program, statically

`MovieObject.bdmv` was already parsed (`mobj.go`) and decoded (`vm.go`). `mobj_static.go` reads it without running it:

- `ObjectTargets` lists every `PLAY_PL`, `PLAY_PL_PI`, `PLAY_PL_PM`, `JUMP_OBJECT`, `JUMP_TITLE`, `CALL_OBJECT` and `CALL_TITLE` in program order. A register operand is resolved when an earlier `MOVE GPRn, constant` was the last write to it. Otherwise the target is `-1` and is counted as unknown.
- `StaticReach` follows an index entry depth first through object jumps and, optionally, title jumps. Commands under a condition count whichever way the condition would go. It never visits more than 512 objects, so a loop ends.
- The ranking takes the first playlist that first play (then title 1) reaches with at least `min(20 min, half the longest)` of distinct time. Shorter playlists passed on the way are marked `intro`.

When first play or title 1 is a **BD-J** object, it isn't read. `Ranking.BDJ` is set, a warning says the structure alone decided, and only the BD-J object's playlist list (when it names 10 or fewer) adds a small weight.

## Episodes

These are WittyDVD's rules:

- Candidates are clean playlists with video, 8–75 minutes long.
- The episodes are the largest run within ±15 % of the run's median, keeping the lowest-numbered playlist among any that overlap heavily.
- A play-all playlist lasts within ±15 % of the episodes' sum and covers 90 % of each episode's segments or clip time.
- Three episodes are needed without a play-all, and two with one.
- The order is where each episode's first clip appears in the play-all playlist, then by clip number, then by playlist number.

## Limits

- A static reading isn't a run. Programs that compute playlist numbers, test player registers, or wait for a menu button can lead elsewhere. A register target that can't be resolved is counted and reported, not guessed. The dry-run VM in `nav.go` is the better guide for a disc that plays; the ranking is for choosing without playing.
- BD-J programs aren't run for ranking, so BD-J discs are ranked by structure.
- Weights were tuned on synthetic discs that model the published obfuscation patterns, not on a corpus of retail discs.
- Seamless-branching discs (several cuts sharing segments) give `alternate` playlists. The pick is the cut with the most distinct time unless the program leads elsewhere.

## Optional: libbluray cross-check

The package `libbluray/` compares libbluray's reading of a disc's navigation with WittyBluray's (`CrossCheck(disc) []Discrepancy`). It is built only with `-tags libbluray`, cgo and pkg-config's `libbluray`. Without them, a stub reports it unavailable, and `go build ./...` needs nothing. It calls only `bd_open_disc` with no key file, `bd_get_disc_info`, `bd_get_meta`, `bd_get_titles` and `bd_get_title_info`, never `bd_read`.

Before each open it sets `LIBAACS_PATH` and `LIBBDPLUS_PATH` to a path that doesn't exist (`libbluray.NoLibraryPath`), so libbluray can't load libaacs or libbdplus. If it still reports one as loaded, the result is thrown away. An encrypted disc is refused before libbluray sees it.
