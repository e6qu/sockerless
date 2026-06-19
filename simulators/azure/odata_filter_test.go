package main

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

type odItem struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Tier  int    `json:"tier"`
}

func TestAzureODataFilter(t *testing.T) {
	items := []odItem{
		{Name: "alpha", State: "ACTIVE", Tier: 1},
		{Name: "beta", State: "STOPPED", Tier: 3},
		{Name: "alefel", State: "ACTIVE", Tier: 2},
	}
	apply := func(filter string) []odItem {
		q := url.Values{"$filter": {filter}}
		r := httptest.NewRequest("GET", "/x?"+q.Encode(), nil)
		return azureApplyListQuery(items, r)
	}

	cases := []struct {
		filter string
		want   int
	}{
		{"name eq 'alpha'", 1},
		{"name ne 'alpha'", 2},
		{"state eq 'ACTIVE'", 2},
		{"tier ge 2", 2},
		{"tier gt 1 and state eq 'ACTIVE'", 1},
		{"name eq 'alpha' or name eq 'beta'", 2},
		{"startswith(name, 'al')", 2},
		{"endswith(name, 'a')", 2}, // alpha, beta
		{"contains(name, 'lef')", 1},
		{"not (state eq 'ACTIVE')", 1},
		{"substringof('eta', name)", 1},
	}
	for _, tc := range cases {
		if got := apply(tc.filter); len(got) != tc.want {
			t.Errorf("$filter=%q → %d items, want %d", tc.filter, len(got), tc.want)
		}
	}

	// $orderby
	q := url.Values{"$orderby": {"tier desc"}}
	r := httptest.NewRequest("GET", "/x?"+q.Encode(), nil)
	got := azureApplyListQuery(items, r)
	if got[0].Tier != 3 || got[2].Tier != 1 {
		t.Errorf("$orderby=tier desc → %v", got)
	}
}
