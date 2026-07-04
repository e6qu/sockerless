package bleephub

import (
	_ "embed"
	"net/http"
)

// GitHub's codes-of-conduct catalog. The two entries and their body texts
// are the real documents GitHub serves from /codes_of_conduct.

//go:embed coc_contributor_covenant.md
var cocContributorCovenantBody string

//go:embed coc_citizen_code_of_conduct.md
var cocCitizenCodeOfConductBody string

type codeOfConduct struct {
	key  string
	name string
	body string
}

// codesOfConductCatalog is ordered the way GitHub lists it (alphabetical by
// key).
var codesOfConductCatalog = []codeOfConduct{
	{key: "citizen_code_of_conduct", name: "Citizen Code of Conduct", body: cocCitizenCodeOfConductBody},
	{key: "contributor_covenant", name: "Contributor Covenant", body: cocContributorCovenantBody},
}

func (s *Server) registerGHCodesOfConductRoutes() {
	s.route("GET /api/v3/codes_of_conduct", s.handleListCodesOfConduct)
	s.route("GET /api/v3/codes_of_conduct/{key}", s.handleGetCodeOfConduct)
}

// codeOfConductToJSON renders the spec `code-of-conduct` shape. The list
// endpoint omits body (matching GitHub); the get-by-key endpoint includes it.
func codeOfConductToJSON(c codeOfConduct, baseURL string, withBody bool) map[string]interface{} {
	out := map[string]interface{}{
		"key":      c.key,
		"name":     c.name,
		"url":      baseURL + "/api/v3/codes_of_conduct/" + c.key,
		"html_url": nil,
	}
	if withBody {
		out["body"] = c.body
	}
	return out
}

func (s *Server) handleListCodesOfConduct(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(codesOfConductCatalog))
	for _, c := range codesOfConductCatalog {
		out = append(out, codeOfConductToJSON(c, base, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCodeOfConduct(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	for _, c := range codesOfConductCatalog {
		if c.key == key {
			writeJSON(w, http.StatusOK, codeOfConductToJSON(c, s.baseURL(r), true))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}
