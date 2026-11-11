# Changelog

What changed in WittyBluray, newest first. What changed before it had a repository of its own is in the
[WittyMediaServer changelog](https://github.com/wittywilla/WittyMediaServer/blob/master/CHANGELOG.md).

## Unreleased

- The main feature is chosen by a playlist ranking engine (`RankPlaylists`, `Analyze`, `Explain`, `Episodes`, `DefaultWeights`), shaped like WittyDVD's title ranking. It reads only unencrypted navigation and metadata. It scores obfuscated discs by distinct versus total time, repeated and shuffled segments, missing or empty clips, times outside a clip's range, segment spam, the most natural order (ascending clips, continuity in time and in the clip's packet map), chapters, streams, 3D and angles, `.m2ts` sizes, and where first play or title 1 leads. `MainPlaylist` keeps its signature and uses it. See `docs/PLAYLIST-HEURISTICS.md`.
- A static reading of `MovieObject.bdmv` (`ObjectTargets`, `StaticReach`, `FirstPlayReach`, `TitleReach`) finds the playlists, objects and titles a program leads to without running it. BD-J discs are flagged and ranked by structure.
- TV discs: episodes of about the same length (±15 %) are listed in viewing order, with a play-all playlist when there is one.
- The diagnoser lists the ranked playlists with their reasons and warns about decoys.
- Optional `libbluray` package, only with `-tags libbluray` and cgo. It cross-checks the playlist list against libbluray's for the diagnoser. It never reads stream content and never loads libaacs or libbdplus. The default build is unchanged and pure Go.
- No decryption, as before: encrypted discs are still refused.

- The copy for WittyMediaPlayer (`wittyblurayplayer`) goes to 0.2.0, on module API 3.
- The helper says which Java a server would use without running it (`javafind`), and a Java that was tried and failed is an answer rather than an error (`cantRun`), so a server can keep what it printed off the page. Which Java is only ever what the server's config file names.
- The helper is now the only way a server reaches WittyBluray, with no copy compiled in. A disc's Java programs run in the helper too, and it asks for permission to start programs and to write to a folder of its own.
- WittyBluray is now a project of its own, developed and released from this repository.
