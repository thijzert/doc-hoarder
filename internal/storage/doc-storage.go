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
	ReadRootFile(context.Context, string) (io.ReadCloser, error)
	WriteRootFile(context.Context, string) (io.WriteCloser, error)

	ListAttachments(context.Context) ([]string, error)
	ReadAttachment(context.Context, string) (io.ReadCloser, error)
	NewAttachmentID(context.Context, string) (string, error)
	WriteAttachment(context.Context, string) (io.WriteCloser, error)

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

type ExtensionKnower interface {
	AttachmentNameFromID(context.Context, string) (string, error)
}

func AttachmentNameFromID(ctx context.Context, trns DocTransaction, att_id string) (string, error) {
	if ek, ok := trns.(ExtensionKnower); ok {
		return ek.AttachmentNameFromID(ctx, att_id)
	}

	var f io.ReadCloser
	var err error
	for _, e := range []string{"css", "svg", "png", "jpeg", "ico"} {
		attName := "t" + att_id + "." + e
		f, err = trns.ReadAttachment(ctx, attName)
		if err == nil {
			f.Close()
			return attName, nil
		}
	}
	return "", err
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
