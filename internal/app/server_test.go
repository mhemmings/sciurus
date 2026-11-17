package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhemmings/sciurus/internal/model"
)

// request exercises the public HTTP contract with the same headers a local
// browser sends, keeping file and origin checks enabled in every endpoint test.
func request(t *testing.T, handler http.Handler, method, route string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:7480"+route, bytes.NewReader(encoded))
	req.Header.Set("Origin", "http://127.0.0.1:7480")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

// opened reads through the API so tests use real revisions instead of coupling
// optimistic-save expectations to the implementation of the hash function.
func opened(t *testing.T, handler http.Handler) documentResponse {
	t.Helper()
	response := request(t, handler, http.MethodGet, "/api/document", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET document: %d %s", response.Code, response.Body)
	}
	var document documentResponse
	err := json.Unmarshal(response.Body.Bytes(), &document)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

// TestSaveRejectsExternalAndConcurrentEdits verifies that both another tab and
// an external editor can change the baseline without losing their newer work.
func TestSaveRejectsExternalAndConcurrentEdits(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "model.er.json")
	initial, err := model.Encode(model.New("initial"))
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(file, initial, 0600)
	if err != nil {
		t.Fatal(err)
	}
	initialInfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(file, "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	first := opened(t, handler)
	first.Document.Name = "saved"
	payload := map[string]any{"document": first.Document, "revision": first.Revision}
	saved := request(t, handler, http.MethodPut, "/api/document", payload)
	if saved.Code != http.StatusOK {
		t.Fatalf("save: %d %s", saved.Code, saved.Body)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != initialInfo.Mode().Perm() {
		t.Fatalf("save changed file permissions: %v %v", info, err)
	}
	// The old tab must not overwrite the save, even with otherwise valid JSON.
	stale := request(t, handler, http.MethodPut, "/api/document", payload)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale tab: %d %s", stale.Code, stale.Body)
	}
	latest := opened(t, handler)
	if latest.Document.Name != "saved" || latest.Revision == first.Revision {
		t.Fatalf("save did not update state: %+v", latest)
	}
	// Simulate an AI or text editor writing a different valid document on disk.
	external, err := model.Encode(model.New("external_edit"))
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(file, external, 0600)
	if err != nil {
		t.Fatal(err)
	}
	conflict := request(t, handler, http.MethodPut, "/api/document", map[string]any{"document": latest.Document, "revision": latest.Revision})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("external edit: %d %s", conflict.Code, conflict.Body)
	}
	actual, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(actual, external) {
		t.Fatalf("external edit was overwritten: %s (%v)", actual, err)
	}
	// Reopening explicitly adopts the external revision and makes editing safe.
	reopened := opened(t, handler)
	if reopened.Document.Name != "external_edit" || reopened.Revision == latest.Revision {
		t.Fatalf("reopen did not adopt external edit: %+v", reopened)
	}
	reopened.Document.Version = 999
	rejected := request(t, handler, http.MethodPut, "/api/document", map[string]any{"document": reopened.Document, "revision": reopened.Revision})
	if rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid save: %d %s", rejected.Code, rejected.Body)
	}
	actual, err = os.ReadFile(file)
	if err != nil || !bytes.Equal(actual, external) {
		t.Fatalf("invalid save changed disk contents: %s (%v)", actual, err)
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked after save: %v %v", entries, err)
	}
}

// TestNewFileAndSessionSave checks both persistence modes: CLI-selected files
// are created on first save, whereas an unbound session exposes no writable path.
func TestNewFileAndSessionSave(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"", filepath.Join(t.TempDir(), "new.er.json")} {
		handler, err := newHandler(file, "127.0.0.1:7480")
		if err != nil {
			t.Fatal(err)
		}
		initial := opened(t, handler)
		if initial.Revision != "" {
			t.Fatalf("new document already has a revision: %+v", initial)
		}
		initial.Document.Name = "new_schema"
		response := request(t, handler, http.MethodPut, "/api/document", map[string]any{"document": initial.Document, "revision": ""})
		if response.Code != http.StatusOK {
			t.Fatalf("new save: %d %s", response.Code, response.Body)
		}
		reopened := opened(t, handler)
		if reopened.Document.Name != "new_schema" || reopened.Revision == "" {
			t.Fatalf("new save was not retained: %+v", reopened)
		}
		if file == "" {
			if reopened.Path != "" {
				t.Fatal("an unbound browser acquired a filesystem path")
			}
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil || !bytes.Contains(data, []byte("new_schema")) {
			t.Fatalf("new file not written: %s %v", data, err)
		}
	}
}

