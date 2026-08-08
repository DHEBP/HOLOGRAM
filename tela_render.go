package main

// Rendering for untrusted TELA content: syntax highlighting for source files and
// markdown for READMEs.
//
// Structure and the escape-on-every-failure-path discipline follow
// go-gitea/gitea, modules/highlight/highlight.go and modules/markup/markdown
// (both MIT). Two things are deliberately NOT copied from Gitea.
//
// Gitea renders markdown with goldmark's html.WithUnsafe() and lets raw HTML
// through to a permissive sanitiser. HOLOGRAM leaves goldmark's Unsafe=false
// default alone, so attacker-authored HTML is replaced by an HTML comment and
// never reaches the sanitiser at all. bluemonday then runs anyway. Two
// independent gates: never remove one because the other is present. The reason
// the bar is higher here than in Gitea is the blast radius - this HTML is
// injected into the app's own privileged webview, the same JS realm where the
// Wails bindings expose the wallet. In Gitea an XSS steals a session; here it
// would reach GetSeedPhrase and Transfer.
//
// Gitea also ports a large hand-built lexer lookup table because lexers.Get is
// slow at its scale. TELA's file vocabulary is seven extensions, so
// lexers.Match is enough and the table is not worth the divergence.

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
)

const (
	// highlightStyleName must round-trip through styles.Get. A name chroma does
	// not know returns the "swapoff" style rather than nil or an error, so a
	// typo here ships the wrong theme silently. TestHighlightStyleExists pins it.
	highlightStyleName = "catppuccin-mocha"

	// maxHighlightBytes caps what is worth tokenising. Highlighting inflates a
	// file roughly 6x for JavaScript and 9x for HTML, so a large file becomes a
	// DOM an order of magnitude larger than itself. A TELA DOC is capped at 18KB
	// by validateFileContent, so this is a guard against reassembled shards and
	// arbitrary files read off a clone directory, not a policy - it should never
	// fire on ordinary TELA content.
	maxHighlightBytes = 256 << 10

	// maxPlaintextBytes caps what is returned at all. Past this the content is
	// dropped and only its size is reported, because handing tens of megabytes
	// across the Wails bridge and into the DOM freezes the UI thread.
	maxPlaintextBytes = 2 << 20

	// maxMarkdownBytes caps markdown input. Rejected rather than truncated: a
	// half-parsed document is a half-sanitised document.
	//
	// It is NOT the real protection - see markdownRenderBudget. It is set at the
	// same 256KB as maxHighlightBytes because that is what bounds the work a
	// render that has already been given up on can still be doing.
	maxMarkdownBytes = 256 << 10

	// maxMarkdownHTMLBytes caps the HTML handed back to the webview.
	//
	// A SECOND gate, not a restatement of the first: input size does not predict
	// output size, and the worst amplifier measured here is also one of the
	// fastest, so the time budget above does not catch it. Measured on this
	// machine at 200KB of input: a ```js fence renders to 3.98MB in 0.57s
	// (x19.9), ">" to 5.4MB (x27) but slowly, task lists x5.9, tables x5.5,
	// emphasis x2.8. That HTML is assigned to Svelte's {@html}, so WebKit builds
	// every element of it on the UI thread — the ">" case is roughly one nested
	// <blockquote> per input byte.
	//
	// 2MB leaves a wide margin over anything a README does: 150KB of ordinary
	// prose, headings, links and lists renders to well under a tenth of it.
	maxMarkdownHTMLBytes = 2 << 20

	// markdownRenderBudget is how long a markdown render may take before the
	// caller is told it failed.
	//
	// goldmark is quadratic on some nested-inline shapes and nothing in it takes
	// a context, so size alone cannot bound the cost. Measured on this machine
	// with "[x](" repeated: 50KB 0.77s, 100KB 3.0s, 200KB 12.1s - a clean 4x per
	// doubling. The repository view renders a README the moment it opens, so an
	// unbounded parse is a zero-click freeze armed by whoever published the DOC.
	//
	// ⚠️ Honest residual: goldmark cannot be cancelled, so a render that blows
	// the budget keeps running to completion on its own goroutine. Nothing waits
	// for it and its result is dropped, but it does hold a core until it
	// finishes - which is the reason maxMarkdownBytes above is 256KB and not 1MiB.
	markdownRenderBudget = 2 * time.Second

	// binarySniffBytes is how much of a file is inspected for NUL bytes and
	// invalid UTF-8. Same window git uses.
	binarySniffBytes = 8000
)

