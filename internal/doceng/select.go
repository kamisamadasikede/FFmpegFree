package doceng

// PrefOrder 返回按设置和自动顺序排列的引擎 id 列表（6.12.25）。
// pref 是 Settings.docEngine：auto | office | wps | component；空 = auto。
func PrefOrder(pref string) []string {
	auto := []string{IDOffice, IDWPS, IDComponent}
	switch pref {
	case "", "auto":
		return auto
	case IDOffice, IDWPS, IDComponent:
		out := []string{pref}
		for _, id := range auto {
			if id != pref {
				out = append(out, id)
			}
		}
		return out
	default:
		return auto
	}
}

// Candidate 是挑出来准备试的一个引擎（含它当前能处理的 families）。
type Candidate struct {
	Detected
}

// Pick 按顺序挑能做 src→target、且 available、且没被跳过的引擎。
// office 的 csv 目标还要求 ExcelMajor ≥ 16（未知时允许试，真机失败再跳）。
// component 的 downloaded 优先于 system：Detected 里只保留正在用的那一个。
func Pick(order []string, engines []Detected, skips *SkipTracker, src, target string) []Candidate {
	byID := map[string]Detected{}
	for _, e := range engines {
		if e.Installed || e.ID == IDComponent {
			byID[e.ID] = e
		}
	}
	var out []Candidate
	for _, id := range order {
		e, ok := byID[id]
		if !ok || !e.Available {
			continue
		}
		fam := familyOf(src)
		if fam == "" {
			continue
		}
		if skips != nil && skips.Skipped(id, fam) {
			continue
		}
		// 按实际 families 过滤
		if !CanConvert(id, src, target, e.Families) {
			continue
		}
		if id == IDOffice && target == "csv" && e.ExcelMajor > 0 && e.ExcelMajor < 16 {
			continue
		}
		out = append(out, Candidate{Detected: e})
	}
	return out
}

// EnginesForTarget 返回现在能做这个转换的引擎 id（按使用顺序），供 DocTarget.engines。
func EnginesForTarget(order []string, engines []Detected, skips *SkipTracker, src, target string) []string {
	cs := Pick(order, engines, skips, src, target)
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return ids
}

// PrimarySource 是设置页“正在使用 …”看的 source（6.12.28）：按顺序第一个 available 的引擎。
func PrimarySource(order []string, engines []Detected) (source, version string) {
	byID := map[string]Detected{}
	for _, e := range engines {
		byID[e.ID] = e
	}
	for _, id := range order {
		e, ok := byID[id]
		if !ok || !e.Available {
			continue
		}
		switch id {
		case IDOffice:
			return IDOffice, e.Version
		case IDWPS:
			return IDWPS, e.Version
		case IDComponent:
			return e.Source, e.Version
		}
	}
	return "", ""
}
