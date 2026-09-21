package metadata

import (
	"os"
	"testing"
	"time"

	"github.com/HansMontana/podsync/internal/episode"
	"github.com/bogem/id3v2/v2"
)

func TestNormalizeMP3WritesAndReusesCanonicalFields(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	episodeValue := episode.Episode{Title: "Episode title", PublishedAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)}
	changed, err := NormalizeMP3(path, "Example Podcast", episodeValue)
	if err != nil {
		t.Fatalf("NormalizeMP3() returned error: %v", err)
	}
	if !changed {
		t.Fatal("NormalizeMP3() reported no change for an untagged file")
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	if tag.Title() != "Episode title" || tag.Album() != "Example Podcast" || tag.Artist() != "Example Podcast" || tag.Genre() != "Podcast" || tag.Year() != "2026" {
		t.Fatalf("got metadata title=%q album=%q artist=%q genre=%q year=%q", tag.Title(), tag.Album(), tag.Artist(), tag.Genre(), tag.Year())
	}
	_ = tag.Close()

	changed, err = NormalizeMP3(path, "Example Podcast", episodeValue)
	if err != nil {
		t.Fatalf("second NormalizeMP3() returned error: %v", err)
	}
	if changed {
		t.Fatal("second NormalizeMP3() reported a change")
	}
}

func TestNormalizeMP3UsesEpisodeAuthorForArtist(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := NormalizeMP3(path, "Example Podcast", episode.Episode{Title: "Episode", Author: "Episode Author"})
	if err != nil || !changed {
		t.Fatalf("NormalizeMP3() changed=%v error=%v", changed, err)
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tag.Close()
	if tag.Artist() != "Episode Author" {
		t.Fatalf("artist = %q", tag.Artist())
	}
}

func TestNormalizeMP3SupportsUnicodeEpisodeTitles(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	title := `Immersive Remix: "Fantaisie Impromptu No. 4 in C#min, Op. 66” by Carlos Hernandez`
	changed, err := NormalizeMP3(path, "Example Podcast", episode.Episode{Title: title})
	if err != nil || !changed {
		t.Fatalf("NormalizeMP3() changed=%v error=%v", changed, err)
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tag.Close()
	if tag.Title() != title {
		t.Fatalf("title = %q, want %q", tag.Title(), title)
	}
}

func TestNormalizeMP3UpgradesIncompleteTagsForUnicode(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	tag := id3v2.NewEmptyTag()
	tag.SetVersion(3)
	tag.SetAlbum("Example Podcast")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tag.WriteTo(file); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("audio"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	title := `Immersive Remix: "Fantaisie Impromptu No. 4 in C#min, Op. 66” by Carlos Hernandez`
	changed, err := NormalizeMP3(path, "Example Podcast", episode.Episode{Title: title})
	if err != nil || !changed {
		t.Fatalf("NormalizeMP3() changed=%v error=%v", changed, err)
	}
	parsed, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	if parsed.Title() != title {
		t.Fatalf("title = %q, want %q", parsed.Title(), title)
	}
}

func TestNormalizeMP3ReplacesInvalidUTF8(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := NormalizeMP3(path, "Example Podcast", episode.Episode{Title: string([]byte{'E', 'p', 'i', 's', 'o', 'd', 'e', 0xff})})
	if err != nil || !changed {
		t.Fatalf("NormalizeMP3() changed=%v error=%v", changed, err)
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tag.Close()
	if tag.Title() != "Episode\uFFFD" {
		t.Fatalf("title = %q", tag.Title())
	}
}

func TestNormalizeMP3PreservesExistingMetadata(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	tag := id3v2.NewEmptyTag()
	tag.SetVersion(3)
	tag.SetTitle("Publisher title")
	tag.SetAlbum("Publisher album")
	tag.SetArtist("Publisher artist")
	tag.SetGenre("Science")
	tag.SetYear("2025")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tag.WriteTo(file); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("audio"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	changed, err := NormalizeMP3(path, "Configured podcast", episode.Episode{Title: "Feed title"})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("NormalizeMP3() changed complete existing metadata")
	}

	parsed, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	if parsed.Title() != "Publisher title" || parsed.Album() != "Publisher album" || parsed.Artist() != "Publisher artist" || parsed.Genre() != "Science" || parsed.Year() != "2025" {
		t.Fatalf("existing metadata changed: title=%q album=%q artist=%q genre=%q year=%q", parsed.Title(), parsed.Album(), parsed.Artist(), parsed.Genre(), parsed.Year())
	}
}

func TestNeedsNormalizationReadsTagsWithoutChangingAudio(t *testing.T) {
	path := t.TempDir() + "/episode.mp3"
	tag := id3v2.NewEmptyTag()
	tag.SetVersion(3)
	tag.SetTitle("Publisher title")
	tag.SetAlbum("Publisher album")
	tag.SetArtist("Publisher artist")
	tag.SetGenre("Science")
	tag.SetYear("2025")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tag.WriteTo(file); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("audio payload"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	needs, err := NeedsNormalization(path, episode.Episode{Title: "Feed title"})
	if err != nil {
		t.Fatal(err)
	}
	if needs {
		t.Fatal("NeedsNormalization() reported complete metadata as missing")
	}
}
