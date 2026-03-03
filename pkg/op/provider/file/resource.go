package file

import (
	"errors"
	"fmt"
	"io"
	"os"
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
		return NewResource(s)
	})
}

// Resource represents a handle to data that can be streamed.
type Resource struct {
	op.Resource
	SourcePath string
	Inode      uint64
	Device     uint64
	Size       int64
	Mode       os.FileMode
	ModTime    time.Time
	Checksum   string
}

type Tombstone struct {
	RecoveryPath string // Where it is now
	OriginalPath string // Where it used to be
}

// NewResource initializes a new [Resource] by checking the existence and size of the file at the provided sourcePath.
//
// Parameters:
//   - sourcePath: path to the file to read
//
// Returns: an error if the path does not exist or is a directory
func NewResource(path string) (Resource, error) {

	// Generate a URI for the resource
	uri := fmt.Sprintf("file://%s", path)

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// This is a "Known Path but No Data" state.
			// The Executor will need to fulfill this via a Node later.
			return Resource{
				Resource:   op.Resource{URI: uri},
				SourcePath: path,
			}, nil
		}
		return Resource{}, fmt.Errorf("failed to stat blob source: %w", err)
	}

	// Extract syscall.Stat_t to get the Inode and Device
	var inode, device uint64
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev)
	}

	return Resource{
		Resource:   op.Resource{URI: uri},
		Inode:      inode,
		Device:     device,
		SourcePath: path,
		Size:       info.Size(),
		Mode:       info.Mode(),
		ModTime:    info.ModTime(),
		Checksum:   checksumFile(path),
	}, nil
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

	// We re-calculate the checksum because the content has changed
	r.Checksum = checksumFile(r.SourcePath)

	return nil
}

func (r *Resource) verifyMetadata(info os.FileInfo, err error) error {

	if err != nil {
		return err
	}

	return nil
}

// endregion
