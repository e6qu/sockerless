package core

import (
	"errors"
	"testing"

	"github.com/sockerless/api"
)

func TestMapCloudError(t *testing.T) {
	patterns := CloudErrorPatterns{
		NotFound: []string{"not found", "ResourceNotFoundException"},
		Conflict: []string{"already exists"},
		Invalid:  []string{"ValidationException"},
	}
	if MapCloudError(nil, "container", "c", patterns) != nil {
		t.Fatal("nil in, nil out")
	}
	var nf *api.NotFoundError
	if err := MapCloudError(errors.New("ResourceNotFoundException: no such task"), "container", "abc", patterns); !errors.As(err, &nf) || nf.ID != "abc" {
		t.Fatalf("not-found = %v", err)
	}
	var conflict *api.ConflictError
	if err := MapCloudError(errors.New("bucket already exists"), "volume", "v", patterns); !errors.As(err, &conflict) {
		t.Fatalf("conflict = %v", err)
	}
	var invalid *api.InvalidParameterError
	if err := MapCloudError(errors.New("ValidationException: bad cpu"), "container", "c", patterns); !errors.As(err, &invalid) {
		t.Fatalf("invalid = %v", err)
	}
	var server *api.ServerError
	if err := MapCloudError(errors.New("throttled"), "container", "c", patterns); !errors.As(err, &server) || server.Message != "throttled" {
		t.Fatalf("unmatched = %v", err)
	}
	if ContainsAny("abc", "x", "b") != true || ContainsAny("abc") {
		t.Fatal("ContainsAny")
	}
}