// HighlightResult is the outcome of highlighting one file.
//
// HTML is safe to inject with Svelte's {@html}: chroma runs every token's text
// through html.EscapeString, and every path in this file that does not produce
// chroma output either escapes the content itself or returns nothing. It is one
// of only two values in this package for which that is true.
type HighlightResult struct {
	HTML        string `json:"html"`
	Language    string `json:"language"`
	Highlighted bool   `json:"highlighted"`
	Reason      string `json:"reason,omitempty"`
	Bytes       int    `json:"bytes"`
}

// chromaStyleRe bounds the style attribute the sanitiser will keep.
//
// bluemonday matches this against the WHOLE attribute value, so the alternation
// is closed: it can express a colour and a font tweak and nothing else. No
// url(), no position, no width - a span that survives can be coloured, and that
// is the entire capability admitted.
//
// Nothing an attacker writes reaches this regex under normal operation, because
// goldmark's Unsafe=false already replaced their markup with a comment. The only
// producer of these attributes is chroma itself. The regex exists so that if the
// first gate ever fails, what gets through is a colour.
var chromaStyleRe = regexp.MustCompile(
	`^(?:` + chromaStyleDecl + `)(?:;(?:` + chromaStyleDecl + `))*;?$`)

const chromaStyleDecl = `(?:color|background-color):#[0-9a-fA-F]{3,8}` +
	`|font-weight:(?:bold|normal)` +
	`|font-style:(?:italic|normal)` +
	`|text-decoration:(?:underline|line-through|none)`

// langClassRe keeps the language hint goldmark puts on fenced code blocks that
// chroma had no lexer for, so the view can still label them.
var langClassRe = regexp.MustCompile(`^language-[A-Za-z0-9_+#.-]{1,32}$`)

var checkboxTypeRe = regexp.MustCompile(`^checkbox$`)

// telaMarkdownPolicy builds the sanitiser applied to every rendered TELA
// markdown document.
//
// Derived from Gitea's modules/markup/sanitizer_default.go (MIT), narrowed.
// Gitea's widenings - video, picture/source, MathML, AllowURLSchemesMatching
// with an allow-all regex - are deliberately not taken.
//
// Gitea's explicit disallowScheme("javascript") style calls are also not copied.
// They exist in Gitea only because Gitea first widens to an allow-all scheme
// regex. This policy never widens, so the allowlist below is the actual
// protection; restating the denials would read as the protection while doing
// nothing.
func telaMarkdownPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	// Relative URLs are meaningless for SCID-addressed DOCs and actively
	// harmful: in a Wails webview they resolve against the app's own asset
	// origin, so a relative href could navigate the whole app frame away from
	// the Svelte router. Dropping the attribute leaves the link text visible.
	p.AllowRelativeURLs(false)

	// Restated so a future edit cannot widen the scheme set by accident.
	// UGCPolicy already sets exactly these through AllowStandardURLs.
	p.AllowURLSchemes("http", "https", "mailto")

	// Syntax-highlighted code arrives as coloured spans.
	p.AllowElements("span")
	p.AllowAttrs("style").Matching(chromaStyleRe).OnElements("span", "pre", "code")
	p.AllowAttrs("class").Matching(langClassRe).OnElements("code", "pre")

	// goldmark's TaskList emits <input checked disabled type="checkbox">.
	// UGCPolicy allows no <input> at all, so checklists vanish without this.
	p.AllowAttrs("type").Matching(checkboxTypeRe).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")

	// The frontend must still intercept clicks and hand external links to the
	// system browser; target="_blank" is a signal, not the enforcement.
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	return p
}

