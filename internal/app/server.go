// Package app implements Sciurus's command line and loopback-only editor.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mhemmings/sciurus/examples"
	"github.com/mhemmings/sciurus/internal/model"
	"github.com/mhemmings/sciurus/web"
)

// documentResponse includes the revision required for the next optimistic save.
// An empty path indicates an in-memory document that the browser must download
// or save using its own file handle if it should survive stopping the server.
type documentResponse struct {
	Document *model.Document `json:"document"`
	Path     string          `json:"path"`
	Revision string          `json:"revision"`
}

// editor serializes file/session edits so concurrent browser tabs cannot silently
// overwrite one another. The only writable disk path is selected at CLI startup.
type editor struct {
	mu sync.Mutex
	documentResponse
	port string
}

// newHandler loads the CLI-selected model and wraps every route in the same
// loopback Host and browser Origin checks, including the embedded static app.
func newHandler(modelPath, address string) (http.Handler, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	e := &editor{documentResponse: documentResponse{Document: model.New("my_schema")}, port: port}
	if modelPath != "" {
		e.Path, err = localPath(modelPath)
		if err != nil {
			return nil, err
		}
		data, _, readErr := readFile(e.Path)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, readErr
		}
		if readErr == nil {
			e.Document, err = model.Decode(data)
			if err != nil {
				return nil, fmt.Errorf("read model: %w", err)
			}
			e.Revision = revision(data)
		}
	}
	assets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/document", e.getDocument)
	mux.HandleFunc("PUT /api/document", e.putDocument)
	mux.HandleFunc("POST /api/validate", e.validate)
	mux.HandleFunc("POST /api/generate", e.generate)
	mux.HandleFunc("POST /api/import", e.importSQL)
	mux.HandleFunc("GET /api/examples", e.listExamples)
	mux.HandleFunc("GET /api/examples/{file}", e.getExample)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, errors.New("API endpoint not found"))
	})
	static := http.FileServer(http.FS(assets))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		static.ServeHTTP(w, r)
	})
	return e.protect(mux), nil
}

