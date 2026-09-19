package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type FileCopy struct {
	Source   string
	Relative string
}

type PlaylistFile struct {
	Relative string
	Content  []byte
}

type FilePlan struct {
	Copies    []FileCopy
	Playlists []PlaylistFile
	Keep      []string
	Deletes   []string
}

// BuildFilePlan compares managed device paths with desired paths. Deletes are
// limited to paths explicitly identified as podsync-managed.
func BuildFilePlan(managed []string, copies []FileCopy, playlists []PlaylistFile, keep []string) (FilePlan, error) {
	plan := FilePlan{
		Copies:    append([]FileCopy(nil), copies...),
		Playlists: append([]PlaylistFile(nil), playlists...),
		Keep:      append([]string(nil), keep...),
	}
	desired := make(map[string]struct{}, len(copies)+len(playlists))
	for _, copy := range copies {
		if err := validateRelative(copy.Relative); err != nil {
			return FilePlan{}, fmt.Errorf("copy path: %w", err)
		}
		if _, exists := desired[copy.Relative]; exists {
			return FilePlan{}, fmt.Errorf("duplicate desired path: %q", copy.Relative)
		}
		desired[copy.Relative] = struct{}{}
	}
	for _, playlist := range playlists {
		if err := validateRelative(playlist.Relative); err != nil {
			return FilePlan{}, fmt.Errorf("playlist path: %w", err)
		}
		if _, exists := desired[playlist.Relative]; exists {
			return FilePlan{}, fmt.Errorf("duplicate desired path: %q", playlist.Relative)
		}
		desired[playlist.Relative] = struct{}{}
	}
	for _, relative := range keep {
		if err := validateRelative(relative); err != nil {
			return FilePlan{}, fmt.Errorf("keep path: %w", err)
		}
		desired[relative] = struct{}{}
	}
	for _, path := range managed {
		if err := validateRelative(path); err != nil {
			return FilePlan{}, fmt.Errorf("managed path: %w", err)
		}
		if _, exists := desired[path]; !exists {
			plan.Deletes = append(plan.Deletes, path)
		}
	}
	return plan, nil
}

// ApplyFilePlan applies a plan beneath deviceRoot. It never deletes files not
// present in the plan's managed set.
func ApplyFilePlan(ctx context.Context, deviceRoot string, plan FilePlan) error {
	for _, copy := range plan.Copies {
		if err := copyFile(ctx, deviceRoot, copy); err != nil {
			return err
		}
	}
	for _, playlist := range plan.Playlists {
		if err := writeFile(ctx, deviceRoot, playlist.Relative, playlist.Content); err != nil {
			return err
		}
	}
	for _, relative := range plan.Deletes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := validateRelative(relative); err != nil {
			return fmt.Errorf("delete path: %w", err)
		}
		if err := os.Remove(filepath.Join(deviceRoot, filepath.FromSlash(relative))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %q: %w", relative, err)
		}
	}
	return nil
}

func copyFile(ctx context.Context, root string, copy FileCopy) error {
	if err := validateRelative(copy.Relative); err != nil {
		return fmt.Errorf("copy path: %w", err)
	}
	input, err := os.Open(copy.Source)
	if err != nil {
		return fmt.Errorf("open copy source %q: %w", copy.Source, err)
	}
	defer input.Close()

	return writeFromReader(ctx, root, copy.Relative, input)
}

func writeFile(ctx context.Context, root, relative string, content []byte) error {
	return writeFromReader(ctx, root, relative, bytes.NewReader(content))
}

func writeFromReader(ctx context.Context, root, relative string, reader io.Reader) error {
	if err := validateRelative(relative); err != nil {
		return fmt.Errorf("write path: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	destination := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".podsync-*")
	if err != nil {
		return fmt.Errorf("create temporary destination: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := io.Copy(temporary, contextReader{ctx: ctx, reader: reader}); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary destination: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary destination: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("install %q: %w", relative, err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}

func validateRelative(relative string) error {
	if relative == "" || filepath.IsAbs(relative) {
		return fmt.Errorf("path must be relative: %q", relative)
	}
	clean := filepath.ToSlash(filepath.Clean(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path escapes device root: %q", relative)
	}
	return nil
}
