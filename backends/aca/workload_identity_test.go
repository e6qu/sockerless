package aca

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
)

// With a managed identity configured, a Container App and a Job run with it
// and declare it as the credential for the registry their images live in;
// without one they declare nothing, and the platform pulls anonymously.
func TestSpecsCarryTheWorkloadIdentityAndRegistryCredential(t *testing.T) {
	s := newServerForAppSpec(t)
	s.config.ManagedIdentityID = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/sockerless"
	s.config.ACRName = "myreg"
	ci := demoAppContainer("abcdef012345abcdef", "/webapp", "myreg.azurecr.io/app:v1")

	app, err := s.buildAppSpec(context.Background(), []containerInput{ci})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.buildJobSpec(context.Background(), []containerInput{ci})
	if err != nil {
		t.Fatal(err)
	}
	for name, identity := range map[string]*armappcontainers.ManagedServiceIdentity{"app": app.Identity, "job": job.Identity} {
		if identity == nil || identity.Type == nil || *identity.Type != armappcontainers.ManagedServiceIdentityTypeUserAssigned {
			t.Fatalf("%s identity = %v, want the user-assigned identity", name, identity)
		}
		if _, ok := identity.UserAssignedIdentities[s.config.ManagedIdentityID]; !ok {
			t.Fatalf("%s identity does not name %s", name, s.config.ManagedIdentityID)
		}
	}
	for name, registries := range map[string][]*armappcontainers.RegistryCredentials{"app": app.Properties.Configuration.Registries, "job": job.Properties.Configuration.Registries} {
		if len(registries) != 1 || *registries[0].Server != "myreg.azurecr.io" || *registries[0].Identity != s.config.ManagedIdentityID {
			t.Fatalf("%s registries = %v, want the registry pulled as the identity", name, registries)
		}
	}

	s.config.ManagedIdentityID = ""
	app, _ = s.buildAppSpec(context.Background(), []containerInput{ci})
	job, _ = s.buildJobSpec(context.Background(), []containerInput{ci})
	if app.Identity != nil || job.Identity != nil || app.Properties.Configuration.Registries != nil || job.Properties.Configuration.Registries != nil {
		t.Fatal("without a managed identity the specs declare no identity and no registry credential")
	}
}
