package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

func init() {
	f := func(rootPath string) (DocStore, error) {
		rv := jankyFS{
			RootDirectory: rootPath,
		}
		if rootPath == "" {
			rv.RootDirectory = "doc"
		}

		return rv, nil
	}
	RegisterStorageMethod("", f)
	RegisterStorageMethod("fs", f)
}

type jankyFS struct {
	RootDirectory string
}

// DocumentIDs lists all ID's for documents in this store
func (jfs jankyFS) DocumentIDs(context.Context) ([]string, error) {
	d, err := os.Open(jfs.RootDirectory)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	contents, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	rv := make([]string, 0, len(contents))
	for _, fi := range contents {
		n := fi.Name()
		if fi.IsDir() && len(n) == 11 && n[0] == 'g' {
			rv = append(rv, n[1:])
		}
	}
	return rv, nil
}

// NewDocumentID generates a new document ID that is not yet present in this store
func (jfs jankyFS) NewDocumentID(ctx context.Context) (string, error) {
	var rv string
	for ctx.Err() == nil {
		rv = NewDocumentID()
		_, err := os.Stat(path.Join(jfs.RootDirectory, "g"+rv))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				os.MkdirAll(path.Join(jfs.RootDirectory, "g"+rv), 0755)
				return rv, nil
			} else {
				return "", err
			}
		}
	}
	return "", ctx.Err()
}

func (jfs jankyFS) GetDocument(docID string) (DocTransaction, error) {
	if len(docID) != 10 {
		return nil, fmt.Errorf("invalid document ID")
	}
	if fi, err := os.Stat(path.Join(jfs.RootDirectory, "g"+docID)); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("invalid document ID")
	}

	return jankyTransaction{
		RootDirectory: jfs.RootDirectory,
		DocumentID:    docID,
	}, nil
}

type jankyTransaction struct {
	RootDirectory string
	DocumentID    string
}

func (t jankyTransaction) ReadRootFile(ctx context.Context, name string) (io.ReadCloser, error) {
	return os.Open(path.Join(t.RootDirectory, "g"+t.DocumentID, name))
}
func (t jankyTransaction) WriteRootFile(ctx context.Context, name string) (io.WriteCloser, error) {
	return os.Create(path.Join(t.RootDirectory, "g"+t.DocumentID, name))
}

func (t jankyTransaction) ListAttachments(ctx context.Context) ([]string, error) {
	d, err := os.Open(path.Join(t.RootDirectory, "g"+t.DocumentID, "att"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	contents, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	rv := make([]string, 0, len(contents))
	for _, fi := range contents {
		n := fi.Name()
		if fi.IsDir() && len(n) == 11 && n[0] == 't' {
			rv = append(rv, n[1:])
		}
	}
	return rv, nil
}
func (t jankyTransaction) ReadAttachment(ctx context.Context, name string) (io.ReadCloser, error) {
	return os.Open(path.Join(t.RootDirectory, "g"+t.DocumentID, "att", name))
}
func (t jankyTransaction) NewAttachmentID(ctx context.Context, ext string) (string, error) {
	os.MkdirAll(path.Join(t.RootDirectory, "g"+t.DocumentID, "att"), 0755)
	var rv string
	for ctx.Err() == nil {
		rv = NewDocumentID()
		fp := path.Join(t.RootDirectory, "g"+t.DocumentID, "att", "t"+rv+"."+ext)
		_, err := os.Stat(fp)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				g, err := os.Create(fp)
				if err == nil {
					g.Close()
					return rv, nil
				}
			} else {
				return "", err
			}
		}
	}
	return "", ctx.Err()
}
func (t jankyTransaction) WriteAttachment(ctx context.Context, name string) (io.WriteCloser, error) {
	os.MkdirAll(path.Join(t.RootDirectory, "g"+t.DocumentID, "att"), 0755)
	return os.Create(path.Join(t.RootDirectory, "g"+t.DocumentID, "att", name))
}

func (t jankyTransaction) Commit(ctx context.Context, name string) error {
	return nil
}
