package team

// Character offsets preserve original messages and allow deterministic bounded
// retrieval without an extra summarizer or a silent loss of earlier content.
type MessageSlice struct {
	Message    Message `json:"message"`
	Offset     int     `json:"offset"`
	NextOffset int     `json:"nextOffset"`
	TotalChars int     `json:"totalChars"`
	Truncated  bool    `json:"truncated"`
}

func messageSlice(m Message, offset, limit int) MessageSlice {
	text := []rune(m.Content)
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	if limit <= 0 {
		limit = 10000
	}
	if limit > 50000 {
		limit = 50000
	}
	end := min(offset+limit, len(text))
	m.Content = string(text[offset:end])
	return MessageSlice{m, offset, end, len(text), end < len(text)}
}
