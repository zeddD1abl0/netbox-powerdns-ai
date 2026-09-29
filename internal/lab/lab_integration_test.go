//go:build integration

package lab

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestLabNetBoxes checks that each lab NetBox answers its admin token, runs
// its NetBox release series, and has the DNS plugin in the matching series.
func TestLabNetBoxes(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	for _, nb := range NetBoxes {
		t.Run(nb.Name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, nb.URL()+"/api/status/", http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+AdminToken)
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s isn't answering (is the lab up? make lab-up): %v", nb.URL(), err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /api/status/: %s", resp.Status)
			}
			var status struct {
				NetBoxVersion string            `json:"netbox-version"`
				Plugins       map[string]string `json:"plugins"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(status.NetBoxVersion, nb.Version+".") {
				t.Errorf("NetBox %s, want %s.x", status.NetBoxVersion, nb.Version)
			}
			if p := status.Plugins["netbox_dns"]; !strings.HasPrefix(p, nb.PluginVersion+".") {
				t.Errorf("netbox_dns plugin %q, want %s.x (plugins: %v)", p, nb.PluginVersion, status.Plugins)
			}
		})
	}
}
