package bleephub

import (
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// languageForFilename maps a file name to its Linguist-style language. The
// mapping is intentionally small but covers the common cases; GitHub's API
// returns byte totals only, so an approximate mapping is acceptable for tests
// and UI color labels.
func languageForFilename(name string) (string, bool) {
	ext := strings.ToLower(path.Ext(name))
	if ext == "" {
		base := path.Base(name)
		if base == "Makefile" || base == "makefile" {
			return "Makefile", true
		}
		return "", false
	}

	// Strip leading dot.
	ext = ext[1:]

	lang, ok := extensionLanguageMap[ext]
	return lang, ok
}

var extensionLanguageMap = map[string]string{
	"go":         "Go",
	"js":         "JavaScript",
	"mjs":        "JavaScript",
	"cjs":        "JavaScript",
	"jsx":        "JSX",
	"ts":         "TypeScript",
	"tsx":        "TSX",
	"py":         "Python",
	"pyw":        "Python",
	"rb":         "Ruby",
	"java":       "Java",
	"class":      "Java",
	"c":          "C",
	"h":          "C",
	"cpp":        "C++",
	"cc":         "C++",
	"cxx":        "C++",
	"hpp":        "C++",
	"cs":         "C#",
	"php":        "PHP",
	"swift":      "Swift",
	"kt":         "Kotlin",
	"kts":        "Kotlin",
	"rs":         "Rust",
	"scala":      "Scala",
	"r":          "R",
	"m":          "Objective-C",
	"mm":         "Objective-C++",
	"sh":         "Shell",
	"bash":       "Shell",
	"zsh":        "Shell",
	"fish":       "Shell",
	"ps1":        "PowerShell",
	"psm1":       "PowerShell",
	"pl":         "Perl",
	"pm":         "Perl",
	"lua":        "Lua",
	"vim":        "Vim Script",
	"elm":        "Elm",
	"erl":        "Erlang",
	"hrl":        "Erlang",
	"ex":         "Elixir",
	"exs":        "Elixir",
	"clj":        "Clojure",
	"cljs":       "ClojureScript",
	"hs":         "Haskell",
	"lhs":        "Haskell",
	"ml":         "OCaml",
	"mli":        "OCaml",
	"fs":         "F#",
	"fsx":        "F#",
	"fsi":        "F#",
	"dart":       "Dart",
	"flutter":    "Dart",
	"html":       "HTML",
	"htm":        "HTML",
	"xhtml":      "HTML",
	"css":        "CSS",
	"scss":       "SCSS",
	"sass":       "Sass",
	"less":       "Less",
	"json":       "JSON",
	"yaml":       "YAML",
	"yml":        "YAML",
	"toml":       "TOML",
	"xml":        "XML",
	"svg":        "SVG",
	"md":         "Markdown",
	"markdown":   "Markdown",
	"rst":        "reStructuredText",
	"txt":        "Text",
	"dockerfile": "Dockerfile",
	"sql":        "SQL",
	"graphql":    "GraphQL",
	"proto":      "Protocol Buffers",
	"tf":         "HCL",
	"hcl":        "HCL",
	"sol":        "Solidity",
	"vue":        "Vue",
	"svelte":     "Svelte",
	"rmd":        "RMarkdown",
	"ipynb":      "Jupyter Notebook",
	"cmake":      "CMake",
	"nix":        "Nix",
	"gradle":     "Gradle",
	"groovy":     "Groovy",
	"jl":         "Julia",
	"cr":         "Crystal",
	"d":          "D",
	"nim":        "Nim",
	"v":          "V",
	"zig":        "Zig",
	"wasm":       "WebAssembly",
	"wat":        "WebAssembly",
}

// linguistVendoredPaths matches directory/file names that GitHub Linguist
// excludes from language statistics. Only exact segment matches are needed for
// the simple paths bleephub repositories contain.
var linguistVendoredPaths = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".git":         true,
}

func isVendoredPath(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if linguistVendoredPaths[seg] {
			return true
		}
	}
	return false
}

// computeRepoLanguages walks the repository's default branch tree and returns
// a map of language name to byte size, sorted by descending size.
func (st *Store) computeRepoLanguages(repo *Repo) map[string]int64 {
	st.mu.RLock()
	stor := st.GitStorages[repo.FullName]
	st.mu.RUnlock()

	if stor == nil {
		return nil
	}

	ref, err := stor.Reference(plumbing.NewBranchReferenceName(repo.DefaultBranch))
	if err != nil {
		return nil
	}

	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		return nil
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil
	}

	counts := map[string]int64{}
	_ = tree.Files().ForEach(func(f *object.File) error {
		if isVendoredPath(f.Name) {
			return nil
		}
		if lang, ok := languageForFilename(f.Name); ok {
			counts[lang] += f.Size
		}
		return nil
	})

	if len(counts) == 0 {
		return nil
	}
	return counts
}

func (s *Server) handleGetRepoLanguages(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")

	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	counts := s.store.computeRepoLanguages(repo)
	if counts == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}

	// GitHub sorts languages by byte count descending.
	type pair struct {
		lang  string
		bytes int64
	}
	pairs := make([]pair, 0, len(counts))
	for lang, n := range counts {
		pairs = append(pairs, pair{lang, n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].bytes != pairs[j].bytes {
			return pairs[i].bytes > pairs[j].bytes
		}
		return pairs[i].lang < pairs[j].lang
	})

	out := make(map[string]interface{}, len(pairs))
	for _, p := range pairs {
		out[p.lang] = p.bytes
	}
	writeJSON(w, http.StatusOK, out)
}
