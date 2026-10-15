package catagent

import (
	"runtime"
	"sort"
	"strings"
)

// MergeToolEnv adds ephemeral application capabilities to CLI children. Values
// never enter native shared configuration files or the user-visible transcript.
func MergeToolEnv(base []string, values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(base)+len(keys))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		replaced := false
		for _, wanted := range keys {
			if key == wanted || runtime.GOOS == "windows" && strings.EqualFold(key, wanted) {
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, entry)
		}
	}
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
