package main

import (
	"bytes"
	"fmt"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

func renderHelpMarkdown(markdown string) (string, error) {
	var buf bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	policy := bluemonday.UGCPolicy()
	policy.RequireNoFollowOnLinks(false)
	return policy.Sanitize(buf.String()), nil
}
