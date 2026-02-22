package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// StorageAccount represents an Azure Storage Account.
type StorageAccount struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Type       string                   `json:"type"`
	Location   string                   `json:"location"`
	Kind       string                   `json:"kind,omitempty"`
	Sku        *StorageSku              `json:"sku,omitempty"`
	Tags       map[string]string        `json:"tags,omitempty"`
	Properties StorageAccountProperties `json:"properties"`
}

// StorageSku holds the SKU for a storage account.
type StorageSku struct {
	Name string `json:"name"`
	Tier string `json:"tier,omitempty"`
}

// StorageAccountProperties holds the properties of a storage account.
type StorageAccountProperties struct {
	ProvisioningState string                    `json:"provisioningState"`
	PrimaryEndpoints  *StoragePrimaryEndpoints  `json:"primaryEndpoints,omitempty"`
	CreationTime      string                    `json:"creationTime,omitempty"`
}

// StoragePrimaryEndpoints holds the primary endpoints for a storage account.
type StoragePrimaryEndpoints struct {
	File  string `json:"file,omitempty"`
	Blob  string `json:"blob,omitempty"`
	Table string `json:"table,omitempty"`
	Queue string `json:"queue,omitempty"`
	Web   string `json:"web,omitempty"`
	Dfs   string `json:"dfs,omitempty"`
}

// FileShare represents an Azure File Share.
type FileShare struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Type       string              `json:"type"`
	Etag       string              `json:"etag,omitempty"`
	Properties FileShareProperties `json:"properties"`
}

// FileShareProperties holds the properties of a file share.
type FileShareProperties struct {
	ShareQuota       int    `json:"shareQuota,omitempty"`
	AccessTier       string `json:"accessTier,omitempty"`
	EnabledProtocols string `json:"enabledProtocols,omitempty"`
	ProvisioningState string `json:"provisioningState,omitempty"`
	LastModifiedTime string `json:"lastModifiedTime,omitempty"`
	LeaseStatus      string `json:"leaseStatus,omitempty"`
	LeaseState       string `json:"leaseState,omitempty"`
}