// TestSymlinkCannotRedirectSave protects both startup selection and replacing a
// bound file with a symlink after the user has already opened it in the editor.
func TestSymlinkCannotRedirectSave(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	target := filepath.Join(directory, "target.er.json")
	link := filepath.Join(directory, "link.er.json")
	data, err := model.Encode(model.New("untouched"))
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(target, data, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(target, link)
	if err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err = newHandler(link, "127.0.0.1:7480")
	if err == nil {
		t.Fatal("a symlink was accepted as a writable model")
	}
	bound := filepath.Join(directory, "bound.er.json")
	err = os.WriteFile(bound, data, 0600)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(bound, "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	initial := opened(t, handler)
	err = os.Remove(bound)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(target, bound)
	if err != nil {
		t.Fatal(err)
	}
	initial.Document.Name = "must_not_be_written"
	response := request(t, handler, http.MethodPut, "/api/document", map[string]any{"document": initial.Document, "revision": initial.Revision})
	if response.Code < 400 {
		t.Fatalf("symlink replacement was accepted: %d", response.Code)
	}
	actual, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("symlink target changed: %s %v", actual, err)
	}
}

// TestLocalOriginBoundary checks rebinding, cross-site forms, opaque origins,
// and invalid ports rather than relying on CORS response headers alone.
func TestLocalOriginBoundary(t *testing.T) {
	t.Parallel()
	handler, err := newHandler("", "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		host   string
		origin string
		site   string
		status int
	}{
		{"local", "127.0.0.1:7480", "http://127.0.0.1:7480", "same-origin", http.StatusOK},
		{"localhost", "localhost:7480", "http://localhost:7480", "same-origin", http.StatusOK},
		{"cli", "127.0.0.1:7480", "", "", http.StatusOK},
		{"rebound domain", "attacker.example:7480", "http://attacker.example:7480", "same-origin", http.StatusForbidden},
		{"wrong port", "127.0.0.1:7481", "http://127.0.0.1:7481", "", http.StatusForbidden},
		{"cross origin", "127.0.0.1:7480", "https://attacker.example", "", http.StatusForbidden},
		{"opaque origin", "127.0.0.1:7480", "null", "", http.StatusForbidden},
		{"cross site without origin", "127.0.0.1:7480", "", "cross-site", http.StatusForbidden},
		{"origin credentials", "127.0.0.1:7480", "http://attacker@127.0.0.1:7480", "", http.StatusForbidden},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7480/api/document", nil)
			req.Host = test.host
			req.Header.Set("Origin", test.origin)
			req.Header.Set("Sec-Fetch-Site", test.site)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != test.status {
				t.Fatalf("got %d, want %d: %s", response.Code, test.status, response.Body)
			}
		})
	}
	// Browser form submissions cannot supply the required JSON content type.
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7480/api/import", strings.NewReader(`{"sql":"DROP TABLE users"}`))
	req.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("form-compatible request accepted: %d", response.Code)
	}
}

// TestMalformedSavePayloads confirms invalid envelopes do not change session
// state, including duplicate aliases accepted by encoding/json's default rules.
func TestMalformedSavePayloads(t *testing.T) {
	t.Parallel()
	handler, err := newHandler("", "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	document, err := model.Encode(model.New("valid"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`{"revision":"","document":` + string(document) + `,"path":"/tmp/arbitrary"}`,
		`{"revision":"","revision":"","document":` + string(document) + `}`,
		`{"revision":"","Revision":"","document":` + string(document) + `}`,
		`{"revision":"","document":` + string(document) + `} {}`,
		`{"document":` + string(document) + `}`,
		`{"revision":null,"document":` + string(document) + `}`,
		`{"revision":"","document":null}`,
		`null`,
		`[]`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:7480/api/document", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code < 400 {
			t.Fatalf("malformed save accepted: %s", body)
		}
	}
	state := opened(t, handler)
	if state.Document.Name != "my_schema" || state.Revision != "" {
		t.Fatalf("malformed request mutated the document: %+v", state)
	}
}

