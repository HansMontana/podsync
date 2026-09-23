package devicefs

import (
	"context"

	"github.com/HansMontana/podsync/internal/application/deviceport"
)

// Operations adapts device filesystem behavior to the sync application port.
type Operations struct{}

var _ deviceport.DeviceFiles = Operations{}

func New() Operations {
	return Operations{}
}

func (Operations) LoadManagedPaths(root string) ([]string, error) {
	return (Layout{Root: root}).LoadManagedPaths()
}

func (Operations) LoadPendingManagedPaths(root string) ([]string, error) {
	return (Layout{Root: root}).LoadPendingManagedPaths()
}

func (Operations) SavePendingManagedPaths(root string, paths []string) error {
	return (Layout{Root: root}).SavePendingManagedPaths(paths)
}

func (Operations) ClearPendingManagedPaths(root string) error {
	return (Layout{Root: root}).ClearPendingManagedPaths()
}

func (Operations) BuildFilePlan(managed []string, copies []deviceport.FileCopy, playlists []deviceport.PlaylistFile, keep []string) (deviceport.FilePlan, error) {
	technicalCopies := make([]FileCopy, len(copies))
	for i, copy := range copies {
		technicalCopies[i] = FileCopy{Source: copy.Source, Relative: copy.Relative}
	}
	technicalPlaylists := make([]PlaylistFile, len(playlists))
	for i, playlist := range playlists {
		technicalPlaylists[i] = PlaylistFile{Relative: playlist.Relative, Content: playlist.Content}
	}
	plan, err := BuildFilePlan(managed, technicalCopies, technicalPlaylists, keep)
	if err != nil {
		return deviceport.FilePlan{}, err
	}
	return deviceport.FilePlan{
		Copies:       convertCopies(plan.Copies),
		Playlists:    convertPlaylists(plan.Playlists),
		Keep:         plan.Keep,
		Deletes:      plan.Deletes,
		VerifyDevice: plan.VerifyDevice,
	}, nil
}

func (Operations) ApplyFilePlan(ctx context.Context, root string, plan deviceport.FilePlan, progress deviceport.FileProgressFunc) error {
	technicalPlan := FilePlan{
		Copies:       convertTechnicalCopies(plan.Copies),
		Playlists:    convertTechnicalPlaylists(plan.Playlists),
		Keep:         plan.Keep,
		Deletes:      plan.Deletes,
		VerifyDevice: plan.VerifyDevice,
	}
	return ApplyFilePlanWithProgress(ctx, root, technicalPlan, func(current FileProgress) {
		if progress != nil {
			progress(deviceport.FileProgress{Phase: current.Phase, Completed: current.Completed, Total: current.Total, Relative: current.Relative})
		}
	})
}

func (Operations) SafeDevicePath(root, relative string, allowMissing bool) (string, error) {
	return SafeDevicePath(root, relative, allowMissing)
}

func convertCopies(copies []FileCopy) []deviceport.FileCopy {
	result := make([]deviceport.FileCopy, len(copies))
	for i, copy := range copies {
		result[i] = deviceport.FileCopy{Source: copy.Source, Relative: copy.Relative}
	}
	return result
}

func convertTechnicalCopies(copies []deviceport.FileCopy) []FileCopy {
	result := make([]FileCopy, len(copies))
	for i, copy := range copies {
		result[i] = FileCopy{Source: copy.Source, Relative: copy.Relative}
	}
	return result
}

func convertPlaylists(playlists []PlaylistFile) []deviceport.PlaylistFile {
	result := make([]deviceport.PlaylistFile, len(playlists))
	for i, playlist := range playlists {
		result[i] = deviceport.PlaylistFile{Relative: playlist.Relative, Content: playlist.Content}
	}
	return result
}

func convertTechnicalPlaylists(playlists []deviceport.PlaylistFile) []PlaylistFile {
	result := make([]PlaylistFile, len(playlists))
	for i, playlist := range playlists {
		result[i] = PlaylistFile{Relative: playlist.Relative, Content: playlist.Content}
	}
	return result
}
