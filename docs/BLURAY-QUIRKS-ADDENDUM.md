# Blu-ray disc quirks: additions for the server's reference

Text for `docs/BLURAY-QUIRKS.md` in WittyMediaServer, kept here because that file lives in the server's repository. It follows that file's layout and its **Tested** marks: *Unit* means a Go test built from synthetic data in the real file format; *None* means written from the format with nothing to test against. No retail disc was available for any of it.

## Playlists (replaces "Finding the feature among decoys")

Some discs carry hundreds or thousands of playlists built to defeat "longest wins". WittyBluray scores every playlist, in minutes of distinct video, and adds or takes away points for what it finds. Each answer comes with its reasons, a confidence (high, medium or low) and the sentences behind it; the diagnose report and `wittybluray rank` show them.

| Pattern | What a disc does | What WittyBluray does | Tested |
|---|---|---|---|
| Permuted decoys | Hundreds or thousands of playlists of the film's clips in other orders, all the same length | Playlists with the same segments form a cluster. Each join between two clips is weighed: the clock carrying on, the clip's packet map, where the clips lie on an image, and last the clip numbers. The best order stands; the rest are decoys | Unit (1,000 decoys, each kind of evidence alone) |
| Nothing tells the orders apart | The same clips in orders that follow on equally well, and no program that can be read | Says so: the playlists are marked ambiguous, the lowest-numbered is picked, the confidence is low and the report warns | Unit |
| The real order only in the HDMV program | First play computes a number into a register; a title compares it against every playlist number | The program is read without running it: registers are followed as sets of constants through arithmetic, comparisons, jumps and calls. The playlist the register selects is the one found | Unit |
| The real order only in a Java program | A BD-J title picks the playlist | Nothing is run. The JAR files' class constants are read for playlist locators, five-digit names and numbers. A program that computes or scrambles them shows nothing, and the structure decides with lower confidence | Unit |
| Repeated clips | A decoy plays clips again to run longer than the film | Each stretch of a clip counts once | Unit |
| Fake short clips | Clips of a fraction of a second between the real ones | A copy padded this way is a scrambled copy by playing time; a playlist mostly of such pieces is segment spam | Unit |
| Shuffled subsets | Some of the film's clips, out of order | Decoys: they go back on the disc and mostly play another playlist's video | Unit |
| Missing or empty clips | A playlist names a clip with no information or no stream file, or times past a clip's end | Never chosen ahead of one that plays | Unit |
| Two cuts of a film | Theatrical and extended cuts share most clips (seamless branching) | Each cut is listed as an edition with its length and what only it has. The one the disc's program plays is the main one; without a program, the longer | Unit |
| A cut that is only a subset | A playlist of some of the film's clips in order | Cannot be told from a shorter cut by structure: listed as an edition, never picked over the full film unless the program plays it | Unit |
| Multi-angle | A play item with several clips | The first angle is scored; the count is reported | Unit |
| TV discs | A playlist per episode, often numbered out of order, and one that plays them all | Episodes are the largest run of clean playlists of similar length; their order is where each starts in the play-all playlist, which is split at those points | Unit |
| TV disc with only a play-all | One playlist of several whole clips of even length | Its parts are offered as a guess, with a warning | Unit |
| Forced trailers | First play runs trailers before the film | Short playlists passed on the way are intros; the first long one reached is the feature | Unit |
| 3D twin | The same film as a 2D and a 3D playlist | The 2D one is preferred, as 3D plays here as 2D. The 3D stream table and the extra sub-paths in the playlist's extension data are read | Unit |
| Ultra HD | HEVC, HDR10, HDR10+, a Dolby Vision layer; clips of tens of gigabytes | Reported per playlist; sizes do not wrap | Unit |

None of this decrypts anything. It reads only the parts of a disc that are never encrypted, and a disc protected with AACS or BD+ is still refused, whatever its playlists score.

What is not read for ranking: the commands of menu buttons (they are in the stream files), and BD-J programs as they run. A film that only a menu button starts is found by title programs and structure.

## Chapters

| Feature | What WittyBluray does | Tested |
|---|---|---|
| Marks across play items | A mark names a play item and a time in its clip. A chapter's start is the play items before it plus how far into its own item it lies | Unit |
| Marks out of order, twice at one instant, outside their item, or naming an item that isn't there | Sorted; counted once; moved to the item's nearer end; left out | Unit |
| Link points | Not chapters. Commands can still reach them | Unit |

## File layouts and damage (additions)

| Feature | What WittyBluray does | Tested |
|---|---|---|
| A structure that parses but is wrong | A playlist naming a clip that isn't there, or an index naming movie objects that aren't there, is taken from `BACKUP` when the backup doesn't have the fault. The report says which files came from where | Unit |
| A backup that differs | Remarked on. The first copy is used when it reads well | Unit |
| A file damaged part of the way | A stream table, mark table, sub-path, extension block, program table or entry-point map that is cut off or runs past its length is left out and named; the rest of the file is kept | Unit, fuzzed |
| Movie-only copies | A folder with playlists, clip information and streams but no `index.bdmv` or `MovieObject.bdmv` plays from its playlists, and isn't called damaged | Unit |
| A copy cut short | A stream file shorter than its clip information counts is flagged, with how much of the clip the bytes there can play | Unit |
| Hostile sizes | No count or length from a file is trusted: an entry-point map that claims billions of entries, or a movie object file that claims gigabytes of commands, reads what the file holds. A disc has a budget of what it may keep in memory | Unit, fuzzed |
| UDF 2.50 images | Metadata partition and its mirror, extended file entries, files in many extents past 4 GB, chained descriptors, images of Ultra HD size, small files kept inside their entry | Unit (images written by the tests) |
| UDF 2.60 pseudo-overwrite (BD-R written in sessions), sparable and virtual partitions | Not read: a pressed disc and a finished image don't use them | None |
| A folder of only `.m2ts` files | Not opened as a disc: there is no clip information to tell lengths by | None |

## libbluray as a second opinion

| Feature | What WittyBluray does | Tested |
|---|---|---|
| A libbluray the user installed | Loaded at run time when asked (never needed, never shipped, no cgo). Its playlists, lengths, chapters, angles and choice of feature are compared with WittyBluray's; differences are listed and lower the confidence, and WittyBluray's answer stands | Unit, with a stand-in library. Not run against a real libbluray |
| Encrypted discs | Never handed to it: a disc must read as clear and have no AACS and no BD+ folder. No key file is passed, and it cannot be asked to read streams | Unit |
| Where | 64-bit macOS and Windows. On Linux only in a build with the `libbluray` tag, because the released Linux program is a static binary | Unit (macOS) |
