package azurecommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/rs/zerolog"
)

// armClientOptions points an ARM client at the simulator the way the ACR
// registry provisioning above does — the endpoint is the only coordinate.
func armClientOptions() *arm.ClientOptions {
	return &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
		Cloud: cloud.Configuration{Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
			cloud.ResourceManager: {Endpoint: acrSimURL, Audience: "https://management.azure.com/"},
		}},
		InsecureAllowCredentialWithHTTP: true,
	}}
}

// The build-context blob client authenticates with the storage account's own
// shared key, read through the Azure Resource Manager the way an
// administrator reads it. The simulator enforces storage authorization on
// every data-plane request, so an unauthenticated client is not a shortcut
// that happens to work — it is refused; this proves the signed client's
// round trip end to end: resolve the advertised blob endpoint, list the
// account keys, sign, upload, and read the bytes back.
func TestACRBuildBlobClientAuthenticatesWithTheAccountSharedKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// The blob endpoint is advertised on a per-account hostname under
	// .shim.localhost — a deployment resolves those through its own DNS (the
	// Linux test harness's dnsmasq, systemd-resolved on CI, both of which
	// answer any *.localhost with loopback). macOS's resolver will not, and
	// installing that mapping needs root — a host capability no test can
	// provide, the same class as a missing kernel capability. Linux never
	// skips.
	if _, err := net.LookupHost("probe.blob.shim.localhost"); err != nil {
		if runtime.GOOS == "linux" {
			t.Fatalf("Linux must resolve *.shim.localhost to loopback: %v", err)
		}
		t.Skipf("host resolver does not map *.shim.localhost to loopback (GOOS %s): %v", runtime.GOOS, err)
	}

	const accountName = "acrbuildblobsa"
	accounts, err := armstorage.NewAccountsClient(acrSimSubscription, acrEntraCredential, armClientOptions())
	if err != nil {
		t.Fatalf("storage accounts client: %v", err)
	}
	poller, err := accounts.BeginCreate(ctx, acrSimResourceGroup, accountName, armstorage.AccountCreateParameters{
		Location: to.Ptr("eastus"),
		SKU:      &armstorage.SKU{Name: to.Ptr(armstorage.SKUNameStandardLRS)},
		Kind:     to.Ptr(armstorage.KindStorageV2),
	}, nil)
	if err != nil {
		t.Fatalf("create storage account: %v", err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		t.Fatalf("storage account never provisioned: %v", err)
	}

	service, err := NewACRBuildService(acrEntraCredential, acrSimSubscription, acrSimResourceGroup,
		acrSimRegistryName, accountName, "", acrSimURL, zerolog.Nop())
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	if err := service.ensureBlobClient(ctx); err != nil {
		t.Fatalf("resolve the blob client through ARM: %v", err)
	}

	if _, err := service.blobClient.CreateContainer(ctx, service.containerName, nil); err != nil {
		t.Fatalf("create build-context container: %v", err)
	}
	payload := []byte("build context payload " + fmt.Sprint(time.Now().UnixNano()))
	if _, err := service.blobClient.UploadBuffer(ctx, service.containerName, "context.tar.gz", payload, nil); err != nil {
		t.Fatalf("upload through the shared-key client: %v", err)
	}
	download, err := service.blobClient.DownloadStream(ctx, service.containerName, "context.tar.gz", nil)
	if err != nil {
		t.Fatalf("download through the shared-key client: %v", err)
	}
	defer download.Body.Close()
	got, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("read downloaded blob: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("the blob round trip must return the uploaded bytes; got %d bytes, want %d", len(got), len(payload))
	}
}
