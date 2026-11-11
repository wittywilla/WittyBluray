# Playlist heuristics

How WittyBluray decides which playlist is the main feature, which are editions of it, which are episodes of a TV disc, and which are decoys, without playing the disc and without trusting any one thing the disc says. The code is in `heuristics.go`, `episodes.go`, `mobj_static.go`, `bdjstatic.go`, `consistency.go` and `chapters.go`. The API mirrors WittyDVD's title ranking (`docs/TITLE-HEURISTICS.md` there), so a server can treat DVDs and Blu-rays alike.

## No decryption

The engine reads only what a Blu-ray keeps unencrypted: the playlists (`PLAYLIST/*.mpls`), the clip information (`CLIPINF/*.clpi`), `index.bdmv`, `MovieObject.bdmv`, the BD-J objects and JAR files, the disc library XML, and the sizes and positions of the `.m2ts` files. It never reads stream content for ranking, and it has no AACS or BD+ code, no key database, and no libaacs or libbdplus. An encrypted disc is still refused by `Open`'s callers through `Disc.Encrypted`, `AACSFolder` and `BDPlus`, exactly as before. A ranking says nothing about whether a disc can play.

## API

Everything added since 0.3.0 is additive: no signature changed.

| Call | What it gives |
|---|---|
| `d.Analyze(RankOptions) *Ranking` | The full answer (below) |
| `d.RankPlaylists() []Candidate` | Every playlist scored, the pick first, then best first |
| `d.Explain() string` / `r.Explain()` | The ranking in words |
| `d.Episodes() []Episode` | A TV disc's episodes in viewing order, or nil |
| `d.MainPlaylist() *Playlist` | Unchanged signature: the ranking's `Main`, worked out once per opened disc |
| `pl.Chapters() []Chapter` | A playlist's chapters mapped onto its playing time |
| `d.Consistency() []Inconsistency` | Where playlists, clip information and stream files disagree |
| `d.StaticReach`, `FirstPlayReach`, `TitleReach`, `ObjectTargets` | The static reading of the HDMV program |
| `d.BDJStaticHints() *BDJHints`, `ClassConstants` | Playlists the Java program names, read from its class files |
| `d.Meta() *DiscMeta` | The disc library name and title names (`META/DL/bdmt_xxx.xml`) |
| `OpenSource(Source)` | Open a disc from any reader, not only a path |
| `d.FromBackup`, `d.Notes`, `d.MovieOnly` | How the copy was read |
| `DefaultWeights` | The weights `MainPlaylist` uses |

`Ranking` has `Candidates`, `Main`, `PlayAll`, `Episodes`, `Decoys`, `FirstPlay`, `TitleOne`, `BDJ`, `Warnings`, and now:

| Field | Meaning |
|---|---|
| `Confidence` (0 to 1), `ConfidenceLabel` (`high`, `medium`, `low`) | How sure the pick is |
| `Why` | Short sentences: why this playlist, and why this confidence. What a server shows |
| `Editions` | The cuts of a film with several, the main one included: playlist, length, time shared with the main cut, time of its own, label, whether the program plays it |
| `Clusters` | Groups of playlists that play the same segments, with how many are shuffled copies or duplicates, and whether the group is ambiguous |
| `PlayAllParts` | Where each episode begins inside a play-all playlist |
| `Chapters` | The main playlist's chapters |
| `Stereo3D`, `UHD`, `HDR` | The main playlist's video |
| `MovieOnly` | A copy with no disc program |
| `Program` | How the disc's program was read: `hdmv`, `bdj` or `none`; what first play is certain to play; unknown operands; whether the reading ran out of budget; Java classes read |
| `Inconsistencies` | Counts of `Consistency()` entries by kind |
| `SecondOpinion` | The comparison with libbluray, when there was one |

A `Candidate` has `Score`, `Class`, `Flags` and `Reasons` as before, the facts (`Duration`, `Distinct`, `Chapters`, `Items`, `Clips`, `Size`, `Audio`, `Subtitles`, `Languages`, `Angles`), and now `Evidence` (each reason as `{signal, points, detail}`), `Continuity` (-1 to 1), `Cluster` and `ClusterSize`, `Video` (for example `2160p HEVC HDR10+`) and `SubtitleLanguages`. Flags serialize to JSON as a list of names. New flags: `truncated`, `ambiguous`, `edition`, `bdjTarget`, `uhd`, `hdr`. Playlist numbers start at 0, so "none" is `-1`.

`RankOptions.ClipSizes` passes `.m2ts` sizes in (`-1` for a missing file) and `ClipOffsets` their positions on the disc; without them both are read from the disc's source once. `Weights` replaces `DefaultWeights`. `SecondOpinion` passes another reader's account in. `NoBDJ` skips reading JAR files.

