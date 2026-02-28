package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	op.RegisterConstructor(func(v any) (Blob, error) {
		s, ok := v.(string)
		if !ok {
			return Blob{}, fmt.Errorf("Blob: expected string path, got %T", v)
		}
		return NewBlob(s)
	})
}

// Blob represents a handle to data that can be streamed.
type Blob struct {
	SourcePath string
	Size       int64
	Mode       os.FileMode
	ModTime    time.Time
	Checksum   string
}

// Exists returns true if the Blob references a file that existed when the Blob was created.
//
// This method checks if the ModTime is non-zero, indicating that the file existed at the time of Blob creation.
//
// Parameters:
//   - none
//
// Returns:
//   - bool: true if the file exists, false otherwise
func (b Blob) Exists() bool {
	return !b.ModTime.IsZero()
}

// NewBlob initializes a new [Blob] by checking the existence and size of the file at the provided sourcePath.
//
// Parameters:
//   - sourcePath: path to the file to read
//
// Returns: an error if the path does not exist or is a directory
func NewBlob(path string) (Blob, error) {

	info, err := os.Stat(path)

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Blob{SourcePath: path}, nil
		}
		return Blob{}, fmt.Errorf("failed to stat blob source: %w", err)
	}

	if info.IsDir() {
		return Blob{}, fmt.Errorf("source path is a directory: %s", path)
	}

	return Blob{
		SourcePath: path,
		Size:       info.Size(),
		Mode:       info.Mode().Perm(),
		ModTime:    info.ModTime(),
		Checksum:   checksumFile(path),
	}, nil
}

// Reader returns an io.ReadCloser for reading the blob's data from its source path.
//
// The caller is responsible for closing the returned reader.
//
// Parameters:
//   - none
//
// Returns:
//   - io.ReadCloser: an io.ReadCloser for reading the blob's data
//   - error: any error that occurred during opening
func (b Blob) Reader() (io.ReadCloser, error) {
	return os.Open(b.SourcePath)
}

// WriteTo allows the Blob to be streamed directly to any io.Writer.
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
func (b Blob) WriteTo(writer io.Writer) (int64, error) {

	f, err := os.Open(b.SourcePath)

	if err != nil {
		return 0, err
	}

	defer f.Close()
	return io.Copy(writer, f)
}
