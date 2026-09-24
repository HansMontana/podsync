package device

import (
	"fmt"
	"strings"
)

type EpisodeSyncResult struct {
	MediaChanged bool
}

func mediaChanged(plan FilePlan) bool {
	if len(plan.Copies) > 0 {
		return true
	}
	for _, relative := range plan.Deletes {
		if strings.HasPrefix(relative, "Podcasts/") {
			return true
		}
	}
	return false
}

func prepareMediaMutation(plan FilePlan, options EpisodeSyncOptions) error {
	if !mediaChanged(plan) || options.BeforeMediaMutation == nil {
		return nil
	}
	if err := options.BeforeMediaMutation(); err != nil {
		return fmt.Errorf("prepare media mutation: %w", err)
	}
	return nil
}
