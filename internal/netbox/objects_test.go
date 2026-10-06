package netbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// TestRecordedFixture maps the responses recorded from each lab NetBox
// (TestRecord) into the normalized model, and checks the result against the
// fixture they were recorded from.
func TestRecordedFixture(t *testing.T) {
	f := lab.DescribeFixture("rec")
	for _, nb := range lab.NetBoxes {
		t.Run(nb.Name, func(t *testing.T) {
			var (
				status  Status
				zones   page[Zone]
				records page[Record]
			)
			for name, v := range map[string]any{"status.json": &status, "zones.json": &zones, "records.json": &records} {
				b, err := os.ReadFile(filepath.Join("testdata", nb.Name, name))
				if err == nil {
					err = json.Unmarshal(b, v)
				}
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
			}
			if err := status.Check(); err != nil || !strings.HasPrefix(status.NetBoxVersion, nb.Version+".") {
				t.Errorf("status %+v: %v", status, err)
			}
			if len(zones.Results) != 1 || records.Next != "" || len(records.Results) != records.Count {
				t.Fatalf("recorded %d zones and %d of %d records; record them again", len(zones.Results), len(records.Results), records.Count)
			}
			z, probs := normalize(zones.Results[0], records.Results)
			checkFixtureZone(t, f, z, probs)
		})
	}
}