// protect prevents DNS rebinding and cross-origin browser requests from reading
// or changing local files. Native CLI clients may omit Origin; browsers must
// have an exact same-origin Origin whenever they provide one.
func (e *editor) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		host, port, err := net.SplitHostPort(r.Host)
		ip := net.ParseIP(host)
		localHost := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
		if err != nil || port != e.port || !localHost {
			writeError(w, http.StatusForbidden, errors.New("only the editor's local host is allowed"))
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			u, parseErr := url.Parse(origin)
			if parseErr != nil || u.Scheme != "http" || !strings.EqualFold(u.Host, r.Host) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
				writeError(w, http.StatusForbidden, errors.New("cross-origin access to the local editor is not allowed"))
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeError(w, http.StatusForbidden, errors.New("cross-site access to the local editor is not allowed"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			mediaType, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if parseErr != nil || mediaType != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, errors.New("send application/json"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// getDocument reloads valid external edits so reopening recovers cleanly from a
// save conflict without requiring the user to restart the CLI process.
func (e *editor) getDocument(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Path != "" {
		data, _, err := readFile(e.Path)
		if err != nil && !(errors.Is(err, os.ErrNotExist) && e.Revision == "") {
			writeError(w, http.StatusConflict, err)
			return
		}
		if err == nil {
			document, decodeErr := model.Decode(data)
			if decodeErr != nil {
				writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("model on disk is invalid: %w", decodeErr))
				return
			}
			e.Document = document
			e.Revision = revision(data)
		}
	}
	writeJSON(w, http.StatusOK, e.documentResponse)
}

// putDocument replaces the entire model only after strict decoding, validation,
// and matching both the browser's revision and the current on-disk revision.
func (e *editor) putDocument(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Document json.RawMessage `json:"document"`
		Revision *string         `json:"revision"`
	}
	err := readJSON(w, r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if request.Revision == nil {
		writeError(w, http.StatusBadRequest, errors.New("revision is required"))
		return
	}
	document, err := model.Decode(request.Document)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	data, err := model.Encode(document)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if *request.Revision != e.Revision {
		writeError(w, http.StatusConflict, errors.New("another tab saved this model; reopen it before saving"))
		return
	}
	nextRevision := revision(data)
	if e.Path != "" {
		nextRevision, err = writeFile(e.Path, data, request.Revision)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errConflict) {
				status = http.StatusConflict
			}
			writeError(w, status, err)
			return
		}
	}
	e.Document = document
	e.Revision = nextRevision
	writeJSON(w, http.StatusOK, e.documentResponse)
}

// validate accepts a full document so validation never depends on server state.
func (e *editor) validate(w http.ResponseWriter, r *http.Request) {
	_, err := readModel(w, r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

// generate returns MySQL DDL without ever connecting to a database or writing
// disk paths supplied by a browser. The UI chooses how to download the result.
func (e *editor) generate(w http.ResponseWriter, r *http.Request) {
	document, err := readModel(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sql, err := model.Generate(document, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sql": string(sql)})
}

// importSQL converts SQL text into an editable document; it never executes SQL.
func (e *editor) importSQL(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SQL  string `json:"sql"`
		Name string `json:"name"`
	}
	err := readJSON(w, r, &request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(request.SQL) == "" {
		writeError(w, http.StatusBadRequest, errors.New("sql is required"))
		return
	}
	if request.Name == "" {
		request.Name = "imported_schema"
	}
	document, err := model.ImportSQL(request.SQL, request.Name)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, document)
}

// listExamples returns stable embedded filenames, with no access to disk paths.
func (e *editor) listExamples(w http.ResponseWriter, r *http.Request) {
	entries, err := examples.Models.ReadDir(".")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := []map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			data, readErr := examples.Models.ReadFile(entry.Name())
			if readErr != nil {
				writeError(w, http.StatusInternalServerError, readErr)
				return
			}
			var metadata struct {
				Name string `json:"name"`
			}
			decodeErr := json.Unmarshal(data, &metadata)
			if decodeErr != nil {
				writeError(w, http.StatusInternalServerError, decodeErr)
				return
			}
			items = append(items, map[string]string{"name": metadata.Name, "file": entry.Name()})
		}
	}
	writeJSON(w, http.StatusOK, items)
}

// getExample only reads a direct filename from the executable's example models.
func (e *editor) getExample(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if name != path.Base(name) || !strings.HasSuffix(name, ".json") {
		writeError(w, http.StatusNotFound, errors.New("example not found"))
		return
	}
	data, err := examples.Models.ReadFile(name)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("example not found"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// readModel gives browser files and the source editor exactly the same strict
// decoding contract as CLI files. Decode must see the original bytes before a
// permissive JSON decoder could normalize case aliases or malformed Unicode.
func readModel(w http.ResponseWriter, r *http.Request) (*model.Document, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read model JSON (maximum 16 MiB): %w", err)
	}
	return model.Decode(data)
}

// readJSON rejects invalid UTF-8, unknown fields, duplicate keys, trailing JSON,
// and excessive nesting. Duplicate keys are unsafe for AI-authored documents:
// different parsers could otherwise disagree about the intended value.
func readJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read JSON (maximum 16 MiB): %w", err)
	}
	if !utf8.Valid(data) {
		return errors.New("JSON must contain valid UTF-8")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	err = uniqueJSON(tokens, 0)
	if err != nil {
		return err
	}
	_, err = tokens.Token()
	if !errors.Is(err, io.EOF) {
		return errors.New("body must contain exactly one JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(target)
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("body must be a JSON object")
	}
	return nil
}

// uniqueJSON walks one JSON value without retaining it, checking names at each
// object boundary. Its explicit depth limit also bounds recursive stack growth.
func uniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return errors.New("JSON nesting exceeds 128 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return errors.New("unexpected JSON delimiter")
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			token, err = decoder.Token()
			if err != nil {
				return err
			}
			// encoding/json accepts case-insensitive field matches, so aliases
			// such as "name" and "Name" must also count as a duplicate.
			key := strings.ToLower(token.(string))
			if keys[key] {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			keys[key] = true
		}
		err = uniqueJSON(decoder, depth+1)
		if err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

// writeJSON sends a predictable content type for all API responses.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// writeError keeps browser-visible errors machine-readable and straightforward.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
