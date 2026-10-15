package officecli

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// SafeHTML preserves the CLI's drawing/layout markup and CSS while removing its
// editor, network scripts and active document content. The frontend supplies a
// small trusted viewer inside an opaque-origin, network-disabled iframe.
func SafeHTML(raw string) (string, error) {
	if len(raw) > MaxHTML {
		return "", fmt.Errorf("PPT HTML 太大")
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}
	var clean func(*html.Node)
	clean = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode {
				tag := strings.ToLower(child.Data)
				switch tag {
				case "script", "iframe", "object", "embed", "base", "link", "meta", "form", "input", "button", "audio", "video":
					n.RemoveChild(child)
					child = next
					continue
				}
				attrs := child.Attr[:0]
				for _, a := range child.Attr {
					key := strings.ToLower(a.Key)
					val := strings.TrimSpace(a.Val)
					if strings.HasPrefix(key, "on") || key == "srcdoc" || key == "href" || key == "action" || key == "formaction" {
						continue
					}
					if key == "src" && !strings.HasPrefix(strings.ToLower(val), "data:image/") {
						continue
					}
					attrs = append(attrs, a)
				}
				child.Attr = attrs
			}
			clean(child)
			child = next
		}
	}
	clean(doc)
	var buf bytes.Buffer
	if err = html.Render(&buf, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}
