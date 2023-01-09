package gauntlet

import (
	"context"
	"testing"

	"github.com/thijzert/doc-hoarder/internal/storage"
)

func TestImportGauntlet(t *testing.T) {
	ctx := context.Background()

	for _, srcscheme := range gauntletSchemes {
		for _, tgtscheme := range gauntletSchemes {
			t.Run(srcscheme+"→"+tgtscheme, func(t *testing.T) {
				tgtdir := t.TempDir()
				tgt, err := storage.GetDocStore(tgtscheme + ":" + tgtdir)
				t.Logf("target repository of type %T %#v", tgt, tgt)
				if err != nil {
					t.Fatalf("Cannot initialize doc store %s:%s: %v", tgtscheme, tgtdir, err)
				}

				srcdir := t.TempDir()
				err = extractTar(srcdir, "testdata/"+srcscheme+".tar")
				if err != nil {
					t.Errorf("failed to open %s.tar: %v", srcscheme, err)
					return
				}

				src, err := storage.GetDocStore(srcscheme + ":" + srcdir)
				t.Logf("source repository of type %T %#v", src, src)
				if err != nil {
					t.Fatalf("Cannot initialize doc store %s:%s: %v", srcscheme, srcdir, err)
				}

				err = storage.Copy(ctx, tgt, src)
				if err != nil {
					t.Fatalf("Unable to import %s into %s:  %v", srcscheme, tgtscheme, err)
				} else {
					RunStorageGauntlet(ctx, t, tgt)
				}
			})
		}
	}
}
