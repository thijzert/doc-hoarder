package gitstore

import (
	"context"
	"io"
	"io/fs"
	"log"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gitpl "github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
)

func init() {
	ctx := context.Background()
	r := &repo{
		path: "/tmp/test.git",
	}
	log.Print(r.Init(ctx))
	f, err := r.getFileFromBranch(ctx, "main", "README.md")
	if err != nil {
		log.Printf("failed to get readme: %v", err)
	} else {
		cts, _ := io.ReadAll(f)
		f.Close()
		log.Print(cts)
	}

	f, err = r.getFileFromBranch(ctx, "develop", "README.md")
	if err != nil {
		log.Printf("failed to get readme: %v %v", err, errors.Is(err, fs.ErrNotExist))
	} else {
		cts, _ := io.ReadAll(f)
		f.Close()
		log.Print(cts)
	}
}

type repo struct {
	path       string
	repository *git.Repository
}

func (g *repo) Init(ctx context.Context) error {
	if g.repository != nil {
		return nil
	}

	var err error
	g.repository, err = git.PlainOpen(g.path)

	mainBranch := "main"

	if err == nil {
		return nil
	} else if err != git.ErrRepositoryNotExists {
		return errors.Wrapf(err, "cannot open repository")
	}

	g.repository, err = git.PlainInit(g.path, true)
	if err != nil {
		return errors.Wrapf(err, "failed to create repository")
	}
	conf, _ := g.repository.Config()
	if conf == nil {
		conf = config.NewConfig()
	}
	conf.Init.DefaultBranch = mainBranch
	err = g.repository.SetConfig(conf)
	if err != nil {
		return errors.Wrapf(err, "failed to create repository")
	}
	b := &config.Branch{Name: mainBranch}
	err = g.repository.CreateBranch(b)
	if err != nil {
		return errors.Wrapf(err, "failed to create repository")
	}

	s := memory.NewStorage()
	fs := memfs.New()

	wcopy, err := git.Init(s, fs)
	if err != nil {
		return errors.Wrapf(err, "failed to create working copy")
	}
	err = wcopy.SetConfig(conf)
	if err != nil {
		return errors.Wrapf(err, "failed to create repository")
	}
	wt, err := wcopy.Worktree()
	if err != nil {
		return errors.Wrapf(err, "failed to create working copy")
	}
	_, err = wcopy.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{g.path},
	})
	if err != nil {
		return errors.Wrapf(err, "failed to add origin in working copy")
	}
	//err = wcopy.FetchContext(ctx, &git.FetchOptions{
	//	RemoteName: "origin",
	//	Depth:      2,
	//})
	//if err != nil {
	//	return errors.Wrapf(err, "failed to fetch origin in working copy")
	//}

	mainRef := gitpl.NewBranchReferenceName(mainBranch)

	err = wcopy.CreateBranch(&config.Branch{Name: mainBranch})
	if err != nil {
		return errors.Wrapf(err, "failed to create branch in working copy")
	}
	//err = wt.Checkout(&git.CheckoutOptions{
	//	Branch: mainRef,
	//})
	//if err != nil {
	//	return errors.Wrapf(err, "failed to change branch in working copy")
	//}

	f, err := fs.Create("README.md")
	if err != nil {
		return errors.Wrapf(err, "failed to create README")
	}
	f.Write([]byte("Hello, world\n"))
	f.Close()

	_, err = wt.Add("README.md")
	if err != nil {
		return errors.Wrapf(err, "failed to add README")
	}
	mainHash, err := wt.Commit("initial commit", &git.CommitOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to commit README")
	}

	if mainBranch != "master" {
		// For some reason it's impossible to change the branch in an empty repository
		err = wt.Checkout(&git.CheckoutOptions{
			Branch: mainRef,
			Create: true,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create branch in working copy")
		}
	}

	err = wcopy.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(string(mainRef) + ":" + string(mainRef))},
	})
	if err != nil {
		return errors.Wrapf(err, "failed to push results")
	}

	head := gitpl.NewHashReference(gitpl.HEAD, mainHash)
	err = g.repository.Storer.SetReference(head)
	if err != nil {
		return errors.Wrapf(err, "failed to update HEAD")
	}
	return nil
}

func (g *repo) getFileFromBranch(ctx context.Context, branch, filename string) (io.ReadCloser, error) {
	ref, err := g.repository.Reference(gitpl.NewBranchReferenceName(branch), false)
	if err != nil {
		return nil, fs.ErrNotExist
	}

	cmt, err := g.repository.CommitObject(ref.Hash())
	if err != nil {
		return nil, errors.Wrap(err, "error getting commit obj")
	}

	tree, err := g.repository.TreeObject(cmt.TreeHash)
	if err != nil {
		return nil, errors.Wrap(err, "error getting tree obj")
	}

	for _, ent := range tree.Entries {
		if ent.Name == filename {
			blob, err := g.repository.BlobObject(ent.Hash)
			if err != nil {
				return nil, errors.Wrap(err, "error getting blob")
			}

			return blob.Reader()
		}
	}

	return nil, fs.ErrNotExist
}

// DocumentIDs lists all IDs for documents in this store
//func (g *repo) DocumentIDs(context.Context) ([]string, error)

// NewDocumentID generates a new document ID that is not yet present in this store
//func (g *repo) NewDocumentID(context.Context) (string, error)

// GetDocument starts a transaction for a document ID
//func (g *repo) GetDocument(string) (DocTransaction, error)

type transaction struct {
	repo repo
}

//func (t *transaction) DocumentID() string

//func (t *transaction) ReadRootFile(context.Context, string) (io.ReadCloser, error)
//func (t *transaction) WriteRootFile(context.Context, string) (io.WriteCloser, error)

//func (t *transaction) ListAttachments(context.Context) ([]string, error)
//func (t *transaction) ReadAttachment(context.Context, string) (io.ReadCloser, error)
//func (t *transaction) NewAttachmentID(context.Context, string) (string, error)
//func (t *transaction) WriteAttachment(context.Context, string) (io.WriteCloser, error)

//func (t *transaction) Commit(context.Context, string) error
//func (t *transaction) Rollback() error
