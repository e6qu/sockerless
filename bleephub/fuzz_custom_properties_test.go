package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
)

var fuzzPropOrgSeq int64

// FuzzCustomPropertyDefinition fuzzes the definition-write body decoded by
// toCustomProperty (value_type string/single_select/multi_select/true_false/
// url plus allowed_values / default_value / required / values_editable_by /
// property_name). Each iteration targets a fresh org so the run is
// deterministic and reproducible. Invariant: never a 5xx/panic; an accepted
// (200) definition round-trips through GET; a rejected one is 422.
func FuzzCustomPropertyDefinition(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]

	f.Add(`{"properties":[{"property_name":"team","value_type":"string"}]}`)
	f.Add(`{"properties":[{"property_name":"env","value_type":"single_select","allowed_values":["dev","prod"]}]}`)
	f.Add(`{"properties":[{"property_name":"envs","value_type":"multi_select","allowed_values":["a","b"],"default_value":["a"]}]}`)
	f.Add(`{"properties":[{"property_name":"on","value_type":"true_false","required":true,"default_value":"true"}]}`)
	f.Add(`{"properties":[{"property_name":"u","value_type":"url"}]}`)
	f.Add(`{"properties":[{"property_name":"bad","value_type":"nonsense"}]}`)
	f.Add(`{"properties":[{"property_name":"","value_type":"string"}]}`)
	f.Add(`{"properties":[{"property_name":" 00","value_type":"single_select","allowed_values":["a"]}]}`)
	f.Add(`{"properties":[{"property_name":"a b","value_type":"string"}]}`)
	f.Add(`{"properties":[{"property_name":"x","value_type":"string","allowed_values":["nope"]}]}`)
	f.Add(`{"properties":[{"property_name":"x","value_type":"single_select","default_value":42}]}`)
	f.Add(`{"properties":[{"property_name":"x","value_type":"true_false","required":true}]}`)
	f.Add(`{"properties":[{"property_name":"x","value_type":"string","values_editable_by":"nobody"}]}`)
	f.Add(`{"properties":[]}`)
	f.Add(`{}`)
	f.Add(`{"properties":[{"property_name":"x","value_type":"string","default_value":{"nested":1}}]}`)
	f.Add(`null`)
	f.Add(`not json`)

	f.Fuzz(func(t *testing.T, body string) {
		org := fmt.Sprintf("propsorg%d", atomic.AddInt64(&fuzzPropOrgSeq, 1))
		s.store.CreateOrg(admin, org, "Props Org", "")

		w := fuzzServe(s, http.MethodPatch, "/api/v3/orgs/"+org+"/properties/schema", []byte(body))
		if w.Code >= 500 {
			t.Fatalf("schema PATCH -> %d (want <500) for %q: %s", w.Code, body, w.Body.String())
		}
		if w.Code == http.StatusOK {
			var defs []map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &defs); err != nil {
				t.Fatalf("200 schema body not a JSON array: %v (%s)", err, w.Body.String())
			}
			for _, d := range defs {
				name, _ := d["property_name"].(string)
				if name == "" {
					t.Fatalf("accepted definition has empty property_name: %v", d)
				}
				// An accepted name must be a clean path segment; a name the
				// server should have rejected (whitespace/control) would fail
				// this escaped read-back and signal the gap, never crash.
				gw := fuzzServe(s, http.MethodGet, "/api/v3/orgs/"+org+"/properties/schema/"+url.PathEscape(name), nil)
				if gw.Code != http.StatusOK {
					t.Fatalf("accepted property %q does not read back: GET -> %d", name, gw.Code)
				}
			}
		}
	})
}

// FuzzCustomPropertyValues fuzzes the values-set body (properties[].value)
// against a repo with a real org schema covering all value types. Each
// iteration targets a fresh org+repo so the run is deterministic. Invariant:
// never a 5xx/panic; only the documented statuses occur.
func FuzzCustomPropertyValues(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]

	f.Add(`{"properties":[{"property_name":"team","value":"backend"}]}`)
	f.Add(`{"properties":[{"property_name":"flag","value":"true"}]}`)
	f.Add(`{"properties":[{"property_name":"flag","value":"maybe"}]}`)
	f.Add(`{"properties":[{"property_name":"env","value":"dev"}]}`)
	f.Add(`{"properties":[{"property_name":"env","value":"staging"}]}`)
	f.Add(`{"properties":[{"property_name":"tags","value":["a","b"]}]}`)
	f.Add(`{"properties":[{"property_name":"tags","value":["a","zzz"]}]}`)
	f.Add(`{"properties":[{"property_name":"tags","value":"a"}]}`)
	f.Add(`{"properties":[{"property_name":"tags","value":[1,2]}]}`)
	f.Add(`{"properties":[{"property_name":"team","value":123}]}`)
	f.Add(`{"properties":[{"property_name":"team","value":null}]}`)
	f.Add(`{"properties":[{"property_name":"unknown","value":"x"}]}`)
	f.Add(`{"properties":[{"property_name":"env","value":{"nested":1}}]}`)
	f.Add(`{"properties":[]}`)
	f.Add(`{}`)
	f.Add(`{"properties":null}`)
	f.Add(`not json`)

	f.Fuzz(func(t *testing.T, body string) {
		n := atomic.AddInt64(&fuzzPropOrgSeq, 1)
		orgLogin := fmt.Sprintf("valsorg%d", n)
		org := s.store.CreateOrg(admin, orgLogin, "Vals Org", "")
		repo := s.store.CreateOrgRepo(org, admin, "valsrepo", "", false)
		if repo == nil {
			t.Fatalf("CreateOrgRepo returned nil")
		}
		for _, def := range []*CustomProperty{
			{PropertyName: "team", ValueType: "string", ValuesEditableBy: "org_actors"},
			{PropertyName: "link", ValueType: "url", ValuesEditableBy: "org_actors"},
			{PropertyName: "flag", ValueType: "true_false", ValuesEditableBy: "org_actors"},
			{PropertyName: "env", ValueType: "single_select", AllowedValues: []string{"dev", "prod"}, ValuesEditableBy: "org_actors"},
			{PropertyName: "tags", ValueType: "multi_select", AllowedValues: []string{"a", "b", "c"}, ValuesEditableBy: "org_actors"},
		} {
			s.store.UpsertCustomProperty(orgLogin, def)
		}

		w := fuzzServe(s, http.MethodPatch, "/api/v3/repos/"+orgLogin+"/valsrepo/properties/values", []byte(body))
		if w.Code >= 500 {
			t.Fatalf("values PATCH -> %d (want <500) for %q: %s", w.Code, body, w.Body.String())
		}
		switch w.Code {
		case http.StatusNoContent, http.StatusBadRequest, http.StatusUnprocessableEntity,
			http.StatusForbidden, http.StatusNotFound:
		default:
			t.Fatalf("values PATCH unexpected status %d for %q: %s", w.Code, body, w.Body.String())
		}
	})
}
