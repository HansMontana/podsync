package deviceport

import "context"

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
	Copies       []FileCopy
	Playlists    []PlaylistFile
	Keep         []string
	Deletes      []string
	VerifyDevice func() error
}

// DeviceFiles owns the device filesystem operations required by sync.
type DeviceFiles interface {
	LoadManagedPaths(root string) ([]string, error)
	LoadPendingManagedPaths(root string) ([]string, error)
	SavePendingManagedPaths(root string, paths []string) error
	ClearPendingManagedPaths(root string) error
	LoadMediaSignatures(root string) (map[string]string, error)
	SaveMediaSignatures(root string, signatures map[string]string) error
	BuildFilePlan(managed []string, copies []FileCopy, playlists []PlaylistFile, keep []string) (FilePlan, error)
	ApplyFilePlan(context.Context, string, FilePlan, FileProgressFunc) error
	SafeDevicePath(root, relative string, allowMissing bool) (string, error)
}