var (
	// chromaStyle is never nil - styles.Get falls back rather than failing.
	chromaStyle = styles.Get(highlightStyleName)

	// chromaFormatter is safe to share: its only mutable field is a style cache
	// guarded by a mutex, everything else is fixed at construction.
	//
	// PreventSurroundingPre means chroma emits no <pre>, no per-line spans and
	// no background, so the existing <pre class="content-code"> keeps its own
	// layout and background and only token foregrounds come from the style.
	chromaFormatter = chromahtml.New(
		chromahtml.WithClasses(false),
		chromahtml.WithLineNumbers(false),
		chromahtml.PreventSurroundingPre(true),
	)

	telaMarkdownPolicySingleton = telaMarkdownPolicy()

	// telaMarkdown is the goldmark instance used for all TELA markdown.
	//
	// html.WithUnsafe() is absent on purpose - see the file comment.
	// parser.WithAttribute() is absent because {#id .class} syntax on untrusted
	// input buys nothing a README needs. parser.WithAutoHeadingID() is absent
	// because UGCPolicy keeps an id attribute, and ids in untrusted content
	// invite DOM clobbering; anchor links are not worth that.
	//
	// TableCellAlignAttribute is load-bearing rather than cosmetic: it emits
	// align="left", which the policy keeps, where the style-based alternative
	// emits style= and alignment would silently disappear.
	telaMarkdown = goldmark.New(
		goldmark.WithExtensions(
			extension.NewTable(
				extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute),
			),
			extension.Strikethrough,
			extension.TaskList,
			extension.Linkify,
			highlighting.NewHighlighting(
				highlighting.WithStyle(highlightStyleName),
				highlighting.WithFormatOptions(
					chromahtml.WithClasses(false),
					chromahtml.WithLineNumbers(false),
				),
			),
		),
	)
)

// isBinaryContent reports whether content looks like something no viewer should
// try to render. Same rule git uses: a NUL byte or invalid UTF-8 near the start.
//
// It slices rather than copying, so it costs nothing on a large string.
func isBinaryContent(content string) bool {
	head := content
	if len(head) > binarySniffBytes {
		head = head[:binarySniffBytes]
		// A rune split by the sniff boundary is not evidence of binary. A UTF-8
		// rune is at most 4 bytes, so backing off at most 3 resolves the split
		// without hiding genuinely invalid bytes.
		for i := 0; i < 3 && len(head) > 0 && !utf8.ValidString(head); i++ {
			head = head[:len(head)-1]
		}
	}
	if strings.IndexByte(head, 0) >= 0 {
		return true
	}
	return !utf8.ValidString(head)
}

// highlightSource turns one file into HTML for the file viewer.
//
// filename is a lexer lookup key and NOTHING ELSE. It comes off chain through a
// DOC's NameHdr, so it is fully attacker-controlled; it must never be
// interpolated into the returned HTML. Render it with plain Svelte {} instead.
//
// Every path that cannot produce chroma output returns escaped text or nothing.

// telaCompressionSuffixes are what TELA appends to a stored file name when the
// DOC body is compressed (tela/compression.go). They describe how the bytes are
// stored, not what the file is.
var telaCompressionSuffixes = []string{".gz", ".br"}

// lexerName strips a TELA compression suffix so the lexer sees the file's real
// extension.
//
// A compressed DOC carries the suffix verbatim in nameHdr - "index.html.gz" -
// and chroma matches on the LAST extension, so it never saw ".html" and fell
// through to the fallback lexer. Every compressed file therefore rendered
// unhighlighted AND was labelled "plaintext" while displaying obvious HTML.
// This was the common case rather than an edge: 22 of 45 DOCs sampled from
// mainnet are compressed.
//
// Only the lexer lookup uses this. The displayed name stays as published,
// because that is what the chain actually stores.
func lexerName(filename string) string {
	for _, ext := range telaCompressionSuffixes {
		if trimmed := strings.TrimSuffix(filename, ext); trimmed != filename {
			return trimmed
		}
	}
	return filename
}

