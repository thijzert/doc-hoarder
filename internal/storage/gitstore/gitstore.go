package gitstore

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	gitpl "github.com/go-git/go-git/v5/plumbing"
	gitst "github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/pkg/errors"
	"github.com/thijzert/doc-hoarder/internal/storage"
)

const (
	mainBranch string = "main"
)

func init() {
	storage.RegisterStorageMethod("git", func(rootPath string) (storage.DocStore, error) {
		r := &repo{
			path: rootPath,
		}

		ctx := context.Background() //FIXME
		err := r.Init(ctx)
		if err != nil {
			return nil, err
		}
		return r, nil
	})
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
func (g *repo) DocumentIDs(ctx context.Context) ([]string, error) {
	branches, err := g.repository.Branches()
	if err != nil {
		return nil, err
	}
	defer branches.Close()

	rv := []string{}
	err = branches.ForEach(func(ref *gitpl.Reference) error {
		if ctx.Err() != nil {
			return gitst.ErrStop
		}
		name := string(ref.Name())
		if len(name) != 22 {
			return nil
		}

		var id uint64
		if _, err := fmt.Sscanf(name, "refs/heads/g%010x", &id); err != nil {
			return nil
		}
		rv = append(rv, name[12:])
		return nil
	})
	return rv, err
}

// NewDocumentID generates a new document ID that is not yet present in this store
func (g *repo) NewDocumentID(ctx context.Context) (string, error) {
	rv := ""
	tries := 10
	for ctx.Err() == nil {
		tries--
		if tries < 0 {
			return "", errors.New("failed to generate a document ID")
		}
		rv = storage.NewDocumentID()
		_, err := g.repository.Reference(gitpl.NewBranchReferenceName("g"+rv), false)
		if err != nil {
			if errors.Is(err, gitpl.ErrReferenceNotFound) {
				break
			}
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	err := g.repository.CreateBranch(&config.Branch{Name: "g" + rv})
	return rv, err
}

// GetDocument starts a transaction for a document ID
func (g *repo) GetDocument(id string) (storage.DocTransaction, error) {
	ctx := context.Background()
	cl, err := git.CloneContext(ctx, memory.NewStorage(), memfs.New(), &git.CloneOptions{
		URL: g.path,
	})
	if err != nil {
		return nil, err
	}

	brref := gitpl.NewBranchReferenceName("g" + id)
	b, err := g.repository.Reference(brref, false)
	if errors.Is(err, gitpl.ErrReferenceNotFound) {
		b, err = g.repository.Reference(gitpl.NewBranchReferenceName(mainBranch), false)
	}
	if err != nil {
		return nil, errors.Wrap(err, "cannot get id")
	}

	wt, err := cl.Worktree()
	if err != nil {
		return nil, errors.Wrap(err, "cannot get worktree")
	}

	gco := &git.CheckoutOptions{
		Branch: brref,
		Create: true,
		Hash:   b.Hash(),
	}
	err = wt.Checkout(gco)
	if err != nil {
		return nil, errors.Wrap(err, "cannot switch branches")
	}

	return &transaction{
		repo:  g,
		clone: cl,
		dir:   "g" + id,
	}, nil
}

type transaction struct {
	repo  *repo
	clone *git.Repository
	dir   string
}

func (t *transaction) DocumentID() string {
	return t.dir[1:]
}

func (t *transaction) ReadRootFile(ctx context.Context, name string) (io.ReadCloser, error) {
	wt, err := t.clone.Worktree()
	if err != nil {
		return nil, err
	}
	return wt.Filesystem.Open(path.Join(t.dir, name))
}
func (t *transaction) WriteRootFile(ctx context.Context, name string) (io.WriteCloser, error) {
	wt, err := t.clone.Worktree()
	if err != nil {
		return nil, err
	}

	filename := path.Join(t.dir, name)
	rv, err := wt.Filesystem.Create(filename)
	if err != nil {
		return nil, err
	}

	_, err = wt.Add(filename)
	if err != nil {
		rv.Close()
		return nil, err
	}

	return rv, nil
}

func (t *transaction) ListAttachments(context.Context) ([]string, error) {
	wt, err := t.clone.Worktree()
	if err != nil {
		return nil, err
	}
	fileInfos, err := wt.Filesystem.ReadDir(path.Join(t.dir, "att"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	rv := []string{}
	for _, fi := range fileInfos {
		n := fi.Name()
		if fi.IsDir() || len(n) < 12 || n[0] != 't' || n[11] != '.' {
			continue
		}

		var id int64
		var ext string
		if _, err := fmt.Sscanf(n, "t%010x.%s", &id, &ext); err != nil {
			continue
		}
		rv = append(rv, n[1:11])
	}

	return rv, nil
}
func (t *transaction) ReadAttachment(ctx context.Context, name string) (io.ReadCloser, error) {
	wt, err := t.clone.Worktree()
	if err != nil {
		return nil, err
	}
	return wt.Filesystem.Open(path.Join(t.dir, "att", name))
}
func (t *transaction) NewAttachmentID(ctx context.Context, ext string) (string, error) {
	wt, err := t.clone.Worktree()
	if err != nil {
		return "", err
	}
	wt.Filesystem.MkdirAll(path.Join(t.dir, "att"), 0755)

	var rv string
	for ctx.Err() == nil {
		rv = storage.NewDocumentID()
		fp := path.Join(t.dir, "att", "t"+rv+"."+ext)
		_, err := wt.Filesystem.Stat(fp)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				g, err := wt.Filesystem.Create(fp)
				if err == nil {
					g.Close()
					wt.Add(fp)
					return rv, nil
				}
			} else {
				return "", err
			}
		}
	}
	return "", ctx.Err()
}
func (t *transaction) WriteAttachment(ctx context.Context, name string) (io.WriteCloser, error) {
	wt, err := t.clone.Worktree()
	if err != nil {
		return nil, err
	}
	wt.Filesystem.MkdirAll(path.Join(t.dir, "att"), 0755)
	fp := path.Join(t.dir, "att", name)
	rv, err := wt.Filesystem.Create(fp)
	wt.Add(fp)
	return rv, err
}

func (t *transaction) Commit(ctx context.Context, logMessage string) error {
	wt, err := t.clone.Worktree()
	if err != nil {
		return err
	}

	opts := git.CommitOptions{
		All: true,
	}
	_, err = wt.Commit(logMessage, &opts)
	if err != nil {
		return err
	}

	ref := gitpl.NewBranchReferenceName(t.dir)
	push := git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(string(ref) + ":" + string(ref)),
		},
	}
	return t.clone.PushContext(ctx, &push)
}
func (t *transaction) Rollback() error {
	t.clone = nil
	return nil
}
