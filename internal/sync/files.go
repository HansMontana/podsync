package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type FileCopy struct {
	Source   string
	Relative string
}

type PlaylistFile struct {
	Relative string
	Content  []byte
}

type FileProgress struct {
	Phase     string
	Completed int
	Total     int
	Relative  string
}

type FileProgressFunc func(FileProgress)

type FilePlan struct {
	Copies    []FileCopy
	Playlists []PlaylistFile
	Keep      []string
	Deletes   []string
}

// BuildFilePlan limits every device operation to podsync-owned paths.
func BuildFilePlan(managed []string, copies []FileCopy, playlists []PlaylistFile, keep []string) (FilePlan, error) {
	plan := FilePlan{Copies: append([]FileCopy(nil), copies...), Playlists: append([]PlaylistFile(nil), playlists...), Keep: append([]string(nil), keep...)}
	desired := make(map[string]struct{}, len(copies)+len(playlists)+len(keep))
	for _, relative := range append(copyPaths(copies), append(playlistPaths(playlists), keep...)...) {
		if err := validateManagedPath(relative); err != nil {
			return FilePlan{}, err
		}
		if _, exists := desired[relative]; exists {
			return FilePlan{}, fmt.Errorf("duplicate desired path: %q", relative)
		}
		desired[relative] = struct{}{}
	}
	for _, relative := range managed {
		if err := validateManagedPath(relative); err != nil {
			return FilePlan{}, fmt.Errorf("managed path: %w", err)
		}
		if _, exists := desired[relative]; !exists {
			plan.Deletes = append(plan.Deletes, relative)
		}
	}
	return plan, nil
}

func copyPaths(copies []FileCopy) []string {
	paths := make([]string, len(copies))
	for i, copy := range copies {
		paths[i] = copy.Relative
	}
	return paths
}

func playlistPaths(playlists []PlaylistFile) []string {
	paths := make([]string, len(playlists))
	for i, playlist := range playlists {
		paths[i] = playlist.Relative
	}
	return paths
}

// ApplyFilePlan installs content, removes obsolete owned files, then commits
// the manifest last so failed deletion remains recoverable on a later sync.
func ApplyFilePlan(ctx context.Context, deviceRoot string, plan FilePlan) error {
	return ApplyFilePlanWithProgress(ctx, deviceRoot, plan, nil)
}

func ApplyFilePlanWithProgress(ctx context.Context, deviceRoot string, plan FilePlan, progress FileProgressFunc) error {
	copyTotal := len(plan.Copies)
	for i, copy := range plan.Copies {
		if err := copyFile(ctx, deviceRoot, copy); err != nil {
			return err
		}
		if progress != nil {
			progress(FileProgress{Phase: "copy", Completed: i + 1, Total: copyTotal, Relative: copy.Relative})
		}
	}
	var manifest *PlaylistFile
	playlistTotal := 0
	for _, playlist := range plan.Playlists {
		if playlist.Relative != "Podsync/managed-files.txt" {
			playlistTotal++
		}
	}
	playlistCompleted := 0
	for i := range plan.Playlists {
		playlist := plan.Playlists[i]
		if playlist.Relative == "Podsync/managed-files.txt" {
			manifest = &playlist
			continue
		}
		if err := writeFile(ctx, deviceRoot, playlist.Relative, playlist.Content); err != nil {
			return err
		}
		playlistCompleted++
		if progress != nil {
			progress(FileProgress{Phase: "playlist", Completed: playlistCompleted, Total: playlistTotal, Relative: playlist.Relative})
		}
	}
	for i, relative := range plan.Deletes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		destination, err := safeDevicePath(deviceRoot, relative, false)
		if err != nil {
			return fmt.Errorf("delete %q: %w", relative, err)
		}
		if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %q: %w", relative, err)
		}
		if progress != nil {
			progress(FileProgress{Phase: "delete", Completed: i + 1, Total: len(plan.Deletes), Relative: relative})
		}
	}
	if manifest != nil {
		if err := writeFile(ctx, deviceRoot, manifest.Relative, manifest.Content); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(ctx context.Context, root string, copy FileCopy) error {
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
	destination, err := safeDevicePath(root, relative, true)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
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
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary destination: %w", err)
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

var audioPathPattern = regexp.MustCompile(`^Podcasts/feed-[1-9][0-9]*/[a-f0-9]{16}\.[A-Za-z0-9]{1,8}$`)
var playlistPathPattern = regexp.MustCompile(`^Playlists/.+\.m3u8$`)

func validateManagedPath(relative string) error {
	if relative == "" || filepath.IsAbs(relative) || strings.ContainsAny(relative, "\r\n\x00") {
		return fmt.Errorf("path must be a safe relative path: %q", relative)
	}
	clean := filepath.ToSlash(filepath.Clean(relative))
	if clean != relative || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path escapes device root: %q", relative)
	}
	if clean == "Podsync/managed-files.txt" || audioPathPattern.MatchString(clean) || validAudioPath(clean) || validPlaylistPath(clean) {
		return nil
	}
	return fmt.Errorf("path is outside podsync-managed locations: %q", relative)
}

func validAudioPath(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 3 || parts[0] != "Podcasts" || parts[1] == "" {
		return false
	}
	for _, character := range parts[1] {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || strings.ContainsRune("._-", character) {
			continue
		}
		return false
	}
	name := parts[2]
	extension := filepath.Ext(name)
	if extension == "" || len(name) == len(extension) {
		return false
	}
	for _, character := range strings.TrimSuffix(name, extension) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || strings.ContainsRune(" .,_-'()&!", character) {
			continue
		}
		return false
	}
	return true
}

func validPlaylistPath(path string) bool {
	if !playlistPathPattern.MatchString(path) {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(path, "Playlists/"), ".m3u8")
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, character := range name {
		if unicode.IsLetter(character) || unicode.IsNumber(character) || character == ' ' || strings.ContainsRune("._-'()&", character) {
			continue
		}
		return false
	}
	return true
}

// safeDevicePath rejects symlinks in every existing path component. This is a
// portable best-effort guard for device filesystems that normally lack links.
func safeDevicePath(root, relative string, createParents bool) (string, error) {
	if err := validateManagedPath(relative); err != nil {
		return "", err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", fmt.Errorf("inspect device root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("device root is not a real directory")
	}
	parts := strings.Split(relative, "/")
	directory := root
	for _, part := range parts[:len(parts)-1] {
		directory = filepath.Join(directory, part)
		info, err := os.Lstat(directory)
		if os.IsNotExist(err) {
			if !createParents {
				return filepath.Join(directory, parts[len(parts)-1]), nil
			}
			if err := os.Mkdir(directory, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path component %q is not a real directory", part)
		}
	}
	return filepath.Join(directory, parts[len(parts)-1]), nil
}
