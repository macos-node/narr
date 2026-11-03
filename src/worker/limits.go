package worker

import (
	"errors"
	"io"
)

// Ceilings on what a remote server may hand us.
//
// Every fetch here ends up in memory: the ReadAll sites hold the whole body,
// and the refresh path streams into a parser that builds its structures from
// it. The HTTP client caps *time* (30s overall, 10s dial and TLS) but not
// size, so a feed server answering with an endless body — hostile, or just
// misconfigured and serving a disk image — gets to push this process into
// swap for as long as the timeout allows. On a fast link that is a lot.
const (
	maxBodyBytes = 10 << 20 // feeds, discovery pages, reader-mode articles
	maxIconBytes = 1 << 20  // favicons, which have no business being larger
)

var errBodyTooLarge = errors.New("response body exceeds the maximum allowed size")

// capped fails the read once the cap is passed rather than truncating at it.
// Truncating is the tempting one-liner and it is worse than the bug: a feed cut
// off mid-document parses as a malformed feed, so the user is told their feed
// is broken when what happened is that it was too big.
func capped(r io.Reader, max int64) io.Reader {
	return &cappedReader{r: r, max: max}
}

type cappedReader struct {
	r    io.Reader
	max  int64
	read int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += int64(n)
	if c.read > c.max {
		return n, errBodyTooLarge
	}
	return n, err
}

func readAtMost(r io.Reader, max int64) ([]byte, error) {
	return io.ReadAll(capped(r, max))
}
