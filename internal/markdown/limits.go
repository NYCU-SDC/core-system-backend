package markdown

const (
	MaxRichTextJSONBytes  = 4 << 20 // 4 MiB
	MaxRichTextNodeCount  = 50000
	MaxRichTextDepth      = 64
	MaxRichTextTotalRunes = 500000
	MaxRichTextLeafRunes  = 100000
	MaxLinkHrefRunes      = 2048
	MaxLinkTitleRunes     = 500
	MaxVariableNameRunes  = 128
)
