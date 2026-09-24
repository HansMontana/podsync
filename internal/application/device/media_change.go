package device

import "strings"

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