// TestRawModelAPIsRejectAmbiguousJSON uses raw request bytes because parsing
// first would erase the very corruption the file/source editor must detect.
func TestRawModelAPIsRejectAmbiguousJSON(t *testing.T) {
	t.Parallel()
	handler, err := newHandler("", "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	base := `{"version":1,"name":"Example","tables":[],"groups":[],"viewport":{"x":0,"y":0,"zoom":1}}`
	cases := map[string]string{
		"property case":           strings.Replace(base, `"name":`, `"Name":`, 1),
		"nested property case":    strings.Replace(base, `"zoom":`, `"Zoom":`, 1),
		"duplicate":               strings.Replace(base, `"name":"Example"`, `"name":"One","name":"Two"`, 1),
		"duplicate case alias":    strings.Replace(base, `"name":"Example"`, `"name":"One","Name":"Two"`, 1),
		"unpaired high surrogate": strings.Replace(base, `"Example"`, `"\ud800"`, 1),
		"unpaired low surrogate":  strings.Replace(base, `"Example"`, `"\udc00"`, 1),
		"invalid UTF8":            strings.Replace(base, "Example", "broken\xffname", 1),
	}
	for _, endpoint := range []string{"/api/validate", "/api/generate"} {
		for name, body := range cases {
			t.Run(endpoint+"/"+name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7480"+endpoint, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if response.Code < 400 {
					t.Fatalf("ambiguous model accepted: %s", body)
				}
				var failure map[string]string
				decodeErr := json.Unmarshal(response.Body.Bytes(), &failure)
				if decodeErr != nil || failure["error"] == "" {
					t.Fatalf("missing actionable JSON error: %s (%v)", response.Body, decodeErr)
				}
			})
		}
	}
}

// TestImportRejectsInvalidUTF8 ensures envelope decoding cannot silently replace
// corrupted SQL identifiers before the SQL parser ever sees their original text.
func TestImportRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()
	handler, err := newHandler("", "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	body := "{\"sql\":\"CREATE TABLE `broken\xffname` (id INT);\",\"name\":\"Example\"}"
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7480/api/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "UTF-8") {
		t.Fatalf("invalid SQL bytes were not rejected: %d %s", response.Code, response.Body)
	}
}

// TestEmbeddedAppAndSchemaEndpoints verifies a compiled binary can serve its
// entry point and produce SQL using the same API the browser editor consumes.
func TestEmbeddedAppAndSchemaEndpoints(t *testing.T) {
	t.Parallel()
	handler, err := newHandler("", "127.0.0.1:7480")
	if err != nil {
		t.Fatal(err)
	}
	root := request(t, handler, http.MethodGet, "/", nil)
	if root.Code != http.StatusOK || !strings.Contains(strings.ToLower(root.Body.String()), "<html") {
		t.Fatalf("embedded app not served: %d %s", root.Code, root.Body)
	}
	if root.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("embedded app has no content security policy")
	}
	imported := request(t, handler, http.MethodPost, "/api/import", map[string]string{"sql": "CREATE TABLE widgets (id BIGINT NOT NULL, PRIMARY KEY (id)) ENGINE=InnoDB;", "name": "shop"})
	if imported.Code != http.StatusOK {
		t.Fatalf("SQL import: %d %s", imported.Code, imported.Body)
	}
	var document model.Document
	err = json.Unmarshal(imported.Body.Bytes(), &document)
	if err != nil {
		t.Fatal(err)
	}
	validated := request(t, handler, http.MethodPost, "/api/validate", document)
	if validated.Code != http.StatusOK {
		t.Fatalf("validation: %d %s", validated.Code, validated.Body)
	}
	generated := request(t, handler, http.MethodPost, "/api/generate", document)
	if generated.Code != http.StatusOK {
		t.Fatalf("SQL generation: %d %s", generated.Code, generated.Body)
	}
	var response struct {
		SQL string `json:"sql"`
	}
	err = json.Unmarshal(generated.Body.Bytes(), &response)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(response.SQL, "-- Generated at ") || !strings.Contains(response.SQL, "CREATE TABLE `widgets`") {
		t.Fatalf("missing timestamp or schema: %s", response.SQL)
	}
}
