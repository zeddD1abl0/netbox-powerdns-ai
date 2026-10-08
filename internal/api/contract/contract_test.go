package contract

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

const spec = `openapi: 3.1.0
info:
  title: test
  version: 1.0.0
servers:
  - url: /api
paths:
  /things/{id}:
    get:
      operationId: getThing
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: A thing.
          content:
            application/json:
              schema:
                type: object
                required: [id, size]
                properties:
                  id:
                    type: string
                  size:
                    type: [integer, "null"]
        "404":
          description: No such thing.
          content:
            application/problem+json:
              schema:
                type: object
                required: [status]
                properties:
                  status:
                    type: integer
  /things/all:
    get:
      operationId: listThings
      responses:
        "200":
          description: Every thing.
          content:
            application/json:
              schema:
                type: array
`

// recorder is a testing.TB that records errors instead of failing.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func response(req *http.Request, status int, ctype, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {ctype}},
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}
}

func TestCheck(t *testing.T) {
	c, err := New([]byte(spec), "/api")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path  string
		status      int
		ctype, body string
		wantProblem string
	}{
		{"conforms", "/api/things/a", 200, "application/json", `{"id":"a","size":null}`, ""},
		{"missing field", "/api/things/a", 200, "application/json", `{"id":"a"}`, "doesn't conform"},
		{"wrong type", "/api/things/a", 200, "application/json", `{"id":"a","size":"big"}`, "doesn't conform"},
		{"undocumented content type", "/api/things/a", 200, "text/plain", `hello`, "doesn't conform"},
		{"literal segment wins", "/api/things/all", 200, "application/json", `[]`, ""},
		{"no such operation", "/api/widgets/a", 200, "application/json", `{}`, "no operation"},
		{"outside the base", "/things/a", 200, "application/json", `{"id":"a","size":1}`, "no operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{TB: t}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)
			resp := response(req, tt.status, tt.ctype, tt.body)
			defer resp.Body.Close()
			c.Check(r, req, resp)
			got := strings.Join(r.errs, "\n")
			if tt.wantProblem == "" && got != "" || tt.wantProblem != "" && !strings.Contains(got, tt.wantProblem) {
				t.Errorf("errors %q, want %q", got, tt.wantProblem)
			}
			if b, _ := io.ReadAll(resp.Body); string(b) != tt.body {
				t.Errorf("the body after Check is %q", b)
			}
		})
	}
	// Only GET /things/{id} 404 was never checked.
	if got, want := c.Missing(), []string{"GET /things/{id} 404"}; !slices.Equal(got, want) {
		t.Errorf("Missing() = %v, want %v", got, want)
	}
}