func highlightSource(filename, content string) HighlightResult {
	result := HighlightResult{Bytes: len(content)}

	if isBinaryContent(content) {
		result.Reason = "binary"
		return result
	}
	if len(content) > maxPlaintextBytes {
		result.Reason = "too large"
		return result
	}
	if len(content) > maxHighlightBytes {
		result.Language = "plaintext"
		result.Reason = "too large to highlight"
		result.HTML = html.EscapeString(content)
		return result
	}

	// The reported language comes from a lexer that actually matched. The
	// fallback lexer names itself "fallback", which means nothing to a reader,
	// so an unrecognised extension is reported as plaintext instead.
	lexer := lexers.Match(lexerName(filename))
	result.Language = "plaintext"
	if lexer == nil {
		lexer = lexers.Fallback
	} else if cfg := lexer.Config(); cfg != nil && cfg.Name != "" {
		result.Language = cfg.Name
	}

	// Coalesce merges adjacent same-type tokens, which cuts the span count.
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, content)
	if err != nil {
		result.Reason = "tokenise failed"
		result.HTML = html.EscapeString(content)
		return result
	}

	var buf bytes.Buffer
	if err := chromaFormatter.Format(&buf, chromaStyle, iterator); err != nil {
		result.Reason = "format failed"
		result.HTML = html.EscapeString(content)
		return result
	}

	result.HTML = buf.String()
	result.Highlighted = true
	return result
}

// renderMarkdownSafe converts untrusted TELA markdown to sanitised HTML.
//
// The returned string is safe to inject with Svelte's {@html}. Never
// concatenate anything onto it; the guarantee covers exactly what this function
// returns.
//
// On any error it returns an empty string rather than what it managed to
// produce, because a partially rendered document is a partially sanitised one.
// It gives up after markdownRenderBudget rather than blocking the caller for as
// long as the document takes.
func renderMarkdownSafe(source string) (string, error) {
	if len(source) > maxMarkdownBytes {
		return "", fmt.Errorf("markdown too large to render: %d bytes, limit %d", len(source), maxMarkdownBytes)
	}
	if isBinaryContent(source) {
		return "", fmt.Errorf("markdown content is binary")
	}

	type outcome struct {
		html string
		err  error
	}

	// Buffered, so the goroutine can finish and exit after the budget expired
	// instead of blocking forever on a send nobody is receiving.
	done := make(chan outcome, 1)
	go func() {
		html, err := convertMarkdown(source)
		done <- outcome{html, err}
	}()

	timer := time.NewTimer(markdownRenderBudget)
	defer timer.Stop()

	select {
	case result := <-done:
		return result.html, result.err
	case <-timer.C:
		return "", fmt.Errorf("markdown render took longer than %s", markdownRenderBudget)
	}
}

// convertMarkdown is the actual render. It runs on its own goroutine, so it owns
// the panic recovery: an unrecovered panic there would take the process down
// rather than being caught by the caller.
func convertMarkdown(source string) (rendered string, err error) {
	// goldmark parses hostile input; a panic in it must not take the app down.
	// Gitea recovers around its convert for the same reason.
	defer func() {
		if r := recover(); r != nil {
			rendered = ""
			err = fmt.Errorf("markdown render panicked: %v", r)
		}
	}()

	var buf bytes.Buffer
	if convErr := telaMarkdown.Convert([]byte(source), &buf); convErr != nil {
		return "", fmt.Errorf("markdown render failed: %w", convErr)
	}

	// Checked before Sanitize: the point is to never build the string, never
	// sanitise it and never hand it across the bridge.
	if buf.Len() > maxMarkdownHTMLBytes {
		return "", fmt.Errorf("markdown renders to %d bytes of HTML, limit %d", buf.Len(), maxMarkdownHTMLBytes)
	}

	return telaMarkdownPolicySingleton.Sanitize(buf.String()), nil
}

// HighlightSource returns syntax-highlighted HTML for one file.
//
// Callers must render "language" and any filename with plain Svelte {}; only
// "html" is safe for {@html}.
func (a *App) HighlightSource(filename, content string) map[string]interface{} {
	result := highlightSource(filename, content)
	return map[string]interface{}{
		"success":     true,
		"html":        result.HTML,
		"language":    result.Language,
		"highlighted": result.Highlighted,
		"reason":      result.Reason,
		"bytes":       result.Bytes,
	}
}

// RenderTELAMarkdown returns sanitised HTML for a markdown document published
// on chain. Only "html" is safe for {@html}.
func (a *App) RenderTELAMarkdown(source string) map[string]interface{} {
	rendered, err := renderMarkdownSafe(source)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}
	}
	return map[string]interface{}{
		"success": true,
		"html":    rendered,
	}
}
