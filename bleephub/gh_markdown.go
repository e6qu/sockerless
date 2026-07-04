package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	ghtml "github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// GitHub Markdown rendering API (POST /markdown and POST /markdown/raw).
//
// `markdown` mode renders CommonMark plus the GitHub Flavored Markdown
// syntax extensions GitHub applies to all API rendering (tables,
// strikethrough, autolinks). `gfm` mode additionally renders hard line
// breaks and, mirroring GitHub's html-pipeline, links @mentions of existing
// users and #number references to existing issues/pull requests in the
// `context` repository — GitHub only links references that resolve.

var (
	// markdownModeRenderer matches GitHub's `markdown` mode: GFM syntax
	// extensions minus task lists and hard line breaks.
	markdownModeRenderer = goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify),
	)
	// gfmModeRenderer matches GitHub's `gfm` mode: full GFM plus hard wraps.
	gfmModeRenderer = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(ghtml.WithHardWraps()),
	)
)

func (s *Server) registerGHMarkdownRoutes() {
	s.route("POST /api/v3/markdown", s.handleRenderMarkdown)
	s.route("POST /api/v3/markdown/raw", s.handleRenderMarkdownRaw)
}

func (s *Server) handleRenderMarkdown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text    *string `json:"text"`
		Mode    string  `json:"mode"`
		Context string  `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Text == nil {
		writeGHValidationError(w, "Markdown", "text", "missing_field")
		return
	}
	switch req.Mode {
	case "", "markdown", "gfm":
	default:
		writeGHValidationError(w, "Markdown", "mode", "invalid")
		return
	}
	rendered, err := s.renderMarkdown(*req.Text, req.Mode, req.Context, s.baseURL(r))
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "Markdown rendering failed")
		return
	}
	writeRenderedHTML(w, rendered)
}

// handleRenderMarkdownRaw renders the raw request body (text/plain or
// text/x-markdown) in `markdown` mode.
func (s *Server) handleRenderMarkdownRaw(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Could not read request body")
		return
	}
	rendered, err := s.renderMarkdown(string(body), "markdown", "", s.baseURL(r))
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "Markdown rendering failed")
		return
	}
	writeRenderedHTML(w, rendered)
}

func writeRenderedHTML(w http.ResponseWriter, rendered string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(rendered))
}

func (s *Server) renderMarkdown(text, mode, context, baseURL string) (string, error) {
	renderer := markdownModeRenderer
	if mode == "gfm" {
		renderer = gfmModeRenderer
	}
	var buf bytes.Buffer
	if err := renderer.Convert([]byte(text), &buf); err != nil {
		return "", err
	}
	rendered := buf.String()
	if mode == "gfm" {
		rendered = s.linkifyGFMReferences(rendered, context, baseURL)
	}
	return rendered, nil
}

var (
	// mentionRefRe finds @login candidates; group 1 is the leading boundary
	// (start of text or a non-word, non-dot character), group 2 the login.
	mentionRefRe = regexp.MustCompile(`(^|[^0-9A-Za-z_.])@([A-Za-z0-9][A-Za-z0-9-]{0,38})`)
	// issueRefRe finds #number candidates; group 1 is the boundary, group 2
	// the issue/pull-request number.
	issueRefRe = regexp.MustCompile(`(^|[^0-9A-Za-z_.])#([0-9]+)`)
)

// linkifyGFMReferences post-processes rendered HTML the way GitHub's
// html-pipeline does: it walks the document and replaces @mention and
// #number references inside plain text — skipping text already inside
// <a>, <code>, and <pre> — with links, but only when the mention resolves
// to a real user and the number resolves to a real issue or pull request
// in the context repository.
func (s *Server) linkifyGFMReferences(rendered, context, baseURL string) string {
	var contextRepo *Repo
	if owner, name, found := strings.Cut(context, "/"); found {
		contextRepo = s.store.GetRepo(owner, name)
	}

	bodyCtx := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(rendered), bodyCtx)
	if err != nil {
		return rendered
	}

	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			switch n.DataAtom {
			case atom.A, atom.Code, atom.Pre:
				return
			}
		}
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == xhtml.TextNode {
				s.linkifyTextNode(n, c, contextRepo, baseURL)
			} else {
				walk(c)
			}
			c = next
		}
	}
	root := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	walk(root)

	var out bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := xhtml.Render(&out, c); err != nil {
			return rendered
		}
	}
	return out.String()
}

// linkifyTextNode replaces mention/issue references in a single text node
// with anchor elements, splicing the replacement nodes in place.
func (s *Server) linkifyTextNode(parent, textNode *xhtml.Node, contextRepo *Repo, baseURL string) {
	type ref struct {
		start, end int // bounds of the replaced token (@login / #n)
		href       string
		class      string
		label      string
	}
	text := textNode.Data
	var refs []ref

	for _, m := range mentionRefRe.FindAllStringSubmatchIndex(text, -1) {
		login := text[m[4]:m[5]]
		if s.store.LookupUserByLogin(login) == nil {
			continue
		}
		refs = append(refs, ref{
			start: m[4] - 1, end: m[5],
			href:  baseURL + "/" + login,
			class: "user-mention",
			label: "@" + login,
		})
	}
	if contextRepo != nil {
		for _, m := range issueRefRe.FindAllStringSubmatchIndex(text, -1) {
			numStr := text[m[4]:m[5]]
			n, err := strconv.Atoi(numStr)
			if err != nil {
				continue
			}
			var href string
			if s.store.GetIssueByNumber(contextRepo.ID, n) != nil {
				href = baseURL + "/" + contextRepo.FullName + "/issues/" + numStr
			} else if s.store.GetPullRequestByNumber(contextRepo.ID, n) != nil {
				href = baseURL + "/" + contextRepo.FullName + "/pull/" + numStr
			} else {
				continue
			}
			refs = append(refs, ref{
				start: m[4] - 1, end: m[5],
				href:  href,
				class: "issue-link js-issue-link",
				label: "#" + numStr,
			})
		}
	}
	if len(refs) == 0 {
		return
	}
	// Matches come from two independent scans over disjoint token shapes, so
	// they never overlap; order them by position for in-order splicing.
	for i := 1; i < len(refs); i++ {
		for j := i; j > 0 && refs[j].start < refs[j-1].start; j-- {
			refs[j], refs[j-1] = refs[j-1], refs[j]
		}
	}

	pos := 0
	insert := func(n *xhtml.Node) { parent.InsertBefore(n, textNode) }
	for _, rf := range refs {
		if rf.start > pos {
			insert(&xhtml.Node{Type: xhtml.TextNode, Data: text[pos:rf.start]})
		}
		a := &xhtml.Node{
			Type: xhtml.ElementNode, Data: "a", DataAtom: atom.A,
			Attr: []xhtml.Attribute{
				{Key: "class", Val: rf.class},
				{Key: "href", Val: rf.href},
			},
		}
		a.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: rf.label})
		insert(a)
		pos = rf.end
	}
	if pos < len(text) {
		insert(&xhtml.Node{Type: xhtml.TextNode, Data: text[pos:]})
	}
	parent.RemoveChild(textNode)
}
