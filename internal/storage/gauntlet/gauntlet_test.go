package gauntlet

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"testing"
	"time"

	"github.com/thijzert/doc-hoarder/internal/storage"
	_ "github.com/thijzert/doc-hoarder/internal/storage/gitstore"
)

func TestStorageGauntlet(t *testing.T) {
	ctx := context.Background()

	for _, scheme := range []string{"fs", "git"} {
		t.Run(scheme, func(t *testing.T) {
			dir := t.TempDir()
			err := extractTar(dir, "testdata/"+scheme+".tar")
			if err != nil {
				t.Errorf("failed to open %s.tar: %v", scheme, err)
				return
			}

			r, err := storage.GetDocStore(scheme + ":" + dir)
			t.Logf("repository of type %T %#v", r, r)
			if err != nil {
				t.Errorf("Cannot initialize doc store %s:%s: %v", scheme, dir, err)
			} else {
				RunStorageGauntlet(ctx, t, r)
			}
		})
	}
}

func extractTar(destDir, archiveFileName string) error {
	f, err := os.Open(archiveFileName)
	if err != nil {
		return err
	}
	defer f.Close()
	ar := tar.NewReader(f)
	fi, err := ar.Next()
	for fi != nil && err == nil {
		if fi.Typeflag == tar.TypeDir {
			fi, err = ar.Next()
			continue
		}

		buf := make([]byte, int(fi.Size))
		ar.Read(buf)
		d := path.Dir(fi.Name)
		if d != "" && d != "." {
			err = os.MkdirAll(path.Join(destDir, d), 0700)
			if err != nil {
				return err
			}
		}

		//log.Printf("creating %s", path.Join(destDir, fi.Name))
		g, err := os.Create(path.Join(destDir, fi.Name))
		if err != nil {
			return err
		}
		defer g.Close()
		_, err = g.Write(buf)
		if err != nil {
			return err
		}

		fi, err = ar.Next()
	}

	if errors.Is(err, io.EOF) {
		return nil
	} else {
		return err
	}
}

func RunStorageGauntlet(ctx context.Context, t *testing.T, r storage.DocStore) {
	onlyDocument := "43dbc153e5"
	onlyAttachment := "t7e2d9c3e82.css"

	ids, err := r.DocumentIDs(ctx)
	if err != nil {
		t.Fatalf("could not get document IDs: %v", err)
	} else if len(ids) != 1 || ids[0] != onlyDocument {
		t.Errorf("Expected only document %s; got:", onlyDocument)
		for _, id := range ids {
			t.Logf("   -  g%s", id)
		}
	}

	trns, err := r.GetDocument(onlyDocument)
	if err != nil {
		t.Fatalf("could not start transaction: %v", err)
	} else {
		t.Logf("transaction: %v", trns)
	}

	f, err := trns.ReadRootFile(ctx, "document.bin")
	if err != nil {
		t.Fatalf("could not read root file: %v", err)
	} else {
		cts, _ := io.ReadAll(f)
		f.Close()
		if len(cts) < 512 {
			t.Errorf("Conspicuously short document: (%d bytes)\n%s", len(cts), cts)
		}
	}

	atts, err := trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	} else if len(atts) != 1 || atts[0] != onlyAttachment {
		t.Errorf("Expected only attachment %s; got: %v", onlyAttachment, atts)
	}

	attid, err := trns.NewAttachmentID(ctx, "txt")
	if err != nil {
		t.Fatalf("could not create new attachment ID: %v", err)
	}
	otherAttachment := "t" + attid + ".txt"
	g, err := trns.WriteAttachment(ctx, otherAttachment)
	if err != nil {
		t.Fatalf("could not write attachment ID %s: %v", attid, err)
	} else {
		fmt.Fprintf(g, "Date: %s", time.Now())
		g.Close()
	}

	atts, err = trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	} else if len(atts) != 2 || atts[0] == atts[1] || (atts[0] != onlyAttachment && atts[1] != onlyAttachment) || (atts[0] != otherAttachment && atts[1] != otherAttachment) {
		t.Errorf("Expected attachments %s and %s; got: %v", onlyAttachment, otherAttachment, atts)
	}
	t.Logf("Current attachments: %v", atts)

	err = trns.Rollback()
	if err != nil {
		t.Fatal(err)
	}

	id, err := r.NewDocumentID(ctx)
	if err != nil {
		t.Fatalf("could not generate new document ID: %v", err)
	} else {
		t.Logf("Document ID: %s", id)
	}

	trns, err = r.GetDocument(id)
	if err != nil {
		t.Fatalf("could not start transaction: %v", err)
	} else {
		t.Logf("transaction: %v", trns)
	}

	g, err = trns.WriteAttachment(ctx, onlyAttachment)
	if err != nil {
		t.Fatalf("could not write attachment ID %s: %v", onlyAttachment, err)
	} else {
		fmt.Fprintf(g, "Date: %s", time.Now())
		g.Close()
	}
	g, err = trns.WriteRootFile(ctx, "document.bin")
	if err != nil {
		t.Fatalf("could not write root file: %v", err)
	} else {
		fmt.Fprintf(g, "<!DOCTYPE html>\n<html>\n<body>\n<p>Hello, world</p>\n<p>Date: %s</p>\n</body>\n</html>", time.Now())
		g.Close()
	}

	err = trns.Commit(ctx, fmt.Sprintf("'go test' run at %s", time.Now()))
	if err != nil {
		t.Fatalf("could not commit transaction: %v", err)
	}

	// Try deleting an attachment
	trns, err = r.GetDocument(id)
	if err != nil {
		t.Fatalf("could not start transaction: %v", err)
	}
	atts, err = trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	} else if len(atts) != 1 || atts[0] != onlyAttachment {
		t.Errorf("Expected only attachment %s; got: %v", onlyAttachment, atts)
	}
	err = trns.DeleteAttachment(ctx, onlyAttachment)
	if err != nil {
		t.Fatalf("could not delete attachment: %v", err)
	}
	atts, err = trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	} else if len(atts) != 0 {
		t.Errorf("Expected no attachments; got: %v", atts)
	}
	err = trns.Commit(ctx, "Remove stylesheet")
	if err != nil {
		t.Fatalf("could not commit transaction: %v", err)
	}

	// Check that the deletion was permanent
	trns, err = r.GetDocument(id)
	if err != nil {
		t.Fatalf("could not start transaction: %v", err)
	}
	atts, err = trns.ListAttachments(ctx)
	if err != nil {
		t.Fatalf("could not list attachments: %v", err)
	} else if len(atts) != 0 {
		t.Errorf("Expected no attachments; got: %v", atts)
	}
	trns.Rollback()

	t.Logf("oh hey it's working")
}
