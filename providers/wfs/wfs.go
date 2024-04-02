package wfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	hfs "github.com/hack-pad/hackpadfs"
	"github.com/infogulch/xtemplate"
)

// DotWFS is used to create an xtemplate dot field value that can access files in
// a local directory, or any [fs.FS].
type DotWFS struct {
	fs     fs.FS
	log    *slog.Logger
	w      http.ResponseWriter
	r      *http.Request
	opened map[fs.File]struct{}
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// List reads and returns a slice of names from the given directory
// relative to the FS root.
func (c *DotWFS) List(name string) ([]string, error) {
	entries, err := fs.ReadDir(c.fs, path.Clean(name))
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, dirEntry := range entries {
		names = append(names, dirEntry.Name())
	}

	return names, nil
}

// Exists returns true if filename can be opened successfully.
func (c *DotWFS) Exists(filename string) (bool, error) {
	file, err := c.fs.Open(filename)
	if err == nil {
		file.Close()
		return true, nil
	}
	return false, nil
}

// Stat returns Stat of a filename.
//
// Note: if you intend to read the file, afterwards, calling .Open instead may
// be more efficient.
func (c *DotWFS) Stat(filename string) (fs.FileInfo, error) {
	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// Read returns the contents of a filename relative to the FS root as a
// string.
func (c *DotWFS) Read(filename string) (string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	filename = path.Clean(filename)
	file, err := c.fs.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(buf, file)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Open opens the file
func (c *DotWFS) Open(path_ string) (fs.File, error) {
	path_ = path.Clean(path_)

	file, err := c.fs.Open(path_)
	if err != nil {
		return nil, fmt.Errorf("failed to open file at path '%s': %w", path_, err)
	}

	c.log.Debug("opened file", slog.String("path", path_))
	c.opened[file] = struct{}{}

	return file, nil
}

func (c *DotWFS) MkdirAll(path string) error {
	return hfs.MkdirAll(c.fs, path, os.ModePerm|os.ModeDir)
}

func (c *DotWFS) Create(name string) (fs.File, error) {
	return hfs.Create(c.fs, name)
}

// Write writes data to the file
func (c *DotWFS) Write(file fs.File, data any) error {
	var reader io.Reader
	switch r := data.(type) {
	case io.Reader:
		reader = r
	case []byte:
		reader = bytes.NewReader(r)
	case string:
		reader = strings.NewReader(r)
	default:
		return fmt.Errorf("cannot write type %T to file", data)
	}
	w, ok := file.(io.Writer)
	if !ok {
		return fmt.Errorf("file is not writable")
	}
	_, err := io.Copy(w, reader)
	return err
}

type MultipartMetadata struct {
	Dir       string
	TotalSize int64
	Files     []ReceivePart
}

type ReceivePart struct {
	FormName     string
	FormFileName string
	Header       map[string][]string
	FileName     string
	FileSize     int64
}

// ReceiveFiles streams uploaded files to a new dir under dir
//
// https://github.com/rfielding/uploader/blob/6048015d9ad169f38f2559e38697b1ce02ea947a/uploader.go#L167
//
// TODO(infogulch): Check header and peek at first bytes of uploaded file to
// infer Content-Type, and allow caller to limit which kinds of types are
// allowed.
func (c *DotWFS) ReceiveFiles(dir string, maxFiles int, maxBytes int64) (_ *MultipartMetadata, err error) {
	start := time.Now()
	rid := xtemplate.GetRequestId(c.r.Context())
	if rid == "" {
		rid = uuid.NewString()
	}

	c.r.Body = http.MaxBytesReader(c.w, c.r.Body, maxBytes)

	var partReader *multipart.Reader
	partReader, err = c.r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("failed to get a multipart reader: %w", err)
	}

	dir = path.Join(dir, rid)
	err = hfs.MkdirAll(c.fs, dir, os.ModeDir|os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory to receive uploaded files: %w", err)
	}

	result := MultipartMetadata{}

	defer func() {
		c.log.Debug("finished receiving file uploads", slog.Any("result", result), slog.Duration("receive_time", time.Since(start)), slog.Any("error", err))
	}()

	for i := range maxFiles + 1 {
		part, err := partReader.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to get next part: %w", err)
		} else if i == maxFiles {
			return nil, errors.Join(fmt.Errorf("too many files"), xtemplate.ErrorStatus(http.StatusBadRequest))
		}
		ext := path.Ext(part.FileName())
		if ext == "" {
			ext = ".upload"
		}
		f := ReceivePart{
			FormName:     part.FormName(),
			FormFileName: part.FileName(),
			Header:       part.Header,
			FileName:     path.Join(dir, fmt.Sprintf("%0*d%s", int(math.Ceil(math.Log10(float64(maxFiles)))), i, ext)),
		}
		if err = func() error {
			file, err := hfs.Create(c.fs, f.FileName)
			if err != nil {
				return fmt.Errorf("failed to create file to receive upload: %w", err)
			}
			defer file.Close()
			writer, ok := file.(io.Writer)
			if !ok {
				return fmt.Errorf("failed to convert file to writable file: %w", err)
			}
			n, err := io.Copy(writer, part)
			mberr := &http.MaxBytesError{}
			if errors.As(err, &mberr) {
				return errors.Join(fmt.Errorf("upload too large: %w", mberr), xtemplate.ErrorStatus(http.StatusBadRequest))
			} else if err != nil {
				return fmt.Errorf("failed to write upload to fs (%d bytes written): %w", n, err)
			}
			f.FileSize = n
			result.TotalSize += n
			result.Files = append(result.Files, f)
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return &result, nil
}