func registerAzureFiles(srv *sim.Server) {
	storageAccounts := sim.NewStateStore[StorageAccount]()
	fileShares := sim.NewStateStore[FileShare]()
	fileData := sim.NewStateStore[[]byte]()
	// dataPlaneShares tracks shares created via either ARM or data-plane APIs
	// so that the data-plane middleware can return 404 for non-existent shares.
	dataPlaneShares := sim.NewStateStore[bool]()

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Storage"

	// PUT - Create or update storage account
	srv.HandleFunc("PUT "+armBase+"/storageAccounts/{accountName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "accountName")

		var req StorageAccount
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", sub, rg, name)

		storageAccounts.Get(resourceID)

		kind := req.Kind
		if kind == "" {
			kind = "StorageV2"
		}

		sku := req.Sku
		if sku == nil {
			sku = &StorageSku{Name: "Standard_LRS", Tier: "Standard"}
		}

		// Derive endpoints that use subdomain format:
		//   https://{accountName}.blob.localhost:4568/
		// The azurerm provider parses these URLs to extract the account
		// name (checking the domain suffix against suffixes.storage from
		// the metadata endpoint). Data-plane requests are routed to the
		// simulator via dnsmasq (resolving *.localhost → 127.0.0.1) and
		// handled by the storage data-plane middleware.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		host := r.Host
		// Separate hostname and port
		hostname := host
		portSuffix := ""
		if i := strings.LastIndex(hostname, ":"); i >= 0 {
			portSuffix = hostname[i:]
			hostname = hostname[:i]
		}

		acct := StorageAccount{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.Storage/storageAccounts",
			Location: req.Location,
			Kind:     kind,
			Sku:      sku,
			Tags:     req.Tags,
			Properties: StorageAccountProperties{
				ProvisioningState: "Succeeded",
				PrimaryEndpoints: &StoragePrimaryEndpoints{
					File:  fmt.Sprintf("%s://%s.file.%s%s/", scheme, name, hostname, portSuffix),
					Blob:  fmt.Sprintf("%s://%s.blob.%s%s/", scheme, name, hostname, portSuffix),
					Table: fmt.Sprintf("%s://%s.table.%s%s/", scheme, name, hostname, portSuffix),
					Queue: fmt.Sprintf("%s://%s.queue.%s%s/", scheme, name, hostname, portSuffix),
					Web:   fmt.Sprintf("%s://%s.web.%s%s/", scheme, name, hostname, portSuffix),
					Dfs:   fmt.Sprintf("%s://%s.dfs.%s%s/", scheme, name, hostname, portSuffix),
				},
				CreationTime: time.Now().UTC().Format(time.RFC3339),
			},
		}

		storageAccounts.Put(resourceID, acct)

		// go-azure-sdk expects 200 for create (it treats this as a sync LRO)
		sim.WriteJSON(w, http.StatusOK, acct)
	})

	// GET - Get storage account
	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "accountName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", sub, rg, name)

		acct, ok := storageAccounts.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, acct)
	})

	// DELETE - Delete storage account
	srv.HandleFunc("DELETE "+armBase+"/storageAccounts/{accountName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "accountName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", sub, rg, name)

		if storageAccounts.Delete(resourceID) {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// PUT - Create or update file share
	srv.HandleFunc("PUT "+armBase+"/storageAccounts/{accountName}/fileServices/default/shares/{shareName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		account := sim.PathParam(r, "accountName")
		shareName := sim.PathParam(r, "shareName")

		var req FileShare
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Verify storage account exists
		acctID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", sub, rg, account)
		if _, ok := storageAccounts.Get(acctID); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", account, rg)
			return
		}

		shareID := fmt.Sprintf("%s/fileServices/default/shares/%s", acctID, shareName)

		quota := req.Properties.ShareQuota
		if quota == 0 {
			quota = 5120
		}

		accessTier := req.Properties.AccessTier
		if accessTier == "" {
			accessTier = "TransactionOptimized"
		}

		protocols := req.Properties.EnabledProtocols
		if protocols == "" {
			protocols = "SMB"
		}

		share := FileShare{
			ID:   shareID,
			Name: shareName,
			Type: "Microsoft.Storage/storageAccounts/fileServices/shares",
			Etag: fmt.Sprintf("\"0x%s\"", randomSuffix(16)),
			Properties: FileShareProperties{
				ShareQuota:        quota,
				AccessTier:        accessTier,
				EnabledProtocols:  protocols,
				ProvisioningState: "Succeeded",
				LastModifiedTime:  time.Now().UTC().Format(time.RFC3339),
				LeaseStatus:       "Unlocked",
				LeaseState:        "Available",
			},
		}

		fileShares.Put(shareID, share)
		// Also register in data-plane store so the middleware knows it exists
		dataPlaneShares.Put(account+"/"+shareName, true)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, share)
	})

	// GET - Get file share
	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}/fileServices/default/shares/{shareName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		account := sim.PathParam(r, "accountName")
		shareName := sim.PathParam(r, "shareName")

		shareID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/fileServices/default/shares/%s",
			sub, rg, account, shareName)

		share, ok := fileShares.Get(shareID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The file share '%s' was not found.", shareName)
			return
		}

		sim.WriteJSON(w, http.StatusOK, share)
	})

	// DELETE - Delete file share
	srv.HandleFunc("DELETE "+armBase+"/storageAccounts/{accountName}/fileServices/default/shares/{shareName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		account := sim.PathParam(r, "accountName")
		shareName := sim.PathParam(r, "shareName")

		shareID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/fileServices/default/shares/%s",
			sub, rg, account, shareName)

		if fileShares.Delete(shareID) {
			dataPlaneShares.Delete(account + "/" + shareName)
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// GET - List file shares
	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}/fileServices/default/shares", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		account := sim.PathParam(r, "accountName")

		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/fileServices/default/shares/",
			sub, rg, account)

		filtered := fileShares.Filter(func(s FileShare) bool {
			return strings.HasPrefix(s.ID, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": filtered,
		})
	})

	// POST - List storage account keys (azurerm provider calls this after creating a storage account)
	srv.HandleFunc("POST "+armBase+"/storageAccounts/{accountName}/listKeys", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "accountName")

		acctID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", sub, rg, name)
		if _, ok := storageAccounts.Get(acctID); !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"keys": []map[string]any{
				{"keyName": "key1", "value": "dGVzdGtleTEK", "permissions": "FULL"},
				{"keyName": "key2", "value": "dGVzdGtleTIK", "permissions": "FULL"},
			},
		})
	})

	// GET - List storage accounts at subscription level (azurerm provider checks name uniqueness)
	srv.HandleFunc("GET /subscriptions/{subscriptionId}/providers/Microsoft.Storage/storageAccounts", func(w http.ResponseWriter, r *http.Request) {
		all := storageAccounts.Filter(func(sa StorageAccount) bool { return true })
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": all,
		})
	})

	// Storage service properties handlers — azurerm provider polls these after creating a storage account.
	// Each service (file, blob, queue, table) has a default configuration endpoint.
	storageServiceHandler := func(serviceName, serviceType string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			sub := sim.PathParam(r, "subscriptionId")
			rg := sim.PathParam(r, "resourceGroupName")
			name := sim.PathParam(r, "accountName")

			acctID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s", sub, rg, name)
			if _, ok := storageAccounts.Get(acctID); !ok {
				sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
					"The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", name, rg)
				return
			}

			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"id":   fmt.Sprintf("%s/%s/default", acctID, serviceName),
				"name": "default",
				"type": serviceType,
				"properties": map[string]any{
					"cors": map[string]any{
						"corsRules": []any{},
					},
				},
			})
		}
	}

	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}/fileServices/default",
		storageServiceHandler("fileServices", "Microsoft.Storage/storageAccounts/fileServices"))
	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}/blobServices/default",
		storageServiceHandler("blobServices", "Microsoft.Storage/storageAccounts/blobServices"))
	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}/queueServices/default",
		storageServiceHandler("queueServices", "Microsoft.Storage/storageAccounts/queueServices"))
	srv.HandleFunc("GET "+armBase+"/storageAccounts/{accountName}/tableServices/default",
		storageServiceHandler("tableServices", "Microsoft.Storage/storageAccounts/tableServices"))

	// --- Storage Data Plane Middleware ---
	// The azurerm provider makes data-plane calls to the storage account's
	// primaryEndpoints using subdomain-format URLs like:
	//   https://{accountName}.blob.localhost:4568/?restype=service&comp=properties
	// These requests arrive at the simulator with a Host header containing
	// the subdomain. The middleware intercepts them based on the Host pattern
	// {accountName}.{service}.localhost and returns mock XML responses.
	//
	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			// Strip port
			hostname := host
			if i := strings.LastIndex(hostname, ":"); i >= 0 {
				hostname = hostname[:i]
			}

			// Check for {account}.{service}.localhost pattern
			if strings.Count(hostname, ".") >= 2 && strings.HasSuffix(hostname, ".localhost") {
				prefix := strings.TrimSuffix(hostname, ".localhost")
				parts := strings.SplitN(prefix, ".", 2)
				if len(parts) == 2 {
					accountName := parts[0]
					serviceType := parts[1]
					handleStorageDataPlane(w, r, serviceType, accountName, dataPlaneShares)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	})

	// Keep fileData available for potential future file data-plane operations
	_ = fileData
}

