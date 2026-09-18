package device

import "path/filepath"

type Layout struct {
	Root string
}

func (l Layout) StateDirectory() string {
	return filepath.Join(l.Root, "Podsync")
}

func (l Layout) DatabasePath() string {
	return filepath.Join(l.StateDirectory(), "podsync.db")
}

func (l Layout) ConfigPath() string {
	return filepath.Join(l.StateDirectory(), "podsync.toml")
}

func (l Layout) AudioDirectory() string {
	return filepath.Join(l.Root, "Podcasts")
}

func (l Layout) PlaylistDirectory() string {
	return filepath.Join(l.Root, "Playlists")
}
