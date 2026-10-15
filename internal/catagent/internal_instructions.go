package catagent

import "strings"

// StripInternalInstructions also handles rollouts written before internal rules
// were sent separately from user input. Never remove arbitrary user paragraphs.
func StripInternalInstructions(text string) string {
	for _, tag := range []string{"ffc_internal_instructions", "ffc_internal_file_preview"} {
		start, end := "<"+tag+">", "</"+tag+">"
		for {
			i := strings.Index(text, start)
			if i < 0 {
				break
			}
			j := strings.Index(text[i+len(start):], end)
			if j < 0 {
				break
			}
			text = text[:i] + text[i+len(start)+j+len(end):]
		}
	}
	const start = "ffc app file preview: When the user asks to open/show a workspace file or you produce an Office artifact,"
	const end = "Do not include the capability token in your reply or save it in project files."
	for {
		i := strings.Index(text, start)
		if i < 0 {
			break
		}
		j := strings.Index(text[i+len(start):], end)
		if j < 0 {
			break
		}
		text = text[:i] + text[i+len(start)+j+len(end):]
	}
	return strings.TrimSpace(text)
}
