package diagnose

import "github.com/wittywilla/wittybluray"

// libblurayCrossCheck is set by libbluray.go when the diagnoser is built
// with the libbluray tag; nil otherwise, and the section is left out.
var libblurayCrossCheck func(j *Job, d *wittybluray.Disc)
