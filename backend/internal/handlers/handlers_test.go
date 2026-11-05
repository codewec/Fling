package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"filelit/internal/auth"
	"filelit/internal/config"
	filefs "filelit/internal/storage/fs"
	"filelit/internal/store"
)

type testEnv struct {
	srv *httptest.Server
	cli *http.Client
	cfg *config.Config
}

func newTestEnv(t *testing.T, mutate func(*config.Config)) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Addr:              ":0",
		DataDir:           dir,
		DatabaseURL:       filepath.Join(dir, "test.db"),
		FilesDir:          filepath.Join(dir, "files"),
		SecretKey:         "test-secret",
		TrustedNetworks:   nil, // untrusted by default in tests
		MaxUploadBytes:    1 << 20,
		PINMaxAttempts:    3,
		PINLockoutMinutes: 15,
	}
	if mutate != nil {
		mutate(cfg)
	}
	st, err := store.New(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fsStore, err := filefs.New(cfg.FilesDir)
	if err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(cfg)
	srv := &Server{Cfg: cfg, Store: st, Files: fsStore, AuthSvc: authSvc}

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(authSvc.Middleware(cfg))
		srv.Routes(r)
	})

	jar, _ := cookiejar.New(nil)
	env := &testEnv{
		srv: httptest.NewServer(r),
		cli: &http.Client{Jar: jar},
		cfg: cfg,
	}
	t.Cleanup(env.srv.Close)
	return env
}

func (e *testEnv) do(t *testing.T, method, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := e.cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *testEnv) json(t *testing.T, method, path string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		buf, _ := json.Marshal(payload)
		body = bytes.NewReader(buf)
	}
	return e.do(t, method, path, body, "application/json")
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

type bucketResp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HasPIN    bool   `json:"has_pin"`
	CreatedAt int64  `json:"created_at"`
}

type msgResp struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	Filename  string `json:"filename"`
	Mime      string `json:"mime"`
	Size      int64  `json:"size"`
	Sender    string `json:"sender"`
	CreatedAt int64  `json:"created_at"`
}

func createBucket(t *testing.T, e *testEnv, name, pin string) bucketResp {
	t.Helper()
	resp := e.json(t, "POST", "/api/buckets", map[string]string{"name": name, "pin": pin})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create bucket: %d", resp.StatusCode)
	}
	var b bucketResp
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBucketMessageFileFlow(t *testing.T) {
	e := newTestEnv(t, nil)

	b := createBucket(t, e, "test", "")
	if b.ID == "" || b.HasPIN {
		t.Fatalf("bad bucket: %+v", b)
	}

	// Text message roundtrip.
	resp := e.json(t, "POST", "/api/buckets/"+b.ID+"/messages", map[string]string{"content": "hello", "sender": "alice"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("post message: %d", resp.StatusCode)
	}
	msg := decode[map[string]any](t, resp)
	if msg["kind"] != "text" || msg["content"] != "hello" || msg["sender"] != "alice" {
		t.Fatalf("bad message: %v", msg)
	}

	resp = e.do(t, "GET", "/api/buckets/"+b.ID+"/messages", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list messages: %d", resp.StatusCode)
	}
	list := decode[map[string]any](t, resp)
	if msgs := list["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("messages: %v", list)
	}

	// File upload roundtrip.
	payload := bytes.Repeat([]byte("0123456789"), 5000) // 50 KB
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("sender", "bob"); err != nil {
		t.Fatal(err)
	}
	fw, err := mw.CreateFormFile("file", "data.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	resp = e.do(t, "POST", "/api/buckets/"+b.ID+"/files", &buf, mw.FormDataContentType())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d", resp.StatusCode)
	}
	fmsg := decode[msgResp](t, resp)
	if fmsg.Kind != "file" || fmsg.Filename != "data.txt" || fmsg.Size != int64(len(payload)) || fmsg.Sender != "bob" {
		t.Fatalf("bad file message: %+v", fmsg)
	}
	if !strings.HasPrefix(fmsg.Mime, "text/") {
		t.Fatalf("mime = %q", fmsg.Mime)
	}

	resp = e.do(t, "GET", "/api/files/"+fmsg.ID, nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download: %d", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("downloaded %d bytes, want %d", len(got), len(payload))
	}

	// Incremental fetch returns only newer messages.
	resp = e.do(t, "GET", "/api/buckets/"+b.ID+"/messages?after_id="+msg["id"].(string), nil, "")
	inc := decode[map[string]any](t, resp)
	if msgs := inc["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("incremental: %v", inc)
	}
}

