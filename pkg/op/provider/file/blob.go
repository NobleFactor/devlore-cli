package file

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// Blob represents a handle to data that can be streamed.
type Blob struct {
	SourcePath string
	Size       int64
}

// NewBlob initializes a new [Blob] by checking the existence and  size of the file at the provided sourcePath.
//
// Parameters:
//   - sourcePath: path to the file to read
//
// Returns: an error if the path does not exist or is a directory
func NewBlob(sourcePath string) (Blob, error) {

	info, err := os.Stat(sourcePath)

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Blob{}, fmt.Errorf("blob source missing: %w", err)
		}
		return Blob{}, fmt.Errorf("failed to stat blob source: %w", err)
	}

	if info.IsDir() {
		return Blob{}, fmt.Errorf("source path is a directory: %s", sourcePath)
	}

	return Blob{
		SourcePath: sourcePath,
		Size:       info.Size(),
	}, nil
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
