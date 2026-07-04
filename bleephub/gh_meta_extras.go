package bleephub

import (
	_ "embed"
	"encoding/json"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
)

// Top-level GitHub REST API meta surfaces: the emoji catalog, the zen quote
// endpoint, the octocat ASCII-art endpoint, the REST API versions list, and
// credential revocation.

// gemojiCatalog is the github/gemoji dataset rendered as the GET /emojis
// name → image-path catalog: one "name path" pair per line, where path is
// relative to <base>/images/icons/emoji/ (unicode/<codepoints>.png?v8 for
// Unicode emoji, <name>.png?v8 for GitHub's custom emoji).
//
//go:embed gemoji_catalog.txt
var gemojiCatalog string

// zenQuotes is GitHub's zen quote set, served verbatim by GET /zen and used
// for the octocat speech bubble when no valid `s` parameter is supplied.
var zenQuotes = []string{
	"Accessible for all.",
	"Anything added dilutes everything else.",
	"Approachable is better than simple.",
	"Avoid administrative distraction.",
	"Design for failure.",
	"Encourage flow.",
	"Favor focus over features.",
	"Half measures are as bad as nothing at all.",
	"It's not fully shipped until it's fast.",
	"Keep it logically awesome.",
	"Mind your words, they are important.",
	"Non-blocking is better than blocking.",
	"Practicality beats purity.",
	"Responsive is better than fast.",
	"Speak like a human.",
}

// octocatSpeechRe is the character set GitHub accepts for the octocat `s`
// parameter (word characters, digits, spaces, commas, hyphens, slashes).
// Anything outside it makes GitHub fall back to a random zen quote.
var octocatSpeechRe = regexp.MustCompile(`^[A-Za-z0-9_ ,\-/]+$`)

func (s *Server) registerGHMetaExtrasRoutes() {
	s.route("GET /api/v3/emojis", s.handleGHEmojis)
	s.route("GET /api/v3/zen", s.handleGHZen)
	s.route("GET /api/v3/octocat", s.handleGHOctocat)
	s.route("GET /api/v3/versions", s.handleGHAPIVersions)
	s.route("POST /api/v3/credentials/revoke", s.handleGHCredentialsRevoke)
	// Instance-hosted emoji images the /emojis catalog URLs point at — a
	// top-level asset path on the GHES host, not part of /api/v3.
	s.route("GET /images/icons/emoji/{path...}", s.handleGHEmojiImage)
}

// handleGHEmojis serves the full GitHub emoji catalog with image URLs
// pointing at this server, matching GitHub Enterprise Server behavior of
// serving emoji assets from the instance host.
func (s *Server) handleGHEmojis(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r) + "/images/icons/emoji/"
	out := make(map[string]string, 2048)
	for _, line := range strings.Split(strings.TrimSpace(gemojiCatalog), "\n") {
		name, path, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		out[name] = base + path
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGHZen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(zenQuotes[rand.Intn(len(zenQuotes))]))
}

func (s *Server) handleGHOctocat(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("s")
	if !octocatSpeechRe.MatchString(text) {
		text = zenQuotes[rand.Intn(len(zenQuotes))]
	}
	w.Header().Set("Content-Type", "application/octocat-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(octocatArt(text)))
}

// octocatArt renders GitHub's octocat ASCII art with the given text in the
// speech bubble, byte-identical to GET https://api.github.com/octocat.
func octocatArt(text string) string {
	inner := len(text) + 2
	bottomTail := inner - 4
	if bottomTail < 0 {
		bottomTail = 0
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("               MMM.           .MMM\n")
	b.WriteString("               MMMMMMMMMMMMMMMMMMM\n")
	b.WriteString("               MMMMMMMMMMMMMMMMMMM      " + strings.Repeat("_", inner) + "\n")
	b.WriteString("              MMMMMMMMMMMMMMMMMMMMM    |" + strings.Repeat(" ", inner) + "|\n")
	b.WriteString("             MMMMMMMMMMMMMMMMMMMMMMM   | " + text + " |\n")
	b.WriteString("            MMMMMMMMMMMMMMMMMMMMMMMM   |_   " + strings.Repeat("_", bottomTail) + "|\n")
	b.WriteString("            MMMM::- -:::::::- -::MMMM    |/\n")
	b.WriteString("             MM~:~ 00~:::::~ 00~:~MM\n")
	b.WriteString("        .. MMMMM::.00:::+:::.00::MMMMM ..\n")
	b.WriteString("              .MM::::: ._. :::::MM.\n")
	b.WriteString("                 MMMM;:::::;MMMM\n")
	b.WriteString("          -MM        MMMMMMM\n")
	b.WriteString("          ^  M+     MMMMMMMMM\n")
	b.WriteString("              MMMMMMM MM MM MM\n")
	b.WriteString("                   MM MM MM MM\n")
	b.WriteString("                   MM MM MM MM\n")
	b.WriteString("                .~~MM~MM~MM~MM~~.\n")
	b.WriteString("             ~~~~MM:~MM~~~MM~:MM~~~~\n")
	b.WriteString("            ~~~~~~==~==~~~==~==~~~~~~\n")
	b.WriteString("             ~~~~~~==~==~==~==~~~~~~\n")
	b.WriteString("                 :~==~==~==~==~~\n")
	return b.String()
}

// handleGHAPIVersions serves GET /versions — the list of supported GitHub
// REST API calendar versions.
func (s *Server) handleGHAPIVersions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []string{"2022-11-28"})
}

// handleGHCredentialsRevoke implements POST /credentials/revoke: it revokes
// every supplied credential that matches a live token in the store (personal
// access tokens, OAuth user-to-server tokens, refresh tokens, and GitHub App
// installation tokens). Real GitHub processes the batch asynchronously and
// answers 202 without a body; unknown credentials are silently accepted, the
// same as GitHub, so a caller cannot probe which tokens exist.
func (s *Server) handleGHCredentialsRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Credentials []string `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if len(req.Credentials) < 1 || len(req.Credentials) > 1000 {
		writeGHValidationError(w, "Credentials", "credentials", "invalid")
		return
	}
	s.store.RevokeCredentials(req.Credentials)
	w.WriteHeader(http.StatusAccepted)
}

// RevokeCredentials deletes every listed credential from all token stores
// and returns how many were revoked.
func (st *Store) RevokeCredentials(credentials []string) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	revoked := 0
	for _, c := range credentials {
		if _, ok := st.Tokens[c]; ok {
			delete(st.Tokens, c)
			if st.persist != nil {
				st.persist.MustDelete("tokens", c)
			}
			revoked++
		}
		if _, ok := st.UserToServerTokens[c]; ok {
			delete(st.UserToServerTokens, c)
			if st.persist != nil {
				st.persist.MustDelete("user_to_server_tokens", c)
			}
			revoked++
		}
		if _, ok := st.RefreshTokens[c]; ok {
			delete(st.RefreshTokens, c)
			if st.persist != nil {
				st.persist.MustDelete("refresh_tokens", c)
			}
			revoked++
		}
		if _, ok := st.InstallationTokens[c]; ok {
			delete(st.InstallationTokens, c)
			if st.persist != nil {
				st.persist.MustDelete("installation_tokens", c)
			}
			revoked++
		}
	}
	return revoked
}
