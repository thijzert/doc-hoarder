package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/thijzert/go-resemble"
)

type job func(ctx context.Context) error

type compileConfig struct {
	Development bool
	Quick       bool
	BaseURL     string
	Domain      string
	AMO         struct {
		Issuer string
		Secret string
	} `json:"-"`
	Version string
	GOOS    string
	GOARCH  string
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
	flag.StringVar(&conf.AMO.Issuer, "amo-issuer", "", "Issuer ID for API key for addons.mozilla.org")
	flag.StringVar(&conf.AMO.Secret, "amo-secret", "", "Secret for API key for addons.mozilla.org")
	flag.StringVar(&conf.GOARCH, "GOARCH", "", "Cross-compile for architecture")
	flag.StringVar(&conf.GOOS, "GOOS", "", "Cross-compile for operating system")
	flag.BoolVar(&watch, "watch", false, "Watch source tree for changes")
	flag.BoolVar(&run, "run", false, "Run doc-hoarder upon successful compilation")
	flag.Parse()

	if conf.BaseURL == "" {
		log.Fatalf("Please set the Base URL using the --base-url flag")
	}

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
			runArgs := append([]string{"build/hoard"}, flag.Args()...)
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
	// Determine version
	conf.Version = "unknown-version"
	gitDescCmd := exec.CommandContext(ctx, "git", "describe")
	gitDescribe, err := gitDescCmd.Output()
	if err == nil && len(gitDescribe) > 0 {
		conf.Version = strings.TrimLeft(strings.TrimSpace(string(gitDescribe)), "v")
	}

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
	execOutput := "build/hoard"
	if runtime.GOOS == "windows" || conf.GOOS == "windows" {
		execOutput = "build/hoard.exe"
	}

	gofiles, err := filepath.Glob("cmd/hoard/*.go")
	if err != nil || gofiles == nil {
		return errors.WithMessage(err, "error: cannot find any go files to compile.")
	}
	compileArgs := append([]string{
		"build",
		"-o", execOutput,
		"-ldflags", fmt.Sprintf("-X 'main.Version=%s' -X 'main.BaseURL=%s'", conf.Version, conf.BaseURL),
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
		defer g.Close()
		if conf.AMO.Issuer != "" {
			err := signedExtension(ctx, g, conf, extName, &rv)
			if err != nil {
				return err
			}
		} else {
			log.Printf("Remember to have this extension signed by Mozilla")
			_, err = io.Copy(g, &rv)
		}

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

			jss = strings.Replace(jss, "\"x.y.z-w-gdeadbeef\"", fmt.Sprintf("\"%s\"", conf.Version), -1)
			jss = strings.Replace(jss, " x.y.z-w-gdeadbeef<", fmt.Sprintf(" %s<", conf.Version), -1)
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

func signedExtension(ctx context.Context, dest io.Writer, conf compileConfig, extName string, xpi io.Reader) error {

	submitted := false

	extStatus := struct {
		Active           bool `json:"active"`
		AutomatedSigning bool `json:"automated_signing"`
		Files            []struct {
			DownloadURL string `json:"download_url"`
			Hash        string `json:"hash"`
			Signed      bool   `json:"signed"`
		} `json:"files"`
		PassedReview      bool        `json:"passed_review"`
		Processed         bool        `json:"processed"`
		Reviewed          interface{} `json:"reviewed"`
		Valid             bool        `json:"valid"`
		ValidationResults interface{} `json:"validation_results"`
		Version           string      `json:"version"`
	}{}

	cl := &http.Client{
		Timeout: 15 * time.Second,
	}

	versionURL := fmt.Sprintf("https://addons.mozilla.org/api/v5/addons/%s@%s/versions/%s/", extName, conf.Domain, conf.Version)

	for ctx.Err() == nil {
		r, err := amoRequest(ctx, conf, "GET", versionURL, nil)
		err = doJson(cl, r, &extStatus)
		if err != nil {
			if a, ok := err.(apiError); ok {
				if a.StatusCode == 404 && !submitted {
					log.Printf("Submitting %s.xpi to addons.mozilla.org for siging... ", extName)

					var fileUpload bytes.Buffer
					mp := multipart.NewWriter(&fileUpload)
					w, err := mp.CreateFormFile("upload", fmt.Sprintf("%s.xpi", extName))
					if err != nil {
						return err
					}
					_, err = io.Copy(w, xpi)
					if err != nil {
						return err
					}
					mp.Close()

					r, err := amoRequest(ctx, conf, "PUT", versionURL, &fileUpload)
					r.Header.Set("Content-Type", "multipart/form-data; boundary="+mp.Boundary())
					err = doJson(cl, r, &extStatus)
					if err != nil {
						return err
					}
					submitted = true
					time.Sleep(8500 * time.Millisecond)
					continue
				}
			}
			return err
		}

		// log.Printf("Extension status: %+v", extStatus)

		if len(extStatus.Files) > 0 && extStatus.Files[0].Signed && extStatus.Files[0].DownloadURL != "" {
			log.Printf("Downloading signed %s.xpi from AMO", extName)
			r, err := amoRequest(ctx, conf, "GET", extStatus.Files[0].DownloadURL, nil)
			resp, err := cl.Do(r)
			if err != nil {
				return err
			}
			_, err = io.Copy(dest, resp.Body)
			return err
		}

		time.Sleep(14500 * time.Millisecond)
	}

	return ctx.Err()
}

func amoRequest(ctx context.Context, conf compileConfig, method string, url string, body io.Reader) (*http.Request, error) {
	// Create the Claims
	t := time.Now().Unix()
	claims := &jwt.StandardClaims{
		Issuer:    conf.AMO.Issuer,
		Id:        randomString(),
		IssuedAt:  t,
		ExpiresAt: t + 90,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(conf.AMO.Secret))
	if err != nil {
		return nil, err
	}

	r, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	r.Header.Add("Authorization", "JWT "+ss)

	r = r.WithContext(ctx)
	return r, nil
}

type apiError struct {
	StatusCode int
	More       interface{}
}

func (a apiError) Error() string {
	if m, ok := a.More.(map[string]interface{}); ok && len(m) == 1 {
		if m["error"] != nil {
			return fmt.Sprintf("http status %d: %s", a.StatusCode, m["error"])
		} else if m["detail"] != nil {
			return fmt.Sprintf("http status %d: %s", a.StatusCode, m["detail"])
		}
	}
	return fmt.Sprintf("http status %d", a.StatusCode)
}

func doJson(cl *http.Client, r *http.Request, res interface{}) error {
	resp, err := cl.Do(r)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(resp.Body)

	if resp.StatusCode >= 400 {
		rv := apiError{StatusCode: resp.StatusCode}
		dec.Decode(&rv.More)
		return rv
	}

	return dec.Decode(res)
}

func randomString() string {
	rv := make([]byte, 0, 40)
	buf := make([]byte, 40)
	rand.Read(buf)
	var extraRandom byte = '4' // chosen by fair dice roll
	for _, c := range buf {
		c = c & 0x7f
		if c <= ' ' || c > '~' || c == '\\' || c == '"' {
			c = extraRandom ^ c
			if c <= ' ' || c > '~' || c == '\\' || c == '"' {
				continue
			}
		}
		rv = append(rv, c)
		extraRandom = c
	}

	return string(rv)
}

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
