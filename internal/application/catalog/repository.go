package catalog

import domaincatalog "github.com/HansMontana/podsync/internal/domain/catalog"

// Repository persists the catalog owned by the application layer.
type Repository interface {
	Load() (domaincatalog.Catalog, error)
	Save(domaincatalog.Catalog) error
	Close() error
}
