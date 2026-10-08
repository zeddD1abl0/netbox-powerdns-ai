// Package contract checks the API's responses against its OpenAPI document,
// for tests (ADR-0033). A Checker validates each response, with
// libopenapi-validator, and records the operation and status code it
// covered, so that a test can fail if any operation, or any status code
// the document lists, is never checked.
package contract

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
)

// A Checker checks responses against an OpenAPI document. It's safe for
// concurrent use.
type Checker struct {
	base string
	v    validator.Validator
	ops  []operation

	mu   sync.Mutex
	seen map[string]bool
}

// An operation is a method on a path template, with the status codes its
// responses list.
type operation struct {
	method string
	path   []string // the template's segments, such as {group}
	codes  []string
}

func (o operation) String() string {
	return o.method + " /" + strings.Join(o.path, "/")
}

// New returns a Checker for the OpenAPI document spec, whose paths start at
// base, its server's path, such as /api.
func New(spec []byte, base string) (*Checker, error) {
	doc, err := libopenapi.NewDocument(spec)
	if err != nil {
		return nil, err
	}
	v, errs := validator.NewValidator(doc)
	if len(errs) > 0 {
		return nil, fmt.Errorf("the OpenAPI document: %v", errs)
	}
	model, err := doc.BuildV3Model()
	if err != nil {
		return nil, err
	}
	c := &Checker{base: base, v: v, seen: map[string]bool{}}
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		for method, op := range item.GetOperations().FromOldest() {
			o := operation{method: strings.ToUpper(method), path: strings.Split(strings.Trim(path, "/"), "/")}
			for code := range op.Responses.Codes.KeysFromOldest() {
				o.codes = append(o.codes, code)
			}
			c.ops = append(c.ops, o)
		}
	}
	return c, nil
}

// Check validates resp, the response to req, against the document, and
// fails t if it doesn't conform, or if no operation in the document
// matches req. It reads resp's body, and leaves it readable again.
func (c *Checker) Check(t testing.TB, req *http.Request, resp *http.Response) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.Request = req
	op, ok := c.match(req)
	if !ok {
		t.Errorf("%s %s: no operation in the OpenAPI document", req.Method, req.URL.Path)
		return
	}
	c.mu.Lock()
	c.seen[op.String()+" "+strconv.Itoa(resp.StatusCode)] = true
	_, errs := c.v.ValidateHttpResponse(req, resp)
	c.mu.Unlock()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	for _, e := range errs {
		var details []string
		for _, f := range e.SchemaValidationErrors {
			details = append(details, fmt.Sprintf("%s at %s", f.Reason, f.FieldPath))
		}
		t.Errorf("%s %s: %d response doesn't conform: %s: %s %v\n%s",
			req.Method, req.URL.Path, resp.StatusCode, e.Message, e.Reason, details, body)
	}
}

// match finds the operation for req: the one whose template matches its
// path with the most literal segments.
func (c *Checker) match(req *http.Request) (operation, bool) {
	rest, ok := strings.CutPrefix(req.URL.Path, c.base+"/")
	if !ok {
		return operation{}, false
	}
	segs := strings.Split(rest, "/")
	method := req.Method
	if method == http.MethodHead {
		method = http.MethodGet
	}
	best, bestLiterals := operation{}, -1
	for _, o := range c.ops {
		if o.method != method || len(o.path) != len(segs) {
			continue
		}
		literals, fits := 0, true
		for i, p := range o.path {
			switch {
			case strings.HasPrefix(p, "{"):
				fits = fits && segs[i] != ""
			case p == segs[i]:
				literals++
			default:
				fits = false
			}
		}
		if fits && literals > bestLiterals {
			best, bestLiterals = o, literals
		}
	}
	return best, bestLiterals >= 0
}

// Missing returns each operation and listed status code that no Check has
// covered, such as "GET /status 200".
func (c *Checker) Missing() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, o := range c.ops {
		for _, code := range o.codes {
			if k := o.String() + " " + code; !c.seen[k] {
				out = append(out, k)
			}
		}
	}
	slices.Sort(out)
	return out
}
