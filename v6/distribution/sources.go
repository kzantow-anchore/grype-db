package distribution

import (
	"context"
	"io"
)

type Latest struct {
	Listing ListingEntry
}

type Listing struct {
	Entries []ListingEntry
}

type ListingEntry struct {
}

type Source interface {
	Handles(url string) bool
	Push(ctx context.Context, url string, mediaType string, contentReader io.Reader) error
	Pull(ctx context.Context, url string) (io.Reader, error)
	List(ctx context.Context, filter string) ([]string, error)
}
