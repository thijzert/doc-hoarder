package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/thijzert/go-resemble"
)

type job func(ctx context.Context) error

type compileConfig struct {
	Development bool
	Quick       bool
	BaseURL     string
	Domain      string
	GOOS        string
	GOARCH      string
}

func main() {
	if _, err := os.Stat("web/assets"); err != nil {
		log.Fatalf("Error: cannot find doc-hoarder assets directory. (error: %s)\nAre you running this from the repository root?", err)
	}

	var conf compileConfig
	watch := false
	run := false
	flag.BoolVar(&conf.Development, "development", false, "Create a development build")
	flag.BoolVar(&conf.Quick, "quick", false, "Create a development build")
	flag.StringVar(&conf.BaseURL, "base-url", "", "Base address where this application will run")
	flag.StringVar(&conf.GOARCH, "GOARCH", "", "Cross-compile for architecture")
	flag.StringVar(&conf.GOOS, "GOOS", "", "Cross-compile for operating system")
	flag.BoolVar(&watch, "watch", false, "Watch source tree for changes")
	flag.BoolVar(&run, "run", false, "Run doc-hoarder upon successful compilation")
	flag.Parse()

	u, err := url.Parse(conf.BaseURL)
	if err == nil {
		conf.Domain = u.Host
	}

	if conf.Development && conf.Quick {
		log.Printf("")
		log.Printf("You requested a quick build. This will assume")
		log.Printf(" you have a version of  `gulp watch`  running")
		log.Printf(" in a separate process.")
		log.Printf("")
	}

	var theJob job

	if run {
		theJob = func(ctx context.Context) error {
			err := compile(ctx, conf)
			if err != nil {
				return err
			}
			runArgs := append([]string{"cmd/hoard/hoard"}, flag.Args()...)
			return passthru(ctx, runArgs...)
		}
	} else {
		theJob = func(ctx context.Context) error {
			return compile(ctx, conf)
		}
	}

	if watch {
		theJob = watchSourceTree([]string{"."}, []string{
			"*.go",
			"web/assets/extensions/*/*",
			"web/assets/extensions/*/*/*",
		}, theJob)
	}

	err = theJob(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}

func compile(ctx context.Context, conf compileConfig) error {
	// Build browser extensions
	if conf.BaseURL != "" {
		f, err := os.Open("web/assets/extensions")
		if err != nil {
			return errors.Errorf("cannot list extensions: %v", err)
		}
		fis, err := f.ReadDir(-1)
		if err != nil {
			return errors.Errorf("cannot list extensions: %v", err)
		}

		for _, fi := range fis {
			if !fi.IsDir() || fi.Name() == "" || fi.Name()[:1] == "." || fi.Name()[:1] == "_" {
				continue
			}
			err = buildBrowserExt(ctx, conf, fi.Name())
			if err != nil {
				return errors.Errorf("error building extension '%s': %v", fi.Name(), err)
			}
		}
	}

	// Compile static assets
	if !conf.Development || !conf.Quick {
		// TODO: compile CSS
		var err error = nil
		if err != nil {
			return errors.WithMessage(err, "error compiling assets")
		}
	}

	// Embed static assets
	if err := os.Chdir("web/assets"); err != nil {
		return errors.Errorf("Error: cannot find doc-hoarder assets directory. (error: %s)\nAre you *sure* you're running this from the repository root?", err)
	}
	var emb resemble.Resemble
	emb.OutputFile = "../plumbing/assets.go"
	emb.PackageName = "plumbing"
	emb.Debug = conf.Development
	emb.AssetPaths = []string{
		".",
	}
	if err := emb.Run(); err != nil {
		return errors.WithMessage(err, "error running 'resemble'")
	}

	os.Chdir("../..")

	// Build main executable
	execOutput := "cmd/hoard/hoard"
	if runtime.GOOS == "windows" || conf.GOOS == "windows" {
		execOutput = "cmd/hoard/hoard.exe"
	}

	gofiles, err := filepath.Glob("cmd/hoard/*.go")
	if err != nil || gofiles == nil {
		return errors.WithMessage(err, "error: cannot find any go files to compile.")
	}
	compileArgs := append([]string{
		"build",
		"-o", execOutput,
		"-ldflags", "-X main.BaseURL=" + conf.BaseURL,
	}, gofiles...)

	compileCmd := exec.CommandContext(ctx, "go", compileArgs...)

	compileCmd.Env = append(compileCmd.Env, os.Environ()...)
	if conf.GOOS != "" {
		compileCmd.Env = append(compileCmd.Env, "GOOS="+conf.GOOS)
	}
	if conf.GOARCH != "" {
		compileCmd.Env = append(compileCmd.Env, "GOARCH="+conf.GOARCH)
	}

	err = passthruCmd(compileCmd)
	if err != nil {
		return errors.WithMessage(err, "compilation failed")
	}

	if conf.Development && !conf.Quick {
		log.Printf("")
		log.Printf("Development build finished. For best results,")
		log.Printf(" watch-compile CSS in a separate process.")
		log.Printf("")
	} else {
		log.Printf("Compilation finished.")
	}

	return nil
}

func buildBrowserExt(ctx context.Context, conf compileConfig, extName string) error {
	var f brextFileHandler

	var rv bytes.Buffer
	var zf *zip.Writer

	if conf.Development {
		f = func(fileName string, contents []byte) error {
			fpath := path.Join("build/extensions", extName, fileName)
			os.MkdirAll(path.Dir(fpath), 0755)
			g, err := os.Create(fpath)
			if err != nil {
				return err
			}
			defer g.Close()
			_, err = g.Write(contents)
			if err != nil {
				return err
			}
			return nil
		}
	} else {
		zf = zip.NewWriter(&rv)
		defer zf.Close()

		f = func(fileName string, contents []byte) error {
			g, err := zf.Create(fileName)
			if err != nil {
				return err
			}
			g.Write(contents)
			return nil
		}
	}

	err := browserExtRecurse(ctx, conf, extName, "", f)
	if err != nil {
		return err
	}

	if !conf.Development {
		zf.Close()

		g, err := os.Create(path.Join("web/assets/extensions", extName+".xpi"))
		if err != nil {
			return err
		}
		log.Printf("TODO: submit to AMO")
		_, err = io.Copy(g, &rv)

		if err != nil {
			return err
		}
	}

	return nil
}

func browserExtRecurse(ctx context.Context, conf compileConfig, extName, dir string, f brextFileHandler) error {
	d, err := os.Open(path.Join("web/assets/extensions", extName, dir))
	if err != nil {
		return err
	}
	fis, err := d.ReadDir(-1)
	if err != nil {
		return err
	}

	for _, fi := range fis {
		fn := fi.Name()
		if fn == "" || fn[:1] == "." {
			continue
		} else if fi.IsDir() {
			browserExtRecurse(ctx, conf, extName, path.Join(dir, fn), f)
			continue
		}

		contents, err := ioutil.ReadFile(path.Join("web/assets/extensions", extName, dir, fn))
		if err != nil {
			return err
		}

		if len(fn) > 5 && fn[len(fn)-4:] != ".png" {
			jss := string(contents)

			jss = strings.Replace(jss, "@xxxxxxxxxxxxxxxxxxxxxxxx", fmt.Sprintf("@%s", conf.Domain), -1)
			jss = strings.Replace(jss, "\"https://xxxxxxxxxxxxxxxxxxxxxxxx\"", fmt.Sprintf("\"%s\"", conf.BaseURL), -1)
			jss = strings.Replace(jss, "\"https://xxxxxxxxxxxxxxxxxxxxxxxx/ext/", fmt.Sprintf("\"%sext/", conf.BaseURL), -1)
			jss = strings.Replace(jss, "\"https://xxxxxxxxxxxxxxxxxxxxxxxx/*\"", fmt.Sprintf("\"%s*\"", conf.BaseURL), -1)

			contents = []byte(jss)
		}

		f(path.Join(dir, fn), contents)
	}

	return nil
}

type brextFileHandler func(fileName string, contents []byte) error

func passthru(ctx context.Context, argv ...string) error {
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	return passthruCmd(c)
}

func passthruCmd(c *exec.Cmd) error {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

func watchSourceTree(paths []string, fileFilter []string, childJob job) job {
	return func(ctx context.Context) error {
		var mu sync.Mutex
		for {
			lastHash := sourceTreeHash(paths, fileFilter)
			current := lastHash
			cctx, cancel := context.WithCancel(ctx)
			go func() {
				mu.Lock()
				err := childJob(cctx)
				if err != nil {
					log.Printf("child process: %s", err)
				}
				mu.Unlock()
			}()

			for lastHash == current {
				time.Sleep(250 * time.Millisecond)
				current = sourceTreeHash(paths, fileFilter)
			}

			log.Printf("Source change detected - rebuilding")
			cancel()
		}
	}
}

func sourceTreeHash(paths []string, fileFilter []string) string {
	h := sha1.New()
	for _, d := range paths {
		h.Write(directoryHash(0, d, fileFilter))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func directoryHash(level int, filePath string, fileFilter []string) []byte {
	h := sha1.New()
	h.Write([]byte(filePath))

	fi, err := os.Stat(filePath)
	if err != nil {
		return h.Sum(nil)
	}
	if fi.IsDir() {
		base := filepath.Base(filePath)
		if level > 0 {
			if base == ".git" || base == ".." || base == "node_modules" || base == "build" || base == "doc" {
				return []byte{}
			}
		}
		// recurse
		var names []string
		f, err := os.Open(filePath)
		if err == nil {
			names, err = f.Readdirnames(-1)
		}
		if err == nil {
			for _, name := range names {
				if name == "" || name[0] == '.' {
					continue
				}
				h.Write(directoryHash(level+1, path.Join(filePath, name), fileFilter))
			}
		}
	} else {
		if fileFilter != nil {
			found := false
			for _, pattern := range fileFilter {
				if ok, _ := filepath.Match(pattern, filePath); ok {
					found = true
				} else if ok, _ := filepath.Match(pattern, filepath.Base(filePath)); ok {
					found = true
				}
			}
			if !found {
				return []byte{}
			}
		}
		f, err := os.Open(filePath)
		if err == nil {
			io.Copy(h, f)
			f.Close()
		}
	}
	return h.Sum(nil)
}
