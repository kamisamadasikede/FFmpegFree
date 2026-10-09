package doceng

import "testing"

func TestPrefOrder(t *testing.T) {
	if got := PrefOrder("auto"); len(got) != 3 || got[0] != IDOffice {
		t.Fatalf("auto: %v", got)
	}
	if got := PrefOrder("wps"); got[0] != IDWPS || got[1] != IDOffice || got[2] != IDComponent {
		t.Fatalf("wps first: %v", got)
	}
	if got := PrefOrder("nope"); got[0] != IDOffice {
		t.Fatalf("bad pref falls to auto: %v", got)
	}
}

func TestPickOrderAndSkip(t *testing.T) {
	engines := []Detected{
		{ID: IDOffice, Installed: true, Available: true, Families: []string{FamilyText, FamilySheet}, Version: "16.0", ExcelMajor: 16},
		{ID: IDWPS, Installed: true, Available: true, Families: []string{FamilyText}, Version: "12.1"},
		{ID: IDComponent, Installed: true, Available: true, Families: []string{FamilyText, FamilySheet, FamilySlide}, Source: "downloaded", Version: "26.2.6"},
	}
	sk := NewSkipTracker()
	cs := Pick(PrefOrder("auto"), engines, sk, "docx", "pdf")
	if len(cs) != 3 || cs[0].ID != IDOffice {
		t.Fatalf("got %#v", cs)
	}
	// WPS 不做 odt
	cs = Pick(PrefOrder("auto"), engines, sk, "odt", "pdf")
	if len(cs) != 2 || cs[0].ID != IDOffice || cs[1].ID != IDComponent {
		t.Fatalf("odt: %#v", cs)
	}
	sk.RecordFailure(IDOffice, FamilyText)
	sk.RecordFailure(IDOffice, FamilyText)
	cs = Pick(PrefOrder("auto"), engines, sk, "docx", "pdf")
	if len(cs) != 2 || cs[0].ID != IDWPS {
		t.Fatalf("after skip: %#v", cs)
	}
}

func TestWPSNoODF(t *testing.T) {
	engines := []Detected{{ID: IDWPS, Installed: true, Available: true, Families: []string{FamilyText, FamilySheet, FamilySlide}}}
	if cs := Pick([]string{IDWPS}, engines, nil, "odt", "pdf"); len(cs) != 0 {
		t.Fatalf("wps should not do odt: %#v", cs)
	}
	if cs := Pick([]string{IDWPS}, engines, nil, "ods", "xlsx"); len(cs) != 0 {
		t.Fatalf("wps should not do ods: %#v", cs)
	}
}

func TestSimplePDF(t *testing.T) {
	if !SimplePDF("docx") || SimplePDF("pptx") {
		t.Fatal("simple pdf inputs")
	}
}

func TestPrimarySource(t *testing.T) {
	engines := []Detected{
		{ID: IDWPS, Available: true, Version: "12"},
		{ID: IDComponent, Available: true, Source: "downloaded", Version: "26"},
	}
	src, ver := PrimarySource(PrefOrder("auto"), engines)
	if src != IDWPS || ver != "12" {
		t.Fatalf("got %s %s", src, ver)
	}
	src, ver = PrimarySource(PrefOrder(IDComponent), engines)
	if src != "downloaded" || ver != "26" {
		t.Fatalf("component pref: %s %s", src, ver)
	}
}
