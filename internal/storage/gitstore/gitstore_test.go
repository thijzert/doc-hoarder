package gitstore

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"testing"
	"time"

	"github.com/pkg/errors"
)

func TestStorageGauntlet(t *testing.T) {
	ctx := context.Background()
	r := &repo{
		path: "/tmp/test.git",
	}
	t.Logf("initialising test repository: E:%v", r.Init(ctx))
	f, err := r.getFileFromBranch(ctx, "main", "README.md")
	if err != nil {
		t.Logf("failed to get readme: %v", err)
	} else {
		cts, _ := io.ReadAll(f)
		f.Close()
		t.Logf("readme.md: [%s]", cts)
	}

	f, err = r.getFileFromBranch(ctx, "develop", "README.md")
	if err != nil {
	} else {
		t.Logf("should not have been able to get readme: %v %v", err, errors.Is(err, fs.ErrNotExist))
		cts, _ := io.ReadAll(f)
		f.Close()
		log.Print(cts)
	}

	ids, err := r.DocumentIDs(ctx)
	if err != nil {
		t.Logf("could not get document IDs: %v", err)
	} else {
		t.Logf("Document IDs:")
		for _, id := range ids {
			t.Logf("   -  g%s", id)
		}
	}

	id, err := r.NewDocumentID(ctx)
	if err != nil {
		t.Logf("could not generate new document ID: %v", err)
	} else {
		t.Logf("Document ID: %s", id)
	}

	trns, err := r.GetDocument("1234567890")
	if err != nil {
		t.Fatalf("could not start transaction: %v", err)
	} else {
		t.Logf("transaction: %v", trns)
	}

	f, err = trns.ReadRootFile(ctx, "document.bin")
	if err != nil {
		t.Fatalf("could not read root file: %v", err)
	} else {
		cts, _ := io.ReadAll(f)
		f.Close()
		t.Logf("document: %s", cts)
	}
	g, err := trns.WriteRootFile(ctx, "document.bin")
	if err != nil {
		t.Fatalf("could not write root file: %v", err)
	} else {
		fmt.Fprintf(g, "<!DOCTYPE html>\n<html>\n<body>\n<p>Hello, world</p>\n<p>Date: %s</p>\n</body>\n</html>", time.Now())
		g.Close()
	}

	atts, err := trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	}
	t.Logf("Current attachments: %v", atts)

	id, err = trns.NewAttachmentID(ctx, "txt")
	if err != nil {
		t.Fatalf("could not create new attachment ID: %v", err)
	}
	g, err = trns.WriteAttachment(ctx, "t"+id+".txt")
	if err != nil {
		t.Fatalf("could not write attachment ID %s: %v", id, err)
	} else {
		fmt.Fprintf(g, "Date: %s", time.Now())
		g.Close()
	}

	atts, err = trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	}
	t.Logf("Current attachments: %v", atts)

	err = trns.Commit(ctx, fmt.Sprintf("'go test' run at %s", time.Now()))
	if err != nil {
		t.Fatalf("could not commit transaction: %v", err)
	}

	t.Logf("oh hey it's working")
}
