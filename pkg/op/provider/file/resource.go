package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	// Execution-time constructor: creates a Resource with full metadata (os.Stat).
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("file.Resource: expected string path, got %T", v)
		}
		return NewResource(s)
	})

	// Plan-time constructor: creates a URI-only Resource with no I/O.
	// Used by the planned bridge for catalog resolution.
	op.RegisterPlanTimeConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("file.Resource: expected string path, got %T", v)
		}
		return Resource{SourcePath: s}, nil
	})
}

// Resource represents a handle to data that can be streamed.
type Resource struct {
	op.ResourceBase
	SourcePath string
	Inode      uint64
	Device     uint64
	Size       int64
	Mode       os.FileMode
	ModTime    time.Time
	Checksum   string
}

// URI returns the canonical file:// URI for this resource.
func (r *Resource) URI() string { return r.NewURI(r) }

// Scheme returns "file".
func (r *Resource) Scheme() string { return op.SchemeFile }

// Host returns empty string — file URIs have no authority.
func (r *Resource) Host() string { return "" }

// Path returns the canonicalized absolute path.
func (r *Resource) Path() string {
	abs, err := filepath.Abs(r.SourcePath)
	if err != nil {
		return filepath.Clean(r.SourcePath)
	}
	return filepath.Clean(abs)
}

// Tombstone holds file-specific compensation state.
//
// The embedded [op.TombstoneBase] carries the affected [Resource] whose
// SourcePath reflects where the data physically IS after the operation
// (e.g., the recovery path after moveToRecovery). OriginalPath records
// where the data WAS before the operation — the restoration target.
type Tombstone struct {
	op.TombstoneBase
	OriginalPath string
}

// NewResource initializes a new [Resource] by checking the existence and size of the file at the provided sourcePath.
//
// Parameters:
//   - sourcePath: path to the file to read
//
// Returns: an error if the path does not exist or is a directory
func NewResource(path string) (Resource, error) {
	r := Resource{SourcePath: path}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Known path but no data — the executor will resolve this later.
			return r, nil
		}
		return Resource{}, fmt.Errorf("failed to stat blob source: %w", err)
	}

	var inode, device uint64
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev)
	}

	r.Inode = inode
	r.Device = device
	r.Size = info.Size()
	r.Mode = info.Mode()
	r.ModTime = info.ModTime()
	r.Checksum = checksumFile(path)
	return r, nil
}

// Exists returns true if the Resource references a file that existed when the Resource was created.
//
// This method checks if the ModTime is non-zero, indicating that the file existed at the time of Resource creation.
//
// Parameters:
//   - none
//
// Returns:
//   - bool: true if the file exists, false otherwise
func (r *Resource) Exists() bool {
	return !r.ModTime.IsZero()
}

// Reader returns an io.ReadCloser for reading the file resource's data from its source path.
//
// The caller is responsible for closing the returned reader.
//
// Parameters:
//   - none
//
// Returns:
//   - io.ReadCloser: an io.ReadCloser for reading the file resource's data
//   - error: any error that occurred during opening
func (r *Resource) Reader() (io.ReadCloser, error) {
	return os.Open(r.SourcePath)
}

// RefreshMetadata updates the Resource's metadata by performing a fresh os.Stat and re-calculating the checksum.
//
// This should be called after any successful physical mutation.
//
// Parameters:
//   - none
//
// Returns:
//   - error: any error that occurred during refreshing
func (r *Resource) RefreshMetadata() error {

	info, err := os.Stat(r.SourcePath)

	if err = r.verifyMetadata(info, err); err != nil {
		return err
	}

	return r.refreshMetadataWith(info, checksumFile(r.SourcePath), info.Size())
}

// RefreshMetadataWith updates the resource metadata after a write operation.
//
// It takes the known size and checksum to avoid redundant I/O but performs an os.Stat to capture Kernel-assigned
// identity (Device and Inode)
//
// Parameters:
//   - checksum: the checksum of the blob after the write operation
//   - size: the size of the blob after the write operation
//
// Returns:
//   - error: any error that occurred during finalization
func (r *Resource) RefreshMetadataWith(checksum string, size int64) error {

	info, err := os.Stat(r.SourcePath)

	if err = r.verifyMetadata(info, err); err != nil {
		return err
	}

	return r.refreshMetadataWith(info, checksum, size)
}

// WriteTo allows the Resource to be streamed directly to any io.Writer.
//
// For efficiency, it uses [io.Copy] which automatically attempts a zero-copy syscall before falling back to a 32KB
// buffer.
//
// Parameters:
//   - writer: io.Writer to write to
//
// Returns:
//   - int64: number of bytes written
//   - error: any error that occurred during writing
func (r *Resource) WriteTo(writer io.Writer) (int64, error) {

	f, err := os.Open(r.SourcePath)

	if err != nil {
		return 0, err
	}
	defer f.Close()

	byteCount, err := io.Copy(writer, f)

	if err = f.Sync(); err != nil {
		return 0, err
	}

	return byteCount, err
}

// region Internals

// refreshMetadataWith updates the Resource's metadata with the provided information.
//
// Parameters:
//   - info: os.FileInfo for the blob
//   - checksum: the checksum of the blob after the write operation
//   - size: the size of the blob after the write operation
//
// Returns:
//   - error: any error that occurred during refreshing
func (r *Resource) refreshMetadataWith(info os.FileInfo, checksum string, size int64) error {

	// Capture physical identity (Inode/Device)

	var inode, device uint64

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev)
	}

	// Update fields

	r.Inode = inode
	r.Device = device
	r.Size = info.Size()
	r.Mode = info.Mode()
	r.ModTime = info.ModTime()

	r.Checksum = checksum

	return nil
}

func (r *Resource) verifyMetadata(info os.FileInfo, err error) error {

	if err != nil {
		return err
	}

	return nil
}

// endregion
