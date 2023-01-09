package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

type DocStore interface {
	// DocumentIDs lists all IDs for documents in this store
	DocumentIDs(context.Context) ([]string, error)

	// NewDocumentID generates a new document ID that is not yet present in this store
	NewDocumentID(context.Context) (string, error)

	// GetDocument starts a transaction for a document ID
	GetDocument(string) (DocTransaction, error)
}

type DocTransaction interface {
	DocumentID() string

	ReadRootFile(context.Context, string) (io.ReadCloser, error)
	WriteRootFile(context.Context, string) (io.WriteCloser, error)

	ListAttachments(context.Context) ([]string, error)
	ReadAttachment(context.Context, string) (io.ReadCloser, error)
	NewAttachmentID(context.Context, string) (string, error)
	WriteAttachment(context.Context, string) (io.WriteCloser, error)
	DeleteAttachment(context.Context, string) error

	Commit(context.Context, string) error
	Rollback() error
}

func NewDocumentID() string {
	var b []byte = make([]byte, 5)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type StorageMethod func(string) (DocStore, error)

var allStorageMethods map[string]StorageMethod

func RegisterStorageMethod(name string, f StorageMethod) {
	if allStorageMethods == nil {
		allStorageMethods = make(map[string]StorageMethod)
	}
	allStorageMethods[name] = f
}

func GetDocStore(descriptor string) (DocStore, error) {
	i := strings.IndexRune(descriptor, ':')
	name := ""
	if i >= 0 {
		name = descriptor[:i]
		descriptor = descriptor[i+1:]
	}
	if allStorageMethods == nil {
		return nil, fmt.Errorf("no storage backends have been initialized")
	}

	if f, ok := allStorageMethods[name]; ok {
		return f(descriptor)
	}
	return nil, fmt.Errorf("storage backend '%s' not registered", name)
}

func Copy(ctx context.Context, target, source DocStore) error {
	ids, err := source.DocumentIDs(ctx)
	if err != nil {
		return err
	}

	rootFiles := []string{"document.bin", "meta.xml"}

	for _, id := range ids {
		err = func(id string) error {
			trnsSrc, err := source.GetDocument(id)
			if err != nil {
				return err
			}
			defer trnsSrc.Rollback()

			trnsTgt, err := target.GetDocument(id)
			if err != nil {
				return err
			}
			commit := false
			defer func() {
				if commit {
					trnsTgt.Commit(ctx, "import from external store")
				} else {
					trnsTgt.Rollback()
				}
			}()

			// TODO: range over all root files, not just the ones I remembered to mention in the list above
			for _, rf := range rootFiles {
				g, err := trnsTgt.WriteRootFile(ctx, rf)
				if err != nil {
					return err
				}
				f, err := trnsSrc.ReadRootFile(ctx, rf)
				if err != nil {
					g.Close()
					return err
				}
				_, err = io.Copy(g, f)
				f.Close()
				g.Close()
				if err != nil {
					return err
				}
			}

			atts, err := trnsSrc.ListAttachments(ctx)
			if err != nil {
				return err
			}

			for _, att := range atts {
				g, err := trnsTgt.WriteAttachment(ctx, att)
				if err != nil {
					return err
				}
				f, err := trnsSrc.ReadAttachment(ctx, att)
				if err != nil {
					g.Close()
					return err
				}
				_, err = io.Copy(g, f)
				f.Close()
				g.Close()
				if err != nil {
					return err
				}
			}

			commit = true
			return nil
		}(id)
		if err != nil {
			return err
		}
	}
	return nil
}

type ExtensionKnower interface {
	AttachmentNameFromID(context.Context, string) (string, error)
}

var knownExtensions []string = []string{"css", "svg", "png", "jpeg", "ico", "woff", "woff2", "eot", "ttf"}

func AttachmentNameFromID(ctx context.Context, trns DocTransaction, att_id string) (string, error) {
	if ek, ok := trns.(ExtensionKnower); ok {
		return ek.AttachmentNameFromID(ctx, att_id)
	}

	var f io.ReadCloser
	var err error
	for _, e := range knownExtensions {
		attName := "t" + att_id + "." + e
		f, err = trns.ReadAttachment(ctx, attName)
		if err == nil {
			f.Close()
			return attName, nil
		}
	}
	return "", err
}

type DirectFileAccesser interface {
	GetRootFile(context.Context, string, string) (io.ReadCloser, error)
	GetAttachment(context.Context, string, string) (io.ReadCloser, error)
}

func GetRootFile(ctx context.Context, st DocStore, doc_id, name string) (io.ReadCloser, error) {
	if dfa, ok := st.(DirectFileAccesser); ok {
		return dfa.GetRootFile(ctx, doc_id, name)
	}

	trns, err := st.GetDocument(doc_id)
	if err != nil {
		return nil, err
	}
	defer trns.Rollback()

	f, err := trns.ReadRootFile(ctx, name)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func GetAttachment(ctx context.Context, st DocStore, doc_id, att_id string) (io.ReadCloser, error) {
	if dfa, ok := st.(DirectFileAccesser); ok {
		return dfa.GetAttachment(ctx, doc_id, att_id)
	}

	trns, err := st.GetDocument(doc_id)
	if err != nil {
		return nil, err
	}
	defer trns.Rollback()

	var f io.ReadCloser
	for _, e := range knownExtensions {
		attName := "t" + att_id + "." + e
		f, err = trns.ReadAttachment(ctx, attName)
		if err == nil {
			return f, nil
		}
	}
	return nil, err
}

type Limit struct {
	Offset int
	Limit  int
}

type DocumentCache interface {
	GetDocuments(context.Context, string, Limit) ([]string, []DocumentMeta, error)
	GetDocumentByURL(context.Context, string, string) (DocTransaction, bool, error)
	GetDocumentMeta(context.Context, string) (DocumentMeta, error)
}

type CacheMethod func(string, DocStore) (DocumentCache, error)

var allCacheMethods map[string]CacheMethod

func RegisterCacheMethod(name string, f CacheMethod) {
	if allCacheMethods == nil {
		allCacheMethods = make(map[string]CacheMethod)
	}
	allCacheMethods[name] = f
}

func GetDocumentCache(descriptor string, store DocStore) (DocumentCache, error) {
	i := strings.IndexRune(descriptor, ':')
	name := ""
	if i >= 0 {
		name = descriptor[:i]
		descriptor = descriptor[i+1:]
	}
	if allCacheMethods == nil {
		return nil, fmt.Errorf("no storage backends have been initialized")
	}

	if f, ok := allCacheMethods[name]; ok {
		return f(descriptor, store)
	}
	return nil, fmt.Errorf("storage backend '%s' not registered", name)
}