func TestPINProtection(t *testing.T) {
	e := newTestEnv(t, nil)

	if resp := e.json(t, "POST", "/api/buckets", map[string]string{"name": "x", "pin": "123"}); resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("short pin accepted: %d", resp.StatusCode)
	}

	b := createBucket(t, e, "secret", "4242")
	if !b.HasPIN {
		t.Fatal("expected HasPIN")
	}

	// Another user (fresh cookie jar) must not inherit the creator's unlock.
	jar, _ := cookiejar.New(nil)
	e.cli = &http.Client{Jar: jar}

	resp := e.do(t, "GET", "/api/buckets/"+b.ID+"/messages", nil, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("pinned bucket readable without unlock: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "0000"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong pin: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "4242"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("correct pin: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/buckets/"+b.ID+"/messages", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("after unlock: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "POST", "/api/buckets/"+b.ID+"/lock", nil, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("lock: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/buckets/"+b.ID+"/messages", nil, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("re-lock did not restore pin requirement: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPINRateLimitByFingerprint(t *testing.T) {
	e := newTestEnv(t, nil)
	b := createBucket(t, e, "rl", "9999")

	// Two wrong attempts → 401.
	for i := 0; i < 2; i++ {
		resp := e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "0000"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}
	// Third wrong attempt hits PINMaxAttempts=3 → 429 lockout.
	resp := e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "0000"})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.StatusCode)
	}
	rl := decode[map[string]any](t, resp)
	if rl["retry_after"] == nil {
		t.Fatalf("missing retry_after: %v", rl)
	}
	// Even the correct pin is rejected while locked.
	resp = e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "9999"})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("correct pin during lockout: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// A fresh fingerprint (fresh cookie jar) is not affected.
	jar, _ := cookiejar.New(nil)
	e.cli = &http.Client{Jar: jar}
	resp = e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "9999"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("other fingerprint blocked: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPINLockoutExpiry(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) { c.PINLockoutMinutes = 0 })
	// lockout duration 0 → locked_until expires immediately.
	b := createBucket(t, e, "rl0", "1111")
	var last *http.Response
	for i := 0; i < 3; i++ {
		last = e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "bad!"})
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", last.StatusCode)
	}
	last.Body.Close()
	resp := e.json(t, "POST", "/api/buckets/"+b.ID+"/unlock", map[string]string{"pin": "1111"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("after zero-length lockout: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSharedPasswordAuth(t *testing.T) {
	e := newTestEnv(t, func(c *config.Config) {
		c.FileLitPassword = "s3cret"
		c.TrustedHosts = []string{"lan.internal"}
	})

	resp := e.do(t, "GET", "/api/buckets", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request allowed: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.json(t, "POST", "/api/auth/login", map[string]string{"password": "wrong"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.json(t, "POST", "/api/auth/login", map[string]string{"password": "s3cret"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/buckets", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated request: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Trusted Host header skips auth even without a session.
	jar, _ := cookiejar.New(nil)
	e.cli = &http.Client{Jar: jar}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/buckets", nil)
	req.Host = "lan.internal"
	resp, err := e.cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("trusted host blocked: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDeleteBucketRemovesFiles(t *testing.T) {
	e := newTestEnv(t, nil)
	b := createBucket(t, e, "tmp", "")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "x.bin")
	fw.Write([]byte("payload"))
	mw.Close()
	resp := e.do(t, "POST", "/api/buckets/"+b.ID+"/files", &buf, mw.FormDataContentType())
	fmsg := decode[msgResp](t, resp)

	resp = e.do(t, "DELETE", "/api/buckets/"+b.ID, nil, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/files/"+fmsg.ID, nil, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("file of deleted bucket still served: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUploadSizeLimit(t *testing.T) {
	e := newTestEnv(t, nil)
	b := createBucket(t, e, "lim", "")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "big.bin")
	fw.Write(make([]byte, 2<<20)) // 2 MB, limit is 1 MB
	mw.Close()
	resp := e.do(t, "POST", "/api/buckets/"+b.ID+"/files", &buf, mw.FormDataContentType())
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBucketNotFound(t *testing.T) {
	e := newTestEnv(t, nil)
	resp := e.do(t, "GET", "/api/buckets/nope1234/messages", nil, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBucketPinning(t *testing.T) {
	e := newTestEnv(t, nil)
	first := createBucket(t, e, "first", "")
	createBucket(t, e, "second", "")

	resp := e.json(t, "POST", "/api/buckets/"+first.ID+"/pin", map[string]bool{"pinned": true})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pin: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/buckets", nil, "")
	list := decode[map[string]any](t, resp)
	bs := list["buckets"].([]any)
	if len(bs) != 2 {
		t.Fatalf("len = %d", len(bs))
	}
	if bs[0].(map[string]any)["id"] != first.ID || bs[0].(map[string]any)["pinned"] != true {
		t.Fatalf("pinned bucket not first: %v", bs[0])
	}

	resp = e.json(t, "POST", "/api/buckets/"+first.ID+"/pin", map[string]bool{"pinned": false})
	resp.Body.Close()
	resp = e.do(t, "GET", "/api/buckets", nil, "")
	list = decode[map[string]any](t, resp)
	bs = list["buckets"].([]any)
	if bs[0].(map[string]any)["id"] != first.ID && bs[0].(map[string]any)["pinned"] != false {
		// order returns to created_at DESC: second (newer) first
		if bs[0].(map[string]any)["id"] == first.ID {
			t.Fatalf("unpin did not restore order: %v", bs)
		}
	}
}

func TestMessagePinAndDelete(t *testing.T) {
	e := newTestEnv(t, nil)
	b := createBucket(t, e, "pin-delete", "")

	resp := e.json(t, "POST", "/api/buckets/"+b.ID+"/messages", map[string]string{"content": "keep me"})
	keep := decode[msgResp](t, resp)
	resp = e.json(t, "POST", "/api/buckets/"+b.ID+"/messages", map[string]string{"content": "pin me"})
	pinMe := decode[msgResp](t, resp)

	// Pin.
	resp = e.json(t, "POST", "/api/messages/"+pinMe.ID+"/pin", map[string]bool{"pinned": true})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pin message: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/buckets/"+b.ID+"/messages", nil, "")
	list := decode[map[string]any](t, resp)
	pinned := list["pinned_messages"].([]any)
	if len(pinned) != 1 || pinned[0].(map[string]any)["id"] != pinMe.ID {
		t.Fatalf("pinned_messages: %v", pinned)
	}

	// Unpin via pinned=false.
	resp = e.json(t, "POST", "/api/messages/"+pinMe.ID+"/pin", map[string]bool{"pinned": false})
	resp.Body.Close()
	resp = e.do(t, "GET", "/api/buckets/"+b.ID+"/messages", nil, "")
	list = decode[map[string]any](t, resp)
	if len(list["pinned_messages"].([]any)) != 0 {
		t.Fatalf("unpin failed: %v", list["pinned_messages"])
	}

	// Delete text message.
	resp = e.do(t, "DELETE", "/api/messages/"+keep.ID, nil, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = e.do(t, "DELETE", "/api/messages/"+keep.ID, nil, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("double delete: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDeleteFileMessageRemovesContent(t *testing.T) {
	e := newTestEnv(t, nil)
	b := createBucket(t, e, "files", "")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "doc.txt")
	fw.Write([]byte("content"))
	mw.Close()
	resp := e.do(t, "POST", "/api/buckets/"+b.ID+"/files", &buf, mw.FormDataContentType())
	fmsg := decode[msgResp](t, resp)

	resp = e.do(t, "DELETE", "/api/messages/"+fmsg.ID, nil, "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, "GET", "/api/files/"+fmsg.ID, nil, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("content of deleted message served: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHeaderAuthPrivateBuckets(t *testing.T) {
	env := newTestEnv(t, func(cfg *config.Config) {
		cfg.AuthUserHeader = "Remote-User"
	})

	// Anonymous cannot create a private bucket.
	resp := env.json(t, "POST", "/api/buckets", map[string]any{"name": "secret", "private": true})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("anonymous private create: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Alice creates a private bucket, bob creates a public one.
	doAs := func(user, method, path string, payload any) *http.Response {
		var body io.Reader
		if payload != nil {
			buf, _ := json.Marshal(payload)
			body = bytes.NewReader(buf)
		}
		req, err := http.NewRequest(method, env.srv.URL+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if user != "" {
			req.Header.Set("Remote-User", user)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp = doAs("alice", "POST", "/api/buckets", map[string]any{"name": "secret", "private": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("alice private create: %d", resp.StatusCode)
	}
	created := decode[bucketResp](t, resp)
	resp = doAs("bob", "POST", "/api/buckets", map[string]any{"name": "public"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bob public create: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List filtering: alice sees 2, bob 1, anonymous 1.
	for user, want := range map[string]int{"alice": 2, "bob": 1, "": 1} {
		resp := doAs(user, "GET", "/api/buckets", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list as %q: %d", user, resp.StatusCode)
		}
		list := decode[map[string]any](t, resp)
		if got := len(list["buckets"].([]any)); got != want {
			t.Fatalf("list as %q: %d buckets, want %d", user, got, want)
		}
	}

	// Access to the private bucket: alice ok, bob and anonymous 403.
	for user, want := range map[string]int{"alice": http.StatusOK, "bob": http.StatusForbidden, "": http.StatusForbidden} {
		resp := doAs(user, "GET", "/api/buckets/"+created.ID+"/messages", nil)
		if resp.StatusCode != want {
			t.Fatalf("messages as %q: %d, want %d", user, resp.StatusCode, want)
		}
		resp.Body.Close()
	}

	// /api/auth/me reports the header identity.
	resp = doAs("alice", "GET", "/api/auth/me", nil)
	me := decode[map[string]any](t, resp)
	if me["user"] != "alice" || me["user_auth"] != true {
		t.Fatalf("me: %+v", me)
	}
	resp = doAs("", "GET", "/api/auth/me", nil)
	me = decode[map[string]any](t, resp)
	if me["user"] != "" {
		t.Fatalf("anonymous me: %+v", me)
	}
}
