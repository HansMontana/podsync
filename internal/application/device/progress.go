package device

import "github.com/HansMontana/podsync/internal/domain/catalog"

type preparationProgress struct {
	total     int
	completed int
	resolver  PathResolver
	seen      map[string]struct{}
	report    ProgressFunc
}

func newPreparationProgress(total int, resolver PathResolver, report ProgressFunc) *preparationProgress {
	return &preparationProgress{
		total:    total,
		resolver: resolver,
		seen:     make(map[string]struct{}, total),
		report:   report,
	}
}

func (p *preparationProgress) Report(_ int, _ int, current catalog.Episode, reused bool) {
	if p.report == nil {
		return
	}
	key := p.resolver.RelativePathFor(current)
	if _, exists := p.seen[key]; exists {
		return
	}
	p.seen[key] = struct{}{}
	p.completed++
	p.report(p.completed, p.total, current, reused)
}
