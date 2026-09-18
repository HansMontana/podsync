package briefing

import (
	"fmt"

	"github.com/HansMontana/podsync/internal/config"
	"github.com/HansMontana/podsync/internal/episode"
	"github.com/HansMontana/podsync/internal/playback"
	"github.com/HansMontana/podsync/internal/selection"
	"github.com/HansMontana/podsync/internal/state"
)

type Plan struct {
	ID       string
	Title    string
	Episodes []episode.Episode
}

func Build(cfg config.Config, current state.State, briefingID string, playbackStates map[string]playback.State) (Plan, error) {
	var briefingConfig *config.Briefing
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
		episodes, err := selection.FeedOrdered(cfg, current, section.Feed, playbackStates, section.UnplayedOnly, section.Order)
		if err != nil {
			return Plan{}, fmt.Errorf("briefing %q section %q: %w", briefingID, section.Feed, err)
		}
		if len(episodes) > section.Limit {
			episodes = episodes[:section.Limit]
		}
		plan.Episodes = append(plan.Episodes, episodes...)
	}
	return plan, nil
}
