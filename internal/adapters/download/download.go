package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/HansMontana/podsync/internal/domain/catalog"
)

var ErrUnsupportedMedia = errors.New("unsupported media type")

type UnsupportedMediaError struct {
	ContentType string
	Detected    string
}

func (e *UnsupportedMediaError) Error() string {
	if e.Detected != "" {
		return fmt.Sprintf("%v: response is %s", ErrUnsupportedMedia, e.Detected)
	}
	return fmt.Sprintf("%v: response is %s", ErrUnsupportedMedia, e.ContentType)
}

func (e *UnsupportedMediaError) Unwrap() error { return ErrUnsupportedMedia }

// Episode downloads audio into host-side staging storage.
func Episode(ctx context.Context, client *http.Client, e catalog.Episode, stagingDir string) (path string, err error) {
	if e.Enclosure.URL == "" {
		return "", fmt.Errorf("episode has no audio URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", fmt.Errorf("create staging directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.Enclosure.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create audio request: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download episode: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download episode: unexpected HTTP status %s", response.Status)
	}
	contentType := response.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	declaredAudio := strings.HasPrefix(strings.ToLower(strings.TrimSpace(e.Enclosure.Type)), "audio/")
	declaredType := strings.ToLower(strings.TrimSpace(e.Enclosure.Type))
	if strings.HasPrefix(declaredType, "video/") || declaredType == "application/mp4" {
		return "", &UnsupportedMediaError{ContentType: declaredType}
	}
	if isKnownNonAudioType(mediaType) || strings.HasPrefix(strings.ToLower(mediaType), "video/") || mediaType == "application/mp4" ||
		(mediaType != "" && !strings.HasPrefix(strings.ToLower(mediaType), "audio/") && !genericBinaryType(mediaType) && !(declaredAudio && mediaType == "text/plain")) {
		return "", &UnsupportedMediaError{ContentType: mediaType}
	}
	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(response.Body, prefix)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read media header: %w", readErr)
	}
	detected := http.DetectContentType(prefix[:n])
	looksLikeMP4 := n >= 8 && bytes.Equal(prefix[4:8], []byte("ftyp"))
	if isKnownNonAudioType(detected) || strings.HasPrefix(detected, "video/") || (looksLikeMP4 && declaredType != "audio/mp4") {
		return "", &UnsupportedMediaError{ContentType: mediaType, Detected: detected}
	}

	file, err := os.CreateTemp(stagingDir, "podsync-download-*")
	if err != nil {
		return "", fmt.Errorf("create staging file: %w", err)
	}
	path = file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	bytesWritten, err := io.Copy(file, io.MultiReader(bytes.NewReader(prefix[:n]), response.Body))
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write staged episode: %w", err)
	}
	if bytesWritten == 0 {
		_ = file.Close()
		return "", fmt.Errorf("download is empty")
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close staged episode: %w", err)
	}
	return path, nil
}

func isKnownNonAudioType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "text/html", "text/xml", "application/xml", "application/xhtml+xml", "application/json", "application/javascript":
		return true
	default:
		return false
	}
}

func genericBinaryType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "application/octet-stream", "binary/octet-stream", "application/ogg":
		return true
	default:
		return false
	}
}
