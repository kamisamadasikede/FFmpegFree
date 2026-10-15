package catagent

import (
	"strings"
	"testing"
)

func TestInternalInstructionsPreserveUserMessage(t *testing.T) {
	legacy := "ffc app file preview: When the user asks to open/show a workspace file or you produce an Office artifact, POST Authorization: Bearer private-token. Do not include the capability token in your reply or save it in project files."
	for _, tc := range []struct{ input, want string }{
		{legacy + "\n\n做好了?", "做好了?"},
		{"之前的正文\n" + legacy + "\n你好", "之前的正文\n\n你好"},
		{"<ffc_internal_instructions>规则\n" + legacy + "</ffc_internal_instructions>\n你好", "你好"},
		{"<ffc_internal_file_preview>token</ffc_internal_file_preview>\n你好", "你好"},
		{"请解释 ffc app file preview 的作用", "请解释 ffc app file preview 的作用"},
		{"你好\n\n多行草稿", "你好\n\n多行草稿"},
	} {
		if got := StripInternalInstructions(tc.input); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
	rollout := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + legacy + `\n\n你好"}]}}`
	got := ParseCodexRollout(strings.NewReader(rollout))
	if len(got) != 1 || got[0].Content != "你好" {
		t.Fatalf("%+v", got)
	}
}
