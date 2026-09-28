package fixture

import (
	"fmt"
	"go/format"
	"strings"
)

func writeGo(root string) error {
	files := map[string]string{
		"go.mod":     "module fixture\n\ngo 1.27\n",
		".gitignore": "dist/\n.env.local\n",
		"AGENTS.md":  "# Go refactoring fixture\n\nRun `go vet ./...`, `go test ./...`, and `go build ./...`.\nTestKnownBaseline intentionally fails with BASELINE_RED; preserve that baseline during differential validation.\nThe plugin-x string registration and v1 payload behavior are compatibility contracts.\n",
		"src/legacy_parser.go": `package fixture
import "strings"
func legacyTokenize(value string) []string { return strings.Fields(value) }
func legacyVersion() int { return 1 }
`,
		"src/plugin_x.go": `package fixture
import "strings"
func init() { plugins["plugin-x"] = strings.ToUpper }
`,
		"src/registry.go": `package fixture
var plugins = map[string]func(string) string{}
func ResolvePlugin(name, input string) (string,bool) { fn,ok:=plugins[name]; if !ok {return "",false};return fn(input),true }
`,
		"src/compat.go": `package fixture
import "encoding/json"
type Payload struct {UserName string; Version int}
func ReadPayload(raw []byte)(Payload,error){
 var data map[string]string
 if err:=json.Unmarshal(raw,&data);err!=nil{return Payload{},err}
 if name,ok:=data["user_name"];ok{return Payload{name,1},nil}
 return Payload{data["userName"],2},nil
}
`,
		"src/broken.go": `package fixture
// BaselineReady models a known defect deliberately left unresolved.
func BaselineReady() bool {return false}
`,
		"src/contract_test.go": `package fixture
import "testing"
func TestKnownBaseline(t *testing.T){if !BaselineReady(){t.Fatal("BASELINE_RED: intentional known failure")}}
func TestContract(t *testing.T){
 panel:=RenderPanel("active",nil)
 if panel.Status!="Active" || len(panel.Sections)!=24 || panel.Total!=0 {t.Fatalf("panel contract: %+v",panel)}
 if value,ok:=ResolvePlugin("plugin-x","ab");!ok || value!="AB" {t.Fatal("string plugin missing")}
 if _,ok:=ResolvePlugin("missing","");ok{t.Fatal("unknown plugin resolved")}
 for _,test:=range []struct{raw string;version int}{ {"{\"user_name\":\"kim\"}",1}, {"{\"userName\":\"lee\"}",2}} {
  payload,err:=ReadPayload([]byte(test.raw));if err!=nil || payload.Version!=test.version || payload.UserName==""{t.Fatalf("payload: %+v %v",payload,err)}
 }
 for _,label:=range []func(string)string{StatusLabelA,StatusLabelB,StatusLabelC}{if label("pending")!="Pending review" || label("missing")!="Unknown"{t.Fatal("status contract")}}
}
`,
	}
	files["CLAUDE.md"] = files["AGENTS.md"]
	for _, suffix := range []string{"A", "B", "C"} {
		files["src/status_"+strings.ToLower(suffix)+".go"] = fmt.Sprintf(`package fixture
func StatusLabel%s(key string)string {
 labels:=map[string]string{"active":"Active","pending":"Pending review","suspended":"Suspended","closed":"Closed"}
 if value,ok:=labels[key];ok{return value};return "Unknown"
}
`, suffix)
	}
	var panel strings.Builder
	panel.WriteString("package fixture\n\nimport \"strings\"\n\ntype Section struct { Label string; Metric int }\ntype Panel struct { Status string; Sections []Section; Total int }\n")
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&panel, `// formatField%d normalizes the field for one panel section.
func formatField%d(value string) string {
 value = strings.TrimSpace(value)
 if len(value) > %d {
  return value[:%d]
 }
 return value
}

// deriveMetric%d calculates the metric for one panel section.
func deriveMetric%d(values map[string]int) int {
 value, ok := values["m%d"]
 if !ok {
  return 0
 }
 return value * %d
}

func renderSection%d(values map[string]int) Section {
 return Section{
  Label: formatField%d("section %d"),
  Metric: deriveMetric%d(values),
 }
}

`, i, i, 20+i, 20+i, i, i, i, i, i, i, i, i)
	}
	panel.WriteString("func RenderPanel(status string, values map[string]int) Panel {\nsections:=[]Section{\n")
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&panel, "renderSection%d(values),\n", i)
	}
	panel.WriteString("}\ntotal:=0\nfor _,section:=range sections {total+=section.Metric}\nreturn Panel{StatusLabelA(status),sections,total}\n}\n")
	files["src/big_panel.go"] = panel.String()
	for path, body := range files {
		if strings.HasSuffix(path, ".go") {
			formatted, err := format.Source([]byte(body))
			if err != nil {
				return fmt.Errorf("format %s: %w", path, err)
			}
			body = string(formatted)
		}
		if err := writeFile(root, path, body); err != nil {
			return err
		}
	}
	return nil
}
