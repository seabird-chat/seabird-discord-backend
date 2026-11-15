package seabird_discord

import (
	"fmt"
	"strings"

	"github.com/seabird-chat/seabird-go/pb"
)

// renderMessage converts an outgoing message to Discord markdown. replace is
// applied to literal text before escaping, so mention replacement still sees
// the original usernames.
//
// Core wraps text-only messages in a bare TextBlock, so a missing or bare
// text block is sent as raw text. That keeps markdown typed by plain-text
// clients working as it did before blocks were rendered.
func renderMessage(text string, block *pb.Block, replace func(string) string) string {
	if replace == nil {
		replace = func(s string) string { return s }
	}

	if block == nil || block.GetText() != nil {
		return replace(text)
	}

	r := &markdownRenderer{replace: replace}
	r.render(block)
	return strings.TrimRight(r.buf.String(), "\n")
}

type markdownRenderer struct {
	buf     strings.Builder
	replace func(string) string
}

func (r *markdownRenderer) render(block *pb.Block) {
	if block == nil {
		return
	}

	switch inner := block.Inner.(type) {
	case *pb.Block_Text:
		r.buf.WriteString(escapeMarkdown(r.replace(inner.Text.Text)))
	case *pb.Block_Container:
		for _, b := range inner.Container.Inner {
			r.render(b)
		}
	case *pb.Block_Italics:
		r.wrap("*", inner.Italics.Inner)
	case *pb.Block_Bold:
		r.wrap("**", inner.Bold.Inner)
	case *pb.Block_Underline:
		r.wrap("__", inner.Underline.Inner)
	case *pb.Block_Strikethrough:
		r.wrap("~~", inner.Strikethrough.Inner)
	case *pb.Block_Spoiler:
		r.wrap("||", inner.Spoiler.Inner)
	case *pb.Block_InlineCode:
		r.buf.WriteString(inlineCode(inner.InlineCode.Text))
	case *pb.Block_FencedCode:
		r.startLine()
		fmt.Fprintf(&r.buf, "```%s\n%s\n```\n", inner.FencedCode.Info, inner.FencedCode.Text)
	case *pb.Block_Link:
		label := r.sub(inner.Link.Inner)
		if label == "" || label == inner.Link.Url {
			r.buf.WriteString(inner.Link.Url)
		} else {
			fmt.Fprintf(&r.buf, "[%s](<%s>)", label, inner.Link.Url)
		}
	case *pb.Block_Heading:
		level := min(max(int(inner.Heading.Level), 1), 3)
		r.startLine()
		r.buf.WriteString(strings.Repeat("#", level) + " " + r.sub(inner.Heading.Inner) + "\n")
	case *pb.Block_Blockquote:
		r.startLine()
		for _, line := range strings.Split(r.sub(inner.Blockquote.Inner), "\n") {
			r.buf.WriteString("> " + line + "\n")
		}
	case *pb.Block_List:
		r.startLine()
		for _, item := range inner.List.Inner {
			r.buf.WriteString("- " + r.sub(item) + "\n")
		}
	case *pb.Block_Timestamp:
		fmt.Fprintf(&r.buf, "<t:%d:f>", inner.Timestamp.Inner.GetSeconds())
	default:
		r.buf.WriteString(escapeMarkdown(r.replace(block.Plain)))
	}
}

// sub renders block on its own, without trailing newlines.
func (r *markdownRenderer) sub(block *pb.Block) string {
	s := &markdownRenderer{replace: r.replace}
	s.render(block)
	return strings.TrimRight(s.buf.String(), "\n")
}

// wrap surrounds the rendered block with delim. Discord ignores delimiters
// next to whitespace, so edge whitespace is moved outside them.
func (r *markdownRenderer) wrap(delim string, block *pb.Block) {
	content := r.sub(block)
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		r.buf.WriteString(content)
		return
	}

	start := strings.Index(content, trimmed)
	r.buf.WriteString(content[:start])
	r.buf.WriteString(delim + trimmed + delim)
	r.buf.WriteString(content[start+len(trimmed):])
}

// startLine ends the current line so block-level markdown starts fresh.
func (r *markdownRenderer) startLine() {
	if s := r.buf.String(); s != "" && !strings.HasSuffix(s, "\n") {
		r.buf.WriteString("\n")
	}
}

// inlineCode picks a backtick fence longer than any run in text.
func inlineCode(text string) string {
	longest, run := 0, 0
	for _, c := range text {
		if c == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}

	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return fence + text + fence
}

var markdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	"*", `\*`,
	"_", `\_`,
	"~", `\~`,
	"|", `\|`,
	"`", "\\`",
	"[", `\[`,
	"]", `\]`,
)

// escapeMarkdown escapes Discord markdown in literal text. URLs are left
// alone so Discord still links them, and > and # are only escaped at line
// start, where they form quotes and headings, so mentions stay intact.
func escapeMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = escapeLine(line)
	}
	return strings.Join(lines, "\n")
}

func escapeLine(text string) string {
	prefix := ""
	if trimmed := strings.TrimLeft(text, " \t"); strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "#") {
		prefix = text[:len(text)-len(trimmed)] + `\`
		text = trimmed
	}

	var out strings.Builder
	for text != "" {
		idx := urlStart(text)
		if idx < 0 {
			idx = len(text)
		}
		out.WriteString(markdownEscaper.Replace(text[:idx]))
		text = text[idx:]

		end := strings.IndexAny(text, " \t\n")
		if end < 0 {
			end = len(text)
		}
		out.WriteString(text[:end])
		text = text[end:]
	}
	return prefix + out.String()
}

// urlStart returns the index of the first http(s) URL in s, or -1.
func urlStart(s string) int {
	for i := range len(s) {
		if strings.HasPrefix(s[i:], "http://") || strings.HasPrefix(s[i:], "https://") {
			return i
		}
	}
	return -1
}
