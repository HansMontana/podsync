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

	"github.com/HansMontana/podsync/internal/episode"
)

const maxDownloadSize int64 = 1 << 30

var ErrUnsupportedMedia = errors.New("unsupported media type")
var ErrOversizedMedia = errors.New("oversized media")

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
func Episode(ctx context.Context, client *http.Client, e episode.Episode, stagingDir string) (path string, err error) {
	if e.Enclosure.URL == "" {
		return "", fmt.Errorf("episode has no audio URL")
	}
	if e.Enclosure.Length > maxDownloadSize {
		return "", fmt.Errorf("%w: episode enclosure exceeds %d bytes", ErrOversizedMedia, maxDownloadSize)
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
	if response.ContentLength > maxDownloadSize {
		return "", fmt.Errorf("%w: download response exceeds %d bytes", ErrOversizedMedia, maxDownloadSize)
	}
	contentType := response.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	declaredAudio := strings.HasPrefix(strings.ToLower(strings.TrimSpace(e.Enclosure.Type)), "audio/")
	if mediaType != "" && !strings.HasPrefix(strings.ToLower(mediaType), "audio/") &&
		(!declaredAudio || strings.HasPrefix(strings.ToLower(mediaType), "video/") || mediaType == "application/mp4") {
		return "", &UnsupportedMediaError{ContentType: mediaType}
	}
	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(response.Body, prefix)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read media header: %w", readErr)
	}
	detected := http.DetectContentType(prefix[:n])
	declaredType := strings.ToLower(strings.TrimSpace(e.Enclosure.Type))
	looksLikeMP4 := n >= 8 && bytes.Equal(prefix[4:8], []byte("ftyp"))
	if strings.HasPrefix(detected, "video/") || (looksLikeMP4 && declaredType != "audio/mp4") {
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

	bytesWritten, err := io.Copy(file, io.LimitReader(io.MultiReader(bytes.NewReader(prefix[:n]), response.Body), maxDownloadSize+1))
	if err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write staged episode: %w", err)
	}
	if bytesWritten > maxDownloadSize {
		_ = file.Close()
		return "", fmt.Errorf("%w: download exceeds %d bytes", ErrOversizedMedia, maxDownloadSize)
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
