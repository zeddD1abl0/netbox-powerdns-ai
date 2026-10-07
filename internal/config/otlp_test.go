package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		in      string
		want    map[string]string
		wantErr string
	}{
		{"", map[string]string{}, ""},
		{"a=b", map[string]string{"a": "b"}, ""},
		{" Authorization = Bearer%20s3cret , X-Tenant=a%2Cb", map[string]string{"Authorization": "Bearer s3cret", "X-Tenant": "a,b"}, ""},
		{"a=", map[string]string{"a": ""}, ""},
		{"s3cret", nil, "pair 1 isn't a header name"},
		{"a=b,=s3cret", nil, "pair 2 isn't a header name"},
		{"bad name=s3cret", nil, "pair 1 isn't a header name"},
		{"a=s3cret%zz", nil, "pair 1's value isn't percent-encoded"},
	}
	for _, tt := range tests {
		got, err := ParseHeaders(tt.in)
		switch {
		case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
			t.Errorf("ParseHeaders(%q): %v, want an error with %q", tt.in, err, tt.wantErr)
		case err != nil && strings.Contains(err.Error(), "s3cret"):
			t.Errorf("ParseHeaders(%q): the error gives the secret: %v", tt.in, err)
		case tt.wantErr == "" && (err != nil || !reflect.DeepEqual(got, tt.want)):
			t.Errorf("ParseHeaders(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
}

func TestHeadersAreChecked(t *testing.T) {
	_, _, err := load(t, "", map[string]string{"NBPDNS_OTLP_HEADERS": "s3cret"})
	if err == nil || !strings.Contains(err.Error(), "otlp.headers") || !strings.Contains(err.Error(), "pair 1 isn't a header name") ||
		strings.Contains(err.Error(), "s3cret") {
		t.Errorf("Load: %v, want an error naming the pair, not its value", err)
	}
}
