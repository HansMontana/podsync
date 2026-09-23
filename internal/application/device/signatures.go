package device

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

func saveEpisodeSignatures(deviceRoot string, episodes []catalog.Episode, skipped map[string]struct{}, resolver PathResolver, files DeviceFiles) error {
	signatures := make(map[string]string, len(episodes))
	for _, current := range episodes {
		relative := resolver.RelativePathFor(current)
		if _, skip := skipped[relative]; skip {
			continue
		}
		signatures[relative] = mediaSignature(current)
	}
	if err := files.SaveMediaSignatures(deviceRoot, signatures); err != nil {
		return fmt.Errorf("save media signatures: %w", err)
	}
	return nil
}

func mediaSignature(current catalog.Episode) string {
	value := fmt.Sprintf("%s\x00%s\x00%d", current.Enclosure.URL, current.Enclosure.Type, current.Enclosure.Length)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
