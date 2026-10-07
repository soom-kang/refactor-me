package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// referenceJSON pins content identity, not verified live provider compatibility.
// Tree hashes use the same path, length, executable-bit and content framing as readTree.
//
//go:embed reference.json
var referenceJSON []byte

type referenceManifest struct {
	Source            string            `json:"source"`
	Revision          string            `json:"revision"`
	HashAlgorithm     string            `json:"hashAlgorithm"`
	LiveCompatibility string            `json:"liveCompatibility"`
	Skills            map[string]string `json:"skills"`
}

// ReferenceCheck distinguishes a reference match from a valid custom catalog.
// It never grants provider compatibility or weakens Load/Verify validation.
func (c *Catalog) ReferenceCheck() (string, string) {
	if c == nil {
		return "FAIL", "catalog unavailable; reference comparison was not possible"
	}
	var ref referenceManifest
	if err := json.Unmarshal(referenceJSON, &ref); err != nil || len(ref.Skills) != len(Required) {
		return "WARN", "embedded skill reference unavailable; catalog compatibility is unverified"
	}
	actual := map[string]string{}
	for _, entry := range c.Entries {
		actual[entry.Name] = entry.SHA256
	}
	changed := []string{}
	for _, name := range Required {
		if ref.Skills[name] == "" || actual[name] != ref.Skills[name] {
			changed = append(changed, name)
		}
	}
	detail := fmt.Sprintf("%s@%s; live compatibility: %s", ref.Source, ref.Revision, ref.LiveCompatibility)
	if len(changed) != 0 {
		return "WARN", "unverified/custom catalog; differs from reference: " + strings.Join(changed, ", ") + "; " + detail
	}
	return "PASS", "reference content match; " + detail
}
