package device

import "github.com/HansMontana/podsync/internal/domain/catalog"

// PathResolver supplies durable device paths for synchronized episodes.
// Path naming remains owned by the technical media adapter.
type PathResolver interface {
	RelativePathFor(catalog.Episode) string
}
