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
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("file.Resource: expected string path, got %T", v)
		}
		return NewResource(s), nil
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

// Path returns the resource's source path. Before Resolve(), this is the
// raw path as given to the constructor. After Resolve(), it is the
// canonicalized absolute path on the target machine.
func (r *Resource) Path() string {
	return r.SourcePath
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

// NewResource creates a [Resource] with the given source path. The constructor
// is pure computation — no I/O, no error. Metadata (size, mode, checksum)
// is populated later by [Resource.Resolve].
func NewResource(path string) Resource {
	return Resource{SourcePath: path}
}

// Resolve populates the resource's metadata by canonicalizing the path and
// performing an os.Stat. If the file does not exist, Resolve returns nil
// and metadata remains empty ([Resource.Exists] returns false). Other
// stat errors are returned.
func (r *Resource) Resolve() error {
	abs, err := filepath.Abs(r.SourcePath)
	if err == nil {
		r.SourcePath = filepath.Clean(abs)
	}

	info, err := os.Stat(r.SourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat: %w", err)
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
	r.Checksum = checksumFile(r.SourcePath)
	return nil
}

// Exists returns true if the resource has been resolved and the file existed
// at resolve time. An unresolved resource always reports Exists() == false.
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

// Refresh re-populates the resource's metadata by performing a fresh os.Stat
// and re-calculating the checksum. Call after any successful physical mutation.
func (r *Resource) Refresh() error {
	info, err := os.Stat(r.SourcePath)
	if err != nil {
		return err
	}
	return r.refreshWith(info, checksumFile(r.SourcePath), info.Size())
}

// RefreshWith updates metadata after a write operation using a known checksum
// and size. An os.Stat is still performed to capture kernel-assigned identity
// (Inode, Device).
func (r *Resource) RefreshWith(checksum string, size int64) error {
	info, err := os.Stat(r.SourcePath)
	if err != nil {
		return err
	}
	return r.refreshWith(info, checksum, size)
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

// refreshWith updates the Resource's metadata with the provided information.
func (r *Resource) refreshWith(info os.FileInfo, checksum string, size int64) error {
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
	r.Checksum = checksum
	return nil
}

// endregion
