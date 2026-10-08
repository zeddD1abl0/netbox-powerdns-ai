package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// referencePage is the page that documents the status JSON.
const referencePage = "../../docs/reference/service-endpoints.md"

// fieldPaths returns the JSON paths of t's fields, such as
// schedule.last_refresh.outcome or groups[].name, and which are leaves: not
// an object or an array of objects.
func fieldPaths(t reflect.Type, prefix string, paths map[string]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		path := prefix + name
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		switch {
		case ft.Kind() == reflect.Struct && ft != reflect.TypeFor[time.Time]():
			paths[path] = false
			fieldPaths(ft, path+".", paths)
		case ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct:
			paths[path] = false
			fieldPaths(ft.Elem(), path+"[].", paths)
		default:
			paths[path] = true
		}
	}
}

// examplePaths returns the paths of every value in v, decoded JSON.
func examplePaths(v any, prefix string, paths map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			paths[prefix+k] = true
			examplePaths(x, prefix+k+".", paths)
		}
	case []any:
		for _, x := range v {
			examplePaths(x, strings.TrimSuffix(prefix, ".")+"[].", paths)
		}
	}
}

func TestStatusIsDocumented(t *testing.T) {
	page, err := os.ReadFile(referencePage)
	if err != nil {
		t.Fatal(err)
	}
	example := regexp.MustCompile("(?s)## `/status`.*?```json\n(.*?)```").FindSubmatch(page)
	if example == nil {
		t.Fatalf("%s has no JSON example under /status", referencePage)
	}
	// The example is a real status: every field it has is one.
	d := json.NewDecoder(bytes.NewReader(example[1]))
	d.DisallowUnknownFields()
	var st Status
	if err := d.Decode(&st); err != nil {
		t.Fatalf("the example isn't a status: %v", err)
	}
	var raw any
	if err := json.Unmarshal(example[1], &raw); err != nil {
		t.Fatal(err)
	}
	inExample := map[string]bool{}
	examplePaths(raw, "", inExample)

	fields := map[string]bool{}
	fieldPaths(reflect.TypeFor[Status](), "", fields)
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z_.\\[\\]]+)` \\|").FindAllSubmatch(page, -1) {
		documented[string(m[1])] = true
	}
	for path, leaf := range fields {
		if leaf && !documented[path] {
			t.Errorf("%s isn't in the field table of %s", path, referencePage)
		}
		if leaf && !inExample[path] {
			t.Errorf("%s isn't in the example of %s", path, referencePage)
		}
	}
	for path := range documented {
		if _, ok := fields[path]; !ok {
			t.Errorf("%s documents %s, which isn't a field", referencePage, path)
		}
	}
}

func TestStatus(t *testing.T) {
	refreshed := false
	s, _, _ := testService(t, Options{
		Version:   version.Info{Version: "v1.2.3", Commit: "0123abc"},
		NetBoxURL: "https://netbox.example.com",
		Groups:    []Group{{Name: "site-a", URL: "https://pdns-a.example.com"}, {Name: "site-b", URL: "https://pdns-b.example.com"}},
		OTLP:      OTLP{Endpoint: "https://otel.example.com:4318", Protocol: "http/protobuf"},
		Refresh: func(context.Context) (drift.Report, error) {
			refreshed = true
			return report(group("site-a", drift.StateDrift, drift.StateInSync), failedGroup("site-b")), nil
		},
	})
	jsonStatus := func() Status {
		t.Helper()
		code, body := get(t, s, http.MethodGet, "/status?json=1")
		var st Status
		if err := json.Unmarshal([]byte(body), &st); err != nil || code != http.StatusOK {
			t.Fatalf("status JSON: %d, %v:\n%s", code, err, body)
		}
		return st
	}

	// Before the first refresh.
	st := jsonStatus()
	if st.Ready || st.Schedule.LastRefresh != nil || st.NetBox.Up != nil || len(st.Groups) != 2 ||
		st.Groups[0].Status != statusUnknown || st.Version != "v1.2.3" || !st.Tracing.Exported {
		t.Errorf("status before a refresh: %+v", st)
	}
	if _, body := get(t, s, http.MethodGet, "/status"); !strings.Contains(body, "there's no drift report until the first refresh finishes") {
		t.Errorf("the page before a refresh:\n%s", body)
	}

	s.refresh(t.Context(), time.Now())
	if !refreshed {
		t.Fatal("no refresh")
	}
	st = jsonStatus()
	a, b := st.Groups[0], st.Groups[1]
	if !st.Ready || st.Schedule.LastRefresh == nil || st.Schedule.LastRefresh.Outcome != "incomplete" || st.NetBox.Up == nil || !*st.NetBox.Up ||
		st.Schedule.Refreshes != (Refreshes{Incomplete: 1}) {
		t.Errorf("status after a refresh: %+v", st)
	}
	if a.Status != drift.StatusOK || a.LastSuccess == nil || a.Counts.Drift != 1 ||
		!slices.Equal(a.DriftedZones, []DriftedZone{{Zone: "a.example.", State: drift.StateDrift, Changes: 1}}) {
		t.Errorf("site-a: %+v", a)
	}
	if b.Status != drift.StatusFailed || b.LastSuccess != nil || b.Error != "PowerDNS isn't reachable" || b.DriftedZones == nil {
		t.Errorf("site-b: %+v", b)
	}

	code, page := get(t, s, http.MethodGet, "/status")
	for _, want := range []string{
		"nbpdns v1.2.3, up ",
		"Ready: yes. Live: yes.",
		", incomplete, in ",
		"finished:      0 complete, 1 incomplete, 0 failed",
		"NetBox at https://netbox.example.com: read by the last refresh",
		"site-a  ok  ",
		"site-b  failed  none",
		"Drifted zones, as of each group's last successful read:",
		"site-a  a.example.  drift  1",
		"Groups whose primary couldn't be read:",
		"site-b  PowerDNS isn't reachable",
		"Spans are exported over http/protobuf to https://otel.example.com:4318.",
	} {
		if code != http.StatusOK || !strings.Contains(page, want) {
			t.Errorf("no %q in the page:\n%s", want, page)
		}
	}
}