Classes are `main`, `playAll`, `episode`, `decoy`, `duplicate`, `alternate`, `intro`, `stub` (under 15 s), `trailer` (under 4 min), `featurette` (under 45 min) and `extra`.

The helper answers the same through `ranking` (`shown: true` asks an installed libbluray too; `limit` is how many candidates come back, best first, 100 unless given, with `total` saying how many there were), `chapters` (`n`: the playlist), `consistency` (with `fromBackup`, `notes`, `damaged`, `movieOnly` and the disc library `meta`) and `capabilities` (which needs no disc open). These are new calls; no existing call changed. `wittybluray rank <disc> [json]` prints it.

## Which order is the film's

An obfuscated disc carries hundreds or thousands of playlists of the same clips in different orders, all the same length. Only the order tells the film from its decoys, so every join between two play items is weighed, from -1 to 1:

| Evidence at a join | Weight |
|---|---|
| The same clip and time base, continuing in time, or at the same entry point of the clip's packet map (`EP_map`) | 1 |
| Another clip whose time carries straight on (one encode cut into clips keeps its clock) | 1 |
| Another clip whose times lie wholly after the first's: the same clock with a gap | 0.5 |
| Another clip whose times lie wholly before | -0.8 |
| On an image: the very next clip on the disc / a later one / an earlier one | 0.8 / 0.5 / -0.8 |
| Nothing better: a whole clip after a whole clip of a higher number / a higher number / a lower number | 0.5 / 0.25 / -0.5 |
| An earlier part of the clip just played | -1 |

Clips that each start their clock afresh overlap in time and say nothing; then the layout decides on an image, and the clip numbers on a folder. A layout that is simply the order of the clips' names is not used: an image rebuilt from a folder of files is laid out that way whatever the film's order, so it says no more than the names do. A film's clips lie on the disc in playing order because a player cannot seek back between two seamless items. An item that claims to continue the last one's clock (connection condition 6) while its time does not follow on loses points.

Playlists with the same set of segments form a cluster. Within it they are rated by the share of joins that carry on (worth most), then by what the disc's program points at, then by the weak evidence; the best stands and the rest are `shuffled`. When more than one is best and nothing singles one out, they are all `ambiguous`, the lowest-numbered is picked, the confidence is low and a warning says so: the engine does not pretend to know.

## Signals and weights

Scores are in "minutes of distinct video". A weight of 15 is worth a quarter of an hour of unique segments.

| Signal | Weight | Flag |
|---|---|---|
| Distinct, readable video: each `(clip, time base, in, out)` counted once | +1 per minute | |
| A play item's clip has no `.clpi`, or its `.m2ts` is missing | -40 | `missingClip` |
| A play item's `.m2ts` is empty | -40 | `zeroSize` |
| Play time on clips whose `.m2ts` is under 98 % of the packets its `.clpi` counts | -30 × share | `truncated` |
| In/out times outside the clip's time base (`STC` range in CLPI) | -10, then -20 × share of items | `outOfRange` |
| Play time spent replaying segments (at least 20 %) | -30 × share | `repeats` |
| Joins that go back | -5, then -35 × share of joins | |
| Joins that go back inside one clip | -40 × share | |
| Joins that carry on (weight 0.75 or more above) | +5 × share of joins | |
| Joins that only go forward | +2 × share | |
| Connection condition 6 without a continuing clock | -3 | |
| Same segment set as a sibling, in a less natural order | -25 | `shuffled` |
| Mostly another playlist's video (by count or by playing time), with more back-jumps | -25 | `shuffled` |
| Same segments in the same order as a lower-numbered playlist | -0.5 | `duplicate` |
| Segment spam: 10+ items under 1 s making half the items, or 20+ items with half under 10 s | -30 | `segmentSpam` |
| Chapter marks naming play items that don't exist | -10 | `badChapters` |
| 3+ chapters of even length (spread under 0.8 of the mean, mean 1–20 min) | +3 | |
| Most audio streams / most PG streams / largest `.m2ts` total among the long playlists | +2 / +1 / +5 | |
| A 3D (MVC) playlist: the 2D twin is preferred, as 3D plays here as 2D | -1 | `stereo3D` |
| First play leads to it (static reading) | +15 | `firstPlay`, `menuTarget` |
| Title 1 leads to it | +10 | `titleOne`, `menuTarget` |
| Some title's HDMV program plays it, among 8 or fewer | +4 | `menuTarget` |
| The BD-J object of first play or title 1 lists it among 10 or fewer | +4 | `menuTarget` |
| The Java program has a locator or `.mpls` name for it / its five digits / pushes its number | +8 / +5 / +2 | `bdjTarget` |

