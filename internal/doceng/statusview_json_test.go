package doceng

import (
	"encoding/json"
	"strings"
	"testing"
)

// doc:component 由 Registry 发 StatusView：字段名必须和 DocComponentStatus 一样（小写），路径不外露。
func TestStatusViewJSONMatchesContract(t *testing.T) {
	b, err := json.Marshal(StatusView{State: "ready", ComponentState: "checking", Engines: []EngineInfo{}, Path: "/secret/soffice"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, k := range []string{`"state":"ready"`, `"componentState":"checking"`, `"engines":[]`, `"canDownload":false`, `"downloadBytes":0`, `"installBytes":0`} {
		if !strings.Contains(s, k) {
			t.Fatalf("缺 %s：%s", k, s)
		}
	}
	if strings.Contains(s, "secret") || strings.Contains(s, `"State"`) {
		t.Fatal(s)
	}
}
