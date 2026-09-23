package metadata

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HansMontana/podsync/internal/domain/catalog"
	"github.com/bogem/id3v2/v2"
)

const genre = "Podcast"

// NeedsNormalization reports whether an MP3 is missing any podcast metadata.
// It only reads the ID3 header and tag, not the audio payload.
func NeedsNormalization(path string, current catalog.Episode) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open MP3: %w", err)
	}
	var header [3]byte
	_, readErr := io.ReadFull(file, header[:])
	_ = file.Close()
	if readErr != nil {
		return false, fmt.Errorf("read MP3 header: %w", readErr)
	}
	if string(header[:]) != "ID3" {
		return true, nil
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return false, fmt.Errorf("open ID3 tag: %w", err)
	}
	defer tag.Close()
	return hasMissingFields(tag, current), nil
}

// NormalizeMP3 fills missing podcast metadata fields on an MP3 file.
// It returns whether the file was changed.
func NormalizeMP3(path, feedName string, current catalog.Episode) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open MP3: %w", err)
	}
	var header [3]byte
	_, readErr := io.ReadFull(file, header[:])
	_ = file.Close()
	if readErr != nil {
		return false, fmt.Errorf("read MP3 header: %w", readErr)
	}

	if string(header[:]) != "ID3" {
		tag := id3v2.NewEmptyTag()
		tag.SetVersion(4)
		setFields(tag, feedName, current)
		if err := prependTag(path, tag); err != nil {
			return false, err
		}
		return true, nil
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return false, fmt.Errorf("open ID3 tag: %w", err)
	}
	defer tag.Close()

	if strings.TrimSpace(feedName) == "" {
		feedName = "Podcast"
	}
	if !hasMissingFields(tag, current) {
		return false, nil
	}

	tag.SetVersion(4)
	setFields(tag, feedName, current)
	if err := tag.Save(); err != nil {
		return false, fmt.Errorf("save ID3 tag: %w", err)
	}
	return true, nil
}

func setFields(tag *id3v2.Tag, feedName string, current catalog.Episode) {
	if strings.TrimSpace(tag.Title()) == "" {
		tag.SetTitle(safeText(current.Title))
	}
	if strings.TrimSpace(tag.Album()) == "" {
		tag.SetAlbum(safeText(feedName))
	}
	if strings.TrimSpace(tag.Artist()) == "" {
		artist := strings.TrimSpace(current.Author)
		if artist == "" {
			artist = feedName
		}
		tag.SetArtist(safeText(artist))
	}
	if strings.TrimSpace(tag.Genre()) == "" {
		tag.SetGenre(genre)
	}
	year := ""
	if !current.PublishedAt.IsZero() {
		year = strconv.Itoa(current.PublishedAt.UTC().Year())
	}
	if strings.TrimSpace(tag.Year()) == "" {
		tag.SetYear(year)
	}
}

func safeText(value string) string {
	return strings.ToValidUTF8(value, "\uFFFD")
}

func hasMissingFields(tag *id3v2.Tag, current catalog.Episode) bool {
	return strings.TrimSpace(tag.Title()) == "" ||
		strings.TrimSpace(tag.Album()) == "" ||
		strings.TrimSpace(tag.Artist()) == "" ||
		strings.TrimSpace(tag.Genre()) == "" ||
		(strings.TrimSpace(tag.Year()) == "" && !current.PublishedAt.IsZero())
}

func prependTag(path string, tag *id3v2.Tag) error {
	input, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open MP3 for tagging: %w", err)
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(path), ".podsync-tagged-*")
	if err != nil {
		return fmt.Errorf("create tagged MP3: %w", err)
	}
	temporaryPath := output.Name()
	defer os.Remove(temporaryPath)
	if _, err := tag.WriteTo(output); err != nil {
		_ = output.Close()
		return fmt.Errorf("write ID3 tag: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("copy MP3 audio: %w", err)
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return fmt.Errorf("sync tagged MP3: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close tagged MP3: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install tagged MP3: %w", err)
	}
	return nil
}