**The pick.** Playlists are ordered in tiers before score: readable ones with video, then decoys with video, then audio-only ones, then audio-only decoys, then playlists only part of which reads (a last resort, with a warning). Ones with nothing readable are never picked. Ties go to more distinct time, then longer, then the lower number. When the top playlist is a longer cut of a film whose other cut is what first play or title 1 plays, the program's cut is picked and the longer one is listed as an edition.

**A decoy** is `shuffled`, `segmentSpam`, `missingClip` or `zeroSize`, runs past a clip's end, spends half its time on repeats, or goes back inside a clip at two joins or more making a quarter of its joins.

## The HDMV program, statically

`mobj_static.go` reads `MovieObject.bdmv` by abstract interpretation, without running it:

- Each general register is known as a set of up to 8 constants, or as "anything". `MOVE`, `SWAP`, arithmetic and bit operations are applied to every combination; `RND` of a small number gives its range.
- A comparison whose operands are known goes one way; any other goes both, and what an equality settles is known on the side where it holds. `GOTO` through a register goes to each value it may hold.
- Register values travel through `JUMP_OBJECT`, `CALL_OBJECT` and title jumps into the object they enter. First play starts with all registers zero, as on a disc just put in; a title entered by itself starts with all unknown.
- After a playlist that has a menu (an IG stream or a menu sub-path), and after a call, registers may be anything, so comparisons go both ways from there. After a playlist without one (a logo, a trailer) they are kept.
- A `PLAY_PL` through a register gives one target for each value, and is counted as unknown when the register is "anything". Commands no path reaches are listed as `Dead` and not followed.
- Work is counted (commands read, register pages merged): past about a million units, 512 objects or 2,048 visits, the reading stops and `Reach.Incomplete` says so. Every title's program together gets twice that.

`Reach.Certain` lists the playlists reached with no undecided comparison on the way and one value in the operand. The ranking takes the first playlist first play (then title 1) reaches with at least `min(20 min, half the longest)` of distinct time; shorter ones passed on the way are `intro`. A title whose program could play more than 8 playlists points at none of them.

Menu buttons are not read for ranking: their commands are in the stream files. So a film that only a menu button starts is found by title programs and structure, not by first play.

## BD-J, statically

When first play or title 1 is a BD-J object, nothing is run for ranking. `bdjstatic.go` opens the disc's JAR files as ZIP archives (at most 32 JARs, 48 MB compressed, 20,000 classes, 4 MB a class) and reads each class file's constant pool and bytecode for string constants that name a playlist (`bd://0.PLAYLIST:00800`, `00800.mpls`, or just `00800`) and for integers pushed or switched on that equal a playlist's number. A number only counts when the program pushes the numbers of at most three long playlists. A program that computes its playlist or scrambles its strings shows nothing, and then the structure decides and the confidence says so.

## Editions

A film with a theatrical and an extended cut keeps both as clean playlists sharing most clips, the longer with a few scenes in clips of its own. Editions are playlists that never go back, are at least half as long as the main one, and share at least half their video with it by clip. Up to 8 are listed, longest first, each with the time it shares and the time only it has. More than 8 is not a film with nine cuts: the rest are named as likely decoys that leave clips out. A decoy that plays a forward subset of the film's clips cannot be told from a shorter cut by structure; it is listed as an edition, never picked over the full film unless the program plays it.

## Episodes and play-all

- Candidates are clean playlists with video, 8–75 minutes long. The episodes are the largest run within ±15 % of the run's median, keeping the lowest-numbered playlist among any that overlap heavily.
- A play-all playlist lasts within ±15 % of the episodes' sum and covers 90 % of each episode's segments or clip time. Three episodes are needed without a play-all, and two with one.
- The order is where each episode's first clip appears in the play-all playlist, then by clip number, then by playlist number.
- `PlayAllParts` maps each episode onto the play-all playlist's items and times. A disc with only a play-all playlist (three or more whole clips of even, episode-like length, none carrying on from the last) gets its parts as a guess, with a warning; `Episodes` stays empty.

## Chapters

`pl.Chapters()` takes the entry marks (type 1; link points are not chapters), drops marks that name a play item the playlist does not have, moves a mark outside its item to the item's nearer end, sorts by playing time, counts two marks at one instant once, and gives each chapter its start and length in seconds of the playlist's playing time: the play items before the mark's item, plus how far into its own item the mark lies.

## Cross-checks

`d.Consistency()` compares what describes the same video three times:

