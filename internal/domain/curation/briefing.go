package curation

import (
	"fmt"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/HansMontana/podsync/internal/domain/playback"
)

type Plan struct {
	ID       string
	Title    string
	Episodes []catalog.Episode
}

func Build(cfg Config, current catalog.Catalog, briefingID string, playbackStates map[string]playback.State) (Plan, error) {
	var briefingConfig *Briefing
	for i := range cfg.Briefings {
		if cfg.Briefings[i].ID == briefingID {
			briefingConfig = &cfg.Briefings[i]
			break
		}
	}
	if briefingConfig == nil {
		return Plan{}, fmt.Errorf("briefing %q not found", briefingID)
	}

	plan := Plan{ID: briefingConfig.ID, Title: briefingConfig.Title}
	for _, section := range briefingConfig.Sections {
		episodes, err := FeedOrdered(cfg, current, section.Feed, playbackStates, false, section.Order)
		if err != nil {
			return Plan{}, fmt.Errorf("briefing %q section %q: %w", briefingID, section.Feed, err)
		}
		if len(episodes) > section.Limit {
			episodes = episodes[:section.Limit]
		}
		if section.UnplayedOnly {
			unplayed := episodes[:0]
			for _, episode := range episodes {
				if !playbackStates[episode.IdentityKey()].Consumed() {
					unplayed = append(unplayed, episode)
				}
			}
			episodes = unplayed
		}
		plan.Episodes = append(plan.Episodes, episodes...)
	}
	return plan, nil
}
