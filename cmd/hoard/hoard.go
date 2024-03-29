package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/thijzert/doc-hoarder/internal/storage"
	_ "github.com/thijzert/doc-hoarder/internal/storage/gitstore"
	"github.com/thijzert/doc-hoarder/web/plumbing"
	weberrors "github.com/thijzert/doc-hoarder/web/plumbing/errors"
	"github.com/thijzert/doc-hoarder/web/plumbing/login"
	"github.com/thijzert/doc-hoarder/web/plumbing/sessions"
	"github.com/thijzert/go-rcfile"
)

var Version string
var BaseURL string
var Domain string

func main() {
	if BaseURL == "" {
		log.Fatal("baseURL not compiled in")
	}
	u, err := url.Parse(BaseURL)
	if err == nil {
		Domain = u.Host
	}

	docStoreLocation := ""
	docCacheLocation := ""
	sessionStoreLocation := ""
	userStoreLocation := ""
	loginProviderID := ""

	cmdline := flag.NewFlagSet("dochoarder", flag.ContinueOnError)

	cmdline.StringVar(&docStoreLocation, "docstore", "", "Type and location for backend document store, e.g. 'fs:/path/to/documents'")
	cmdline.StringVar(&docCacheLocation, "documentcache", "", "Type and location for document cache")
	cmdline.StringVar(&sessionStoreLocation, "sessionstore", "", "Type and location for session store, e.g. 'file:/path/to/sessions.json'")
	cmdline.StringVar(&userStoreLocation, "userprofilestore", "", "Type and location for user profile store, e.g. 'file:/path/to/userprofile.json'")
	cmdline.StringVar(&loginProviderID, "login", "", "Type and URL for login provider, e.g. 'oidc:https://CLIENT_ID:CLIENT_SECRET@login.example.org/auth/realms/example'")

	rcfile.ParseInto(cmdline, "dochoarderrc")
	err = cmdline.Parse(os.Args[1:])
	if err == flag.ErrHelp {
		return
	} else if err != nil {
		cmdline.Usage()
		log.Panic(err)
	}

	log.Printf("Doc-hoarder version %s", Version)

	docStore, err := storage.GetDocStore(docStoreLocation)
	if err != nil {
		log.Fatal(err)
	}
	docCache, err := storage.GetDocumentCache(docCacheLocation, docStore)
	if err != nil {
		log.Fatal(err)
	}

	userStore, err := login.GetUserStore(userStoreLocation)
	if err != nil {
		log.Fatal(err)
	}
	sessStore, err := sessions.GetStore(sessionStoreLocation)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmdlineArgs := cmdline.Args()
	if len(cmdlineArgs) == 2 && cmdlineArgs[0] == "import" {
		// Open a second document store, import all documents into the main store, and exit

		if cmdlineArgs[1] == docStoreLocation {
			log.Fatal("Cannot import a document store into itself")
		}

		importDocs, err := storage.GetDocStore(cmdlineArgs[1])
		if err != nil {
			log.Fatalf("Unable to read source store: %v", err)
		}

		err = storage.Copy(ctx, docStore, importDocs)
		if err != nil {
			log.Fatal(err)
		}

		return
	}

	// Default behaviour: open a web server

	// Clean out old stale sessions from the store
	go func(ctx context.Context) {
		tick := time.NewTicker(10 * time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				err := sessStore.Prune(ctx)
				if err != nil {
					log.Printf("error pruning session store: %v", err)
				}
			}
		}
	}(ctx)

	lg, err := login.OIDCFromURL(ctx, loginProviderID[5:], userStore, BaseURL, "/auth/callback")
	if err != nil {
		log.Fatal(err)
	}

	mustLogin := func(h http.Handler) http.Handler {
		h = lg.Must(h)
		h = sessions.WithSession(sessStore, h)
		return h
	}
	mayLogin := func(h http.Handler) http.Handler {
		h = lg.May(h)
		h = sessions.WithSession(sessStore, h)
		return h
	}

	shouldKey := login.MustHaveAPIKey(userStore)
	mustKey := func(h http.Handler, scope string) http.Handler {
		return plumbing.CORS(shouldKey(h, scope))
	}

	mux := http.NewServeMux()
	mux.Handle("/", plumbing.LandingPageOnly(mayLogin(plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		user, _ := login.GetUser(r)
		ids, metas, err := docCache.GetDocuments(r.Context(), string(user.ID), storage.Limit{0, 200})
		if err != nil {
			return nil, err
		}

		return struct {
			IDs   []string
			Metas []storage.DocumentMeta
		}{ids, metas}, nil
	}), "page/home"))))
	mux.Handle("/auth/callback", sessions.WithSession(sessStore, lg.Callback()))

	mux.Handle("/login", mustLogin(plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		return nil, plumbing.Redirect(302, ".")
	}), "page/home")))
	mux.Handle("/logout", mayLogin(plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		sess := sessions.GetSession(r)
		if sess != nil {
			sess.LoggedInAs = ""
			sess.Destroyed = true
			sess.Dirty = true
		}
		return nil, plumbing.Redirect(302, ".")
	}), "page/home")))
	mux.Handle("/user/profile", mustLogin(plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		user, _ := login.GetUser(r)
		allApikeys, err := userStore.GetAPIKeysForUser(r.Context(), user.ID)
		if err != nil {
			return nil, err
		}

		apikeys := make([]login.APIKey, 0, len(allApikeys))
		for _, k := range allApikeys {
			if !k.Disabled {
				apikeys = append(apikeys, k)
			}
		}
		return struct {
			APIKeys []login.APIKey
		}{apikeys}, nil
	}), "page/user-profile")))

	mux.Handle("/assets/ui-showcase", plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		return nil, nil
	}), "page/ui"))
	mux.Handle("/assets/js/", plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		jspath := strings.Replace(r.URL.Path, "./", "-", -1)
		if len(jspath) < 12 {
			return nil, plumbing.ErrNotFound
		}

		js, err := plumbing.GetAsset(jspath[8:])
		if err != nil {
			return nil, plumbing.ErrNotFound
		}

		return plumbing.Blob{
			ContentType: "application/javascript",
			Contents:    js,
		}, nil
	}), "page/asset"))
	mux.Handle("/assets/", plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		asspath := strings.Replace(r.URL.Path, "./", "-", -1)
		if len(asspath) < 12 {
			return nil, plumbing.ErrNotFound
		}

		js, err := plumbing.GetAsset(asspath[8:])
		if err != nil {
			return nil, plumbing.ErrNotFound
		}
		rv := plumbing.Blob{
			Contents: js,
		}

		if a := strings.LastIndex(asspath, "."); a >= 0 {
			rv.ContentType = mime.TypeByExtension(asspath[a:])
		}

		return rv, nil
	}), "page/asset"))

	mux.Handle("/ext/updates.json", plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		type versionInfo struct {
			Version    string `json:"version"`
			UpdateLink string `json:"update_link"`
		}
		type addonInfo struct {
			Updates []versionInfo `json:"updates"`
		}
		rv := struct {
			Addons map[string]addonInfo `json:"addons"`
		}{make(map[string]addonInfo)}

		addonList := []string{"hoard"}
		for _, addon := range addonList {
			rv.Addons[addon+"@"+Domain] = addonInfo{
				Updates: []versionInfo{
					versionInfo{Version, BaseURL + "ext/" + addon + ".xpi"},
				},
			}
		}

		return rv, nil
	})))
	mux.Handle("/ext/", plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		parts := strings.Split(r.URL.Path, "/")
		extName := parts[2]
		if len(extName) < 5 || extName[0] == '.' || extName[len(extName)-4:] != ".xpi" {
			return nil, plumbing.ErrNotFound
		}

		ext, err := plumbing.GetAsset(path.Join("extensions", "_signed", extName))
		if err != nil {
			ext, err = plumbing.GetAsset(path.Join("extensions", extName))
		}
		if err != nil {
			return nil, plumbing.ErrNotFound
		}

		return plumbing.Blob{
			ContentType: "application/x-xpinstall",
			Contents:    ext,
		}, nil
	}), "page/asset"))

	mux.Handle("/api/user/new-api-key", mustLogin(plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		user, _ := login.GetUser(r)

		scope := r.FormValue("scope")
		if scope == "" {
			return nil, weberrors.BadRequest("invalid scope")
		}

		secret, err := userStore.NewAPIKeyForUser(r.Context(), user.ID, r.FormValue("label"))
		if err != nil {
			return nil, err
		}

		return struct {
			APIKey string `json:"apikey"`
		}{secret}, nil
	}))))
	mux.Handle("/api/user/disable-api-key", mustLogin(plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		user, _ := login.GetUser(r)

		err := userStore.DisableAPIKey(r.Context(), user.ID, login.KeyID(r.FormValue("key_id")))
		if err != nil {
			return nil, err
		}

		return struct {
			Message string `json:"_"`
		}{"ok"}, nil
	}))))

	mux.Handle("/api/user/whoami", mustKey(plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		user, _ := login.GetUser(r)
		rv := struct {
			Ok int `json:"ok"`
			Hello string `json:"hello"`
		}{1, user.GivenName}
		if rv.Hello == "" {
			rv.Hello = user.FullName
		}
		if rv.Hello == "" {
			rv.Hello = "dear user"
		}
		return rv, nil
	})), ""))

	var txmu sync.Mutex
	transactions := make(map[string]cachedTx)

	mux.Handle("/api/new-doc", mustKey(plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		docid, err := docStore.NewDocumentID(r.Context())
		if err != nil {
			return nil, err
		}
		trns, err := docStore.GetDocument(docid)
		if err != nil {
			return nil, err
		}

		user, _ := login.GetUser(r)
		txid := newTxid()

		txmu.Lock()
		transactions[txid] = cachedTx{
			Tx:   trns,
			User: user,
		}
		txmu.Unlock()

		res := struct {
			ID   string `json:"id"`
			Txid string `json:"txid"`
		}{
			ID:   docid,
			Txid: txid,
		}
		return res, nil
	})), "document.create"))
	mux.Handle("/api/capture-new-doc", mustKey(plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		user, ok := login.GetUser(r)
		if !ok {
			return nil, errors.New("nil user")
		}
		page_url := r.FormValue("page_url")

		docid := ""
		trns, ok, err := docCache.GetDocumentByURL(r.Context(), string(user.ID), page_url)
		if err != nil {
			log.Printf("Error reusing transaction: %v", err)
			return nil, err
		}
		if ok {
			docid = trns.DocumentID()
			log.Printf("reused '%s'", docid)

			// Delete all existing attachments
			atts, err := trns.ListAttachments(r.Context())
			if err != nil {
				trns.Rollback()
				return nil, err
			}
			for _, attid := range atts {
				trns.DeleteAttachment(r.Context(), attid)
			}
		} else {
			docid, err = docStore.NewDocumentID(r.Context())
			if err != nil {
				return nil, err
			}
			trns, err = docStore.GetDocument(docid)
			if err != nil {
				return nil, err
			}
		}

		meta := storage.DocumentMeta{
			URL:    page_url,
			Status: storage.StatusDraft,
		}
		meta.Permissions.Owner = string(user.ID)
		err = storage.WriteMeta(r.Context(), trns, meta)
		if err != nil {
			return nil, err
		}

		txid := newTxid()

		txmu.Lock()
		transactions[txid] = cachedTx{
			Tx:   trns,
			User: user,
		}
		txmu.Unlock()

		res := struct {
			ID   string `json:"id"`
			Txid string `json:"txid"`
		}{
			ID:   docid,
			Txid: txid,
		}
		return res, nil
	})), "document.create"))

	const txidParamname string = "txid"
	mustDraft := func(f func(r *http.Request, trns storage.DocTransaction) (interface{}, error)) http.Handler {
		return plumbing.AsJSON(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
			txid := r.FormValue(txidParamname)
			if len(txid) < 10 {
				return nil, weberrors.BadRequest("invalid draft ID")
			}

			txmu.Lock()
			trns, ok := transactions[txid]
			txmu.Unlock()
			if !ok {
				return nil, weberrors.BadRequest("invalid draft ID")
			}

			// meta, err := cache.GetDocumentMeta(draft_id)

			trns.Mu.Lock() // FIXME: is this necessary, really?
			rv, err := f(r, trns.Tx)
			trns.Mu.Unlock()

			return rv, err
		}))
	}

	mux.Handle("/api/finalize-draft", mustDraft(func(r *http.Request, trns storage.DocTransaction) (interface{}, error) {
		meta, err := storage.ReadMeta(r.Context(), trns)
		if err != nil {
			return nil, err
		}

		setForm := func(tgt *string, param string) {
			s := strings.TrimSpace(r.FormValue(param))
			if s != "" {
				*tgt = s
			}
		}

		setForm(&meta.Title, "doc_title")
		setForm(&meta.Author, "doc_author")
		setForm(&meta.IconID, "icon_id")

		meta.Status = "static"
		meta.CaptureDate = time.Now()

		err = storage.WriteMeta(r.Context(), trns, meta)
		if err != nil {
			return nil, err
		}

		logMessage := "Finalize upload"
		setForm(&logMessage, "log_message")

		err = trns.Commit(r.Context(), logMessage)
		if err != nil {
			return nil, err
		}

		// FIXME: figure out an elegant way of having mustDraft() delete the transaction from the cache. Right now the only reaon this doesn't delete arbitrary transactions is that the parameter name happens to be the same in both functions.
		txid := r.FormValue(txidParamname)
		txmu.Lock()
		delete(transactions, txid)
		txmu.Unlock()

		return okay("Document saved")
	}))

	mux.Handle("/api/new-attachment", mustDraft(func(r *http.Request, trns storage.DocTransaction) (interface{}, error) {
		ext := strings.ToLower(r.FormValue("ext"))
		if ext != "css" && ext != "svg" && ext != "png" && ext != "jpeg" && ext != "ico" && ext != "woff" && ext != "woff2" && ext != "eot" && ext != "ttf" { // TODO: this, but better
			return nil, plumbing.BadRequest("Invalid extension '%s'", ext)
		}

		attid_s, err := trns.NewAttachmentID(r.Context(), ext)
		if err != nil {
			return nil, err
		}
		attName := "t" + attid_s + "." + ext

		res := struct {
			ID       string `json:"attachment_id"`
			Filename string `json:"filename"`
		}{
			ID:       attid_s,
			Filename: "att/" + attName,
		}
		return res, nil
	}))

	mux.Handle("/api/upload-draft", mustDraft(func(r *http.Request, trns storage.DocTransaction) (interface{}, error) {
		f, _, err := r.FormFile("document")
		if err != nil {
			return nil, err
		}

		g, err := trns.WriteRootFile(r.Context(), "document.bin")
		if err != nil {
			return nil, err
		}
		defer g.Close()

		_, err = io.Copy(g, f)
		if err != nil {
			return nil, err
		}

		return okay("Chunk uploaded successfully")
	}))
	mux.Handle("/api/upload-attachment", mustDraft(func(r *http.Request, trns storage.DocTransaction) (interface{}, error) {
		att_id := strings.ToLower(r.FormValue("att_id"))
		attName, err := storage.AttachmentNameFromID(r.Context(), trns, att_id)
		if err != nil {
			return nil, plumbing.BadRequest("invalid attachment ID")
		}

		var b bytes.Buffer
		if r.FormValue("truncate") != "1" {
			// Read current contents into the buffer - the request contents will get appended
			curr, err := trns.ReadAttachment(r.Context(), attName)
			if err == nil {
				_, err = io.Copy(&b, curr)
			}
			if err != nil {
				return nil, err
			}
			curr.Close()
		}

		f, _, err := r.FormFile("attachment")
		if err != nil {
			return nil, err
		}
		defer f.Close()

		io.Copy(&b, f)

		g, err := trns.WriteAttachment(r.Context(), attName)
		if g == nil || err != nil {
			return nil, err
		}
		defer g.Close()

		_, err = io.Copy(g, &b)
		if err != nil {
			return nil, err
		}

		return okay("Chunk uploaded successfully")
	}))
	mux.Handle("/api/download-attachment", mustDraft(func(r *http.Request, trns storage.DocTransaction) (interface{}, error) {
		att_id := strings.ToLower(r.FormValue("att_id"))
		attName, err := storage.AttachmentNameFromID(r.Context(), trns, att_id)
		if err != nil {
			return nil, err
		}

		g, err := trns.ReadAttachment(r.Context(), attName)
		if err != nil {
			return nil, err
		}
		defer g.Close()

		rv := plumbing.Blob{}

		rv.Contents, err = ioutil.ReadAll(g)
		if err != nil {
			return nil, err
		}

		t := mime.TypeByExtension(path.Ext(attName))
		if t != "" {
			rv.ContentType = t
		}
		return rv, nil
	}))
	mux.Handle("/api/proxy-attachment", mustDraft(func(r *http.Request, trns storage.DocTransaction) (interface{}, error) {
		// TODO: proxy URL
		proxy_url, err := url.Parse(r.FormValue("url"))
		if err != nil || r.FormValue("url") == "" || (proxy_url.Scheme != "https" && proxy_url.Scheme != "http") {
			return nil, plumbing.BadRequest("invalid url '%s'", r.FormValue("url"))
		}

		pcl := &http.Client{
			Timeout: 15 * time.Second,
		}
		response, err := pcl.Get(proxy_url.String())
		if err != nil {
			return nil, err
		}
		ct := response.Header.Get("Content-Type")

		if (ct == "application/octet-stream" || ct == "binary/octet-stream") && len(proxy_url.Path) > 10 {
			// HACK: the remote server doesn't report MIME types - infer from the file extension
			lp := len(proxy_url.Path)
			if proxy_url.Path[lp-5:] == ".woff" {
				ct = "font/woff"
			} else if proxy_url.Path[lp-6:] == ".woff2" {
				ct = "font/woff2"
			} else if proxy_url.Path[lp-4:] == ".eot" {
				ct = "font/embedded-opentype"
			} else if proxy_url.Path[lp-4:] == ".ttf" || proxy_url.Path[lp-4:] == ".otf" {
				ct = "font/" + proxy_url.Path[lp-3:]
			}
		}

		exts, err := mime.ExtensionsByType(ct)

		if ct == "" || err != nil || len(exts) == 0 {
			return nil, plumbing.BadRequest("unknown mime type \"%s\"", ct)
		}
		if ct != "text/css" && !strstr(ct, "image/") && !strstr(ct, "font/") && ct != "application/font-woff" && !strstr(ct, "text/css;") {
			return nil, plumbing.BadRequest("subresource has invalid mime type '%s'", ct)
		}

		ext := exts[0][1:]
		if ct == "image/jpeg" {
			// HACK: I don't like the default JPEG extension
			ext = "jpeg"
		}

		attid_s, err := trns.NewAttachmentID(r.Context(), ext)
		if err != nil {
			return nil, err
		}
		attName := "t" + attid_s + "." + ext

		f, err := trns.WriteAttachment(r.Context(), attName)
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(f, response.Body)
		if err != nil {
			return nil, err
		}
		f.Close()

		res := struct {
			ID       string `json:"attachment_id"`
			Filename string `json:"filename"`
		}{
			ID:       attid_s,
			Filename: "att/" + attName,
		}
		return res, nil
	}))

	mux.Handle("/documents/view/", mayLogin(plumbing.AsHTML(plumbing.HandlerFunc(func(r *http.Request) (interface{}, error) {
		var docid int64
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) <= 3 {
			return nil, plumbing.ErrNotFound
		}

		_, err := fmt.Sscanf(parts[3], "g%010x", &docid)
		if err != nil {
			return nil, plumbing.ErrNotFound
		}

		if len(parts) == 4 {
			return nil, plumbing.Redirect(302, fmt.Sprintf("g%010x/", docid))
		}

		trns, err := docStore.GetDocument(fmt.Sprintf("%10x", docid))
		if err != nil {
			return nil, err
		}

		user, userOk := login.GetUser(r)
		meta, err := storage.ReadMeta(r.Context(), trns)
		if err != nil {
			return nil, err
		}

		if !meta.Permissions.Public {
			if !userOk {
				return nil, weberrors.ErrLoginRequired
			}
			if string(user.ID) == meta.Permissions.Owner {
				// This is fine
			} else {
				// TODO: check read permissions
				return nil, weberrors.Forbidden("You do not have permission to view this document")
			}
		}

		rv := plumbing.Blob{}

		if len(parts) >= 6 && parts[4] == "att" {
			f, err := trns.ReadAttachment(r.Context(), parts[5])
			if err != nil {
				return nil, err
			}
			defer f.Close()

			t := mime.TypeByExtension(path.Ext(parts[5]))
			if !(t == "text/css" || strstr(t, "image/") || strstr(t, "font/") || strstr(t, "text/css;")) {
				return nil, plumbing.Forbidden("disallowed type '%s'", t)
			}
			rv.ContentType = t

			rv.Contents, err = ioutil.ReadAll(f)
			if err != nil {
				return nil, err
			}

			return rv, nil
		}

		f, err := trns.ReadRootFile(r.Context(), "document.bin")
		if err != nil {
			return nil, plumbing.ErrNotFound
		}

		rv.ContentType = "text/html; charset=utf-8"
		rv.Header = make(http.Header)
		rv.Header.Set("Content-Security-Policy", "default-src 'none'; img-src data: 'self'; style-src 'unsafe-inline' 'self'; font-src 'self'")

		rv.Contents, err = ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}

		return rv, nil
	}), "page/asset")))

	listenAddr := "localhost:2690"
	log.Printf("Listening on %s", listenAddr)
	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	log.Fatal(srv.ListenAndServe())
}

func strstr(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

type okayStruc struct {
	OK      bool   `json:"ok"`
	Message string `json:"_"`
}

func okay(format string, a ...interface{}) (okayStruc, error) {
	return okayStruc{
		OK:      true,
		Message: fmt.Sprintf(format, a...),
	}, nil
}

func newTxid() string {
	buf := make([]byte, 32)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

type cachedTx struct {
	Mu   sync.Mutex
	Tx   storage.DocTransaction
	User *login.User
}