| Kind | What disagrees |
|---|---|
| `clipMissing` | A play item names a clip with no `.clpi` |
| `streamMissing`, `streamEmpty` | A clip has information and no stream file, or an empty one |
| `streamShort` | The stream file is under 98 % of the packets the `.clpi` counts. `PlayableSec` is how much the bytes there can play, by the entry-point map |
| `streamLong`, `streamUnaligned` | More bytes than counted, or not a whole number of 192-byte packets |
| `timesOutside`, `emptyItem` | A play item's times lie outside the clip's time base, or end before they start |
| `stcOrder`, `entryPastEnd` | Time bases out of order, or an entry point past the last packet |
| `bitrate` | A clip that would average under 50 kbit/s or over 160 Mbit/s |
| `markNoItem`, `markOutside` | Chapter marks naming no item, or lying outside theirs |
| `angleMissing`, `subClipMissing` | An angle or a sub-path names a clip with no information |

Repair, where there is something to repair with: every structure falls back on its `BACKUP` copy when it is missing or does not parse (as before), and now also when it parses but contradicts the disc and the backup does not: a playlist that names clips the disc lacks or times past a clip's end, an index that names movie objects `MovieObject.bdmv` does not have. `d.FromBackup` and `d.Notes` say what was taken from where. A backup that merely differs from a good first copy is remarked on and not used.

## Bounds

- Every length, count and address from a file is checked before use; no room is made from a count the file gives; a damaged part of a file (stream table, marks, sub-paths, extension data, program table, entry-point map) is left out and named in `Playlist.Warnings` or `Clip.Warnings` while the rest is kept.
- A disc may hold in memory at most 20,000 playlists, 400,000 play items, a million marks, a million streams in distinct stream tables, 20,000 clips and 8 million entry points; what is past that is left out and named in `d.Notes`. Play items with the same stream table share one parsed copy, so a thousand decoys cost little more than their play items.
- Ranking does not compare every playlist with every other: same-segment groups go by hash, scrambled copies are sought against 24 reference playlists. 2,000 playlists of 30 items open and rank in about 50 ms on a desktop processor with some 33 MB kept; 8,000 take four times that.

## Confidence

`Confidence` starts at 0.5 and moves with the evidence, each step with its sentence in `Why`: the disc's program playing it (+0.1 to +0.25), the margin over the next different playlist (+0.2 to -0.15), the order evidence inside its cluster (+0.12, or -0.35 when ambiguous), faults in the playlist itself (-0.2), several cuts (-0.05), parts of the program unread (-0.05), a Java program that names nothing (-0.1), and a second opinion (+0.1 agreeing, -0.2 disagreeing). 0.75 and up is `high`, 0.5 and up `medium`.

## Optional: libbluray as a second opinion

WittyBluray is an original implementation and contains no libbluray code. The package `libbluray/` looks for a libbluray shared library that the user has installed, loads it at run time (through purego: `dlopen` or `LoadLibraryEx`, no cgo and nothing at build time), and asks it for its list of playlists with lengths, chapters and angles, and for the title it would play as the feature. The answer is compared with WittyBluray's (`libbluray.Analyze`, `CrossCheck`, `Compare`) and surfaces in `Ranking.SecondOpinion`, `Why` and `Confidence`. WittyBluray's pick is never replaced by it.

- **Never required.** `libbluray.Capability()` says whether one was found, its version and path, or why not. `WITTYBLURAY_LIBBLURAY=off` turns loading off; a path there names the one file to load.
- **Where it is built.** macOS and Windows (64-bit) always. Linux only with `-tags libbluray` (still no cgo): a loader makes a Go program depend on the machine's C library, and the release's Linux helper is a static binary that runs on any distribution. Elsewhere, and with `-tags nolibbluray`, a stub reports it unavailable.
- **Never for decryption.** A disc is handed over only when its streams are clear and it has no `AACS` and no `BDSVM` folder at all, which is when libbluray has no reason to load libaacs or libbdplus; `LIBAACS_PATH` and `LIBBDPLUS_PATH` are pointed at a path that does not exist besides. No key file is passed. Only eight functions are bound (`bd_get_version`, `bd_init`, `bd_open_disc`, `bd_get_titles`, `bd_get_main_title`, `bd_get_title_info`, `bd_free_title_info`, `bd_close`); `bd_read` cannot be called from this code.
- **Only on request.** Opening or ranking a disc never loads the library. The helper asks it when a `ranking` call says `shown: true`; the diagnoser and `wittybluray rank` ask when one is there.

## Limits

- A static reading isn't a run. Programs that test player registers, wait for a menu button, or compute what a set of 8 constants cannot follow can lead elsewhere; such targets are counted and reported, not guessed. The dry-run VM in `nav.go` is the better guide for a disc that plays; the ranking is for choosing without playing.
- Weights and patterns were tuned on synthetic discs that model the published obfuscation patterns (`synth_test.go`), not on a corpus of retail discs. Whether a given studio's discs carry a clock through their clips, or number their clips in playing order, is not known from a disc in hand.
- Menu button commands and BD-J programs are not run for ranking.
- A folder copy has no disc layout to read; an image has.
