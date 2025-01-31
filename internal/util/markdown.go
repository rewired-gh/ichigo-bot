package util

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/russross/blackfriday/v2"
)

func TelegramifyMarkdown(markdown string) string {
	definitions := make(map[string]Definition)
	ast := blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions)).Parse([]byte(markdown))

	collectDefinitions(ast, definitions)
	removeDefinitions(ast)

	var buf bytes.Buffer
	renderer := &TelegramRenderer{
		Definitions:             definitions,
		UnsupportedTagsStrategy: "escape",
	}
	ast.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		renderer.RenderNode(&buf, node, entering)
		return blackfriday.GoToNext
	})

	return buf.String()
}

type Definition struct {
	Title string
	URL   string
}

type TelegramRenderer struct {
	Definitions             map[string]Definition
	UnsupportedTagsStrategy string
}

func (r *TelegramRenderer) RenderNode(buf *bytes.Buffer, node *blackfriday.Node, entering bool) {
	if !entering {
		return
	}

	switch node.Type {
	case blackfriday.Heading:
		r.renderHeading(buf, node)
	case blackfriday.Strong:
		r.renderStrong(buf, node)
	case blackfriday.Del:
		r.renderDelete(buf, node)
	case blackfriday.Emph:
		r.renderEmphasis(buf, node)
	case blackfriday.List:
		r.renderList(buf, node)
	case blackfriday.Item:
		r.renderListItem(buf, node)
	case blackfriday.CodeBlock:
		r.renderCodeBlock(buf, node)
	case blackfriday.Link:
		r.renderLink(buf, node)
	case blackfriday.Image:
		r.renderImage(buf, node)
	case blackfriday.Text:
		r.renderText(buf, node)
	case blackfriday.BlockQuote:
		r.renderBlockquote(buf, node)
	case blackfriday.HTMLBlock:
		r.renderHTML(buf, node)
	}
}

func (r *TelegramRenderer) renderHeading(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.WriteString(wrap(string(node.Literal), "*"))
}

func (r *TelegramRenderer) renderStrong(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.WriteString(wrap(string(node.Literal), "*"))
}

func (r *TelegramRenderer) renderDelete(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.WriteString(wrap(string(node.Literal), "~"))
}

func (r *TelegramRenderer) renderEmphasis(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.WriteString(wrap(string(node.Literal), "_"))
}

func (r *TelegramRenderer) renderList(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.Write(node.Literal)
}

func (r *TelegramRenderer) renderListItem(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.WriteString(strings.Replace(string(node.Literal), "*", "•", 1))
}

func (r *TelegramRenderer) renderCodeBlock(buf *bytes.Buffer, node *blackfriday.Node) {
	content := regexp.MustCompile(`^#![a-z]+\n`).ReplaceAllString(string(node.Literal), "")
	buf.WriteString(wrap(escapeSymbols(content, "code"), "```", "\n"))
}

func (r *TelegramRenderer) renderLink(buf *bytes.Buffer, node *blackfriday.Node) {
	text := string(node.FirstChild.Literal)
	url := string(node.LinkData.Destination)
	if !isURL(url) {
		buf.WriteString(escapeSymbols(text))
		return
	}
	buf.WriteString(fmt.Sprintf("[%s](%s)", escapeSymbols(text), escapeSymbols(url, "link")))
}

func (r *TelegramRenderer) renderImage(buf *bytes.Buffer, node *blackfriday.Node) {
	text := string(node.FirstChild.Literal)
	url := string(node.LinkData.Destination)
	if !isURL(url) {
		buf.WriteString(escapeSymbols(text))
		return
	}
	buf.WriteString(fmt.Sprintf("[%s](%s)", escapeSymbols(text), escapeSymbols(url, "link")))
}

func (r *TelegramRenderer) renderText(buf *bytes.Buffer, node *blackfriday.Node) {
	buf.WriteString(escapeSymbols(string(node.Literal)))
}

func (r *TelegramRenderer) renderBlockquote(buf *bytes.Buffer, node *blackfriday.Node) {
	content := string(node.Literal)
	buf.WriteString(processUnsupportedTags(content, r.UnsupportedTagsStrategy))
}

func (r *TelegramRenderer) renderHTML(buf *bytes.Buffer, node *blackfriday.Node) {
	content := string(node.Literal)
	buf.WriteString(processUnsupportedTags(content, r.UnsupportedTagsStrategy))
}

func collectDefinitions(node *blackfriday.Node, definitions map[string]Definition) {
	node.Walk(func(n *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if n.Type == blackfriday.Link && entering {
			definitions[string(n.LinkData.Destination)] = Definition{
				Title: string(n.FirstChild.Literal),
				URL:   string(n.LinkData.Destination),
			}
		}
		return blackfriday.GoToNext
	})
}

func removeDefinitions(node *blackfriday.Node) {
	node.Walk(func(n *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if n.Type == blackfriday.Link && entering {
			n.Unlink()
		}
		return blackfriday.GoToNext
	})
}

func wrap(s string, wrappers ...string) string {
	return strings.Join(append(wrappers, append([]string{s}, reverse(wrappers)...)...), "")
}

func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

func isURL(s string) bool {
	_, err := url.ParseRequestURI(s)
	return err == nil
}

func escapeSymbols(text string, textType ...string) string {
	if text == "" {
		return text
	}
	switch {
	case len(textType) > 0 && textType[0] == "code":
		return strings.ReplaceAll(strings.ReplaceAll(text, "`", "\\`"), "\\", "\\\\")
	case len(textType) > 0 && textType[0] == "link":
		return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(text, "\\", "\\\\"), "(", "\\("), ")", "\\)")
	default:
		replacer := strings.NewReplacer(
			"_", "\\_",
			"*", "\\*",
			"[", "\\[",
			"]", "\\]",
			"(", "\\(",
			")", "\\)",
			"~", "\\~",
			"`", "\\`",
			">", "\\>",
			"#", "\\#",
			"+", "\\+",
			"-", "\\-",
			"=", "\\=",
			"|", "\\|",
			"{", "\\{",
			"}", "\\}",
			".", "\\.",
			"!", "\\!",
		)
		return replacer.Replace(text)
	}
}

func processUnsupportedTags(content, strategy string) string {
	switch strategy {
	case "escape":
		return escapeSymbols(content)
	case "remove":
		return ""
	case "keep":
		fallthrough
	default:
		return content
	}
}