// handleStorageDataPlane returns mock XML responses for Azure Storage data plane
// requests. The azurerm provider reads service properties after creating storage
// accounts (static website for blob, share retention for file, CORS for queue).
func handleStorageDataPlane(w http.ResponseWriter, r *http.Request, serviceType, accountName string, shares *sim.StateStore[bool]) {
	restype := r.URL.Query().Get("restype")
	comp := r.URL.Query().Get("comp")

	// Service properties: GET ?restype=service&comp=properties
	if restype == "service" && comp == "properties" {
		w.Header().Set("Content-Type", "application/xml")
		switch serviceType {
		case "blob":
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><StorageServiceProperties><StaticWebsite><Enabled>false</Enabled></StaticWebsite><Cors /><DefaultServiceVersion>2021-12-02</DefaultServiceVersion></StorageServiceProperties>`)
		case "file":
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><StorageServiceProperties><Cors /><ShareDeleteRetentionPolicy><Enabled>false</Enabled><Days>7</Days></ShareDeleteRetentionPolicy></StorageServiceProperties>`)
		default:
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><StorageServiceProperties><Cors /></StorageServiceProperties>`)
		}
		return
	}

	// File share operations: ?restype=share
	if restype == "share" && serviceType == "file" {
		shareName := strings.TrimPrefix(r.URL.Path, "/")
		shareKey := accountName + "/" + shareName

		// comp=acl: Set/Get ACLs (always succeeds)
		if comp == "acl" {
			w.Header().Set("Content-Type", "application/xml")
			if r.Method == http.MethodPut {
				w.WriteHeader(http.StatusOK)
			} else {
				// GET returns empty ACLs
				fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><SignedIdentifiers />`)
			}
			return
		}

		if r.Method == http.MethodPut {
			shares.Put(shareKey, true)
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusCreated)
			return
		}

		if r.Method == http.MethodDelete {
			shares.Delete(shareKey)
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// GET/HEAD — only return 200 if the share was created
		if _, ok := shares.Get(shareKey); !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><Error><Code>ShareNotFound</Code><Message>The specified share does not exist.</Message></Error>`)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("x-ms-share-quota", "5120")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
		} else {
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><Share><Properties><Quota>5120</Quota></Properties></Share>`)
		}
		return
	}

	// Default: 200 OK (some operations just need a success response)
	w.WriteHeader(http.StatusOK)
}
