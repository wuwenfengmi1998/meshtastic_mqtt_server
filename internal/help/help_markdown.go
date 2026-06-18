package help

import (
	"bytes"
	"fmt"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// RenderMarkdown 把 GFM markdown 转成净化后的 HTML，供 admin 编辑器预览
// 与 /help 路由直接渲染。
func RenderMarkdown(markdown string) (string, error) {
	var buf bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	policy := bluemonday.UGCPolicy()
	policy.RequireNoFollowOnLinks(false)
	return policy.Sanitize(buf.String()), nil
}
