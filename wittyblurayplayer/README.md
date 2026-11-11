# WittyBluray for WittyMediaPlayer

WittyBluray as a program of its own, so WittyMediaPlayer can take a newer version without a new player. It's the same
engine, plus the session that turns its steps into what the window shows. A player also links this package as its
built-in copy, so it always works with nothing downloaded. See WittyMediaPlayer's `DESIGN.md`.

## What it tells the player about a disc

On top of the session calls every disc module answers, this one answers two about the engine's ranking of the disc
(`docs/PLAYLIST-HEURISTICS.md`). Nothing is decrypted to work them out, and a disc with AACS is still refused.

| Call | What it gives |
|---|---|
| `guide` | The playlist picked as the film, one sentence on why and the engine's own reasons, how sure the pick is (`confidence` from 0 to 1, and `level`: `high`, `medium` or `low`), the film's length and chapters, its editions (theatrical and extended cuts), a TV disc's episodes and its play-all playlist, the parts of a play-all playlist that has no playlists for its episodes, the other playlists a viewer might want instead, the engine's warnings, and where playlists, clip information and stream files disagree. Decoy playlists are counted, not listed |
| `choose` with `{"id": "..."}` | Plays one of the guide's choices by its `id`, and answers with a presentation, as `title` does |

A playlist the viewer asks for, by number or from the guide, plays as a title with its chapters and a seek bar, even when
no title of the disc's index names it. Chapters come from the engine (`Playlist.Chapters`): a mark that names no play item
is left out, and one outside its item is moved inside it.

The ranking never asks another reader of the disc for a second opinion here: the engine's optional comparison with a
`libbluray` the viewer has installed stays off.

A disc whose own program plays nothing when it starts (a copy of the film without its program, or a first play that
ends at once) starts with the playlist the engine picked, and says so, rather than with an empty screen.
