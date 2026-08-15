package feeds

// Feed represents a podcast feed known to podsync.
type Feed struct {
	ID           int64
	Name         string
	URL          string
	ETag         string
	LastModified string
}
