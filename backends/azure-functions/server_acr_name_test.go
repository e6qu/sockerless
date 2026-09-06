package azf

import "testing"

// The registry name is the login server's first label, whatever domain the
// cloud advertises the registry under.
func TestAZFACRNameFromLoginServer(t *testing.T) {
	cases := map[string]string{
		"sockerlessacr.azurecr.io":                  "sockerlessacr",
		"https://sockerlessacr.azurecr.io/":         "sockerlessacr",
		"sockerlessacr.azurecr.us":                  "sockerlessacr",
		"sockerlessacr.eastus.data.azurecr.io":      "sockerlessacr",
		"sockerlessacr.azurecr.localhost:4568":      "sockerlessacr",
		"sockerlessacr":                             "sockerlessacr",
		"http://sockerlessacr.azurecr.localhost:80": "sockerlessacr",
	}
	for in, want := range cases {
		if got := azfACRName(in); got != want {
			t.Errorf("azfACRName(%q) = %q, want %q", in, got, want)
		}
	}
}
