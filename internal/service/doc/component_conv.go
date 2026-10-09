package doc

import "FFmpegFree/internal/doceng"

// NewComponentConverter 把本包的格式 / markdown 工具接到 doceng.ComponentConverter。
func (s *Service) NewComponentConverter() *doceng.ComponentConverter {
	return &doceng.ComponentConverter{
		ConvertToArg: convertToArg,
		InFilterFor:  inFilterFor,
		FamilyOf:     familyOf,
		MarkdownToHTML: func(md []byte, title, mode, origDir string) ([]byte, error) {
			return markdownToHTML(md, title, imagesForComponent, origDir)
		},
		HTMLToMarkdown: htmlToMarkdown,
		ReadTextFile:   readTextFile,
		WriteUTF8Temp:  writeUTF8Temp,
	}
}
