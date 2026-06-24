package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Map types

type CMNamespace struct {
	Id          string                 `json:"Id"`
	Arn         string                 `json:"Arn"`
	Name        string                 `json:"Name"`
	Type        string                 `json:"Type"`
	Description string                 `json:"Description,omitempty"`
	Properties  *CMNamespaceProperties `json:"Properties,omitempty"`
	CreateDate  int64                  `json:"CreateDate"`
	// DockerNetworkName is the name of the real Docker user-defined
	// network created when a container-backed registration needs DNS.
	// Pure Cloud Map control-plane resources do not create Docker
	// resources; the network is just the local realization of private
	// namespace DNS for running ECS task containers.
	DockerNetworkName string `json:"DockerNetworkName,omitempty"`
}

type CMNamespaceProperties struct {
	DnsProperties  *CMDnsProperties  `json:"DnsProperties,omitempty"`
	HttpProperties *CMHttpProperties `json:"HttpProperties,omitempty"`
}

type CMHttpProperties struct {
	HttpName string `json:"HttpName"`
}

type CMDnsProperties struct {
	HostedZoneId string `json:"HostedZoneId"`
	SOA          *struct {
		TTL int64 `json:"TTL"`
	} `json:"SOA,omitempty"`
}

type CMService struct {
	Id                string               `json:"Id"`
	Arn               string               `json:"Arn"`
	Name              string               `json:"Name"`
	NamespaceId       string               `json:"NamespaceId"`
	Description       string               `json:"Description,omitempty"`
	DnsConfig         *CMDnsConfig         `json:"DnsConfig,omitempty"`
	HealthCheckConfig *CMHealthCheckConfig `json:"HealthCheckConfig,omitempty"`
	Type              string               `json:"Type,omitempty"`
	CreateDate        int64                `json:"CreateDate"`
	InstanceCount     int                  `json:"InstanceCount"`
}

type CMHealthCheckConfig struct {
	Type             string `json:"Type,omitempty"`
	ResourcePath     string `json:"ResourcePath,omitempty"`
	FailureThreshold int    `json:"FailureThreshold,omitempty"`
}

type CMDnsConfig struct {
	NamespaceId   string        `json:"NamespaceId,omitempty"`
	RoutingPolicy string        `json:"RoutingPolicy,omitempty"`
	DnsRecords    []CMDnsRecord `json:"DnsRecords,omitempty"`
}

type CMDnsRecord struct {
	Type string `json:"Type"`
	TTL  int64  `json:"TTL"`
}

type CMInstance struct {
	Id         string            `json:"Id"`
	Attributes map[string]string `json:"Attributes,omitempty"`
	// CustomHealthStatus, when set, overrides the registered health for an
	// instance whose service uses a HealthCheckCustomConfig (the value
	// UpdateInstanceCustomHealthStatus writes: HEALTHY or UNHEALTHY).
	CustomHealthStatus string `json:"CustomHealthStatus,omitempty"`
}

type CMOperation struct {
	OperationId string            `json:"OperationId"`
	Status      string            `json:"Status"`
	Type        string            `json:"Type,omitempty"`
	NamespaceId string            `json:"NamespaceId,omitempty"`
	ServiceId   string            `json:"ServiceId,omitempty"`
	Targets     map[string]string `json:"Targets,omitempty"`
}

// State stores
var (
	cmNamespaces    sim.Store[CMNamespace]
	cmNamespaceVPCs sim.Store[string]
	cmServices      sim.Store[CMService]
	cmInstances     sim.Store[CMInstance]
	cmOperations    sim.Store[CMOperation]
	// cmServiceAttributes holds the per-service key/value attribute map
	// exposed by Get/Update/DeleteServiceAttributes, keyed by service ID.
	cmServiceAttributes sim.Store[map[string]string]
	// cmServiceRevisions tracks each service's instance-list revision,
	// incremented on every RegisterInstance/DeregisterInstance and returned
	// by DiscoverInstancesRevision. Keyed by service ID.
	cmServiceRevisions sim.Store[int64]
)

func cmArn(resourceType, id string) string {
	return fmt.Sprintf("arn:aws:servicediscovery:%s:%s:%s/%s", awsRegion(), awsAccountID(), resourceType, id)
}

func cmInstanceKey(serviceId, instanceId string) string {
	return serviceId + ":" + instanceId
}

func registerCloudMap(r *sim.AWSRouter, srv *sim.Server) {
	cmNamespaces = sim.MakeStore[CMNamespace](srv.DB(), "cloudmap_namespaces")
	cmNamespaceVPCs = sim.MakeStore[string](srv.DB(), "cloudmap_namespace_vpcs")
	cmServices = sim.MakeStore[CMService](srv.DB(), "cloudmap_services")
	cmInstances = sim.MakeStore[CMInstance](srv.DB(), "cloudmap_instances")
	cmOperations = sim.MakeStore[CMOperation](srv.DB(), "cloudmap_operations")
	cmServiceAttributes = sim.MakeStore[map[string]string](srv.DB(), "cloudmap_service_attributes")
	cmServiceRevisions = sim.MakeStore[int64](srv.DB(), "cloudmap_service_revisions")

	r.Register("Route53AutoNaming_v20170314.CreatePrivateDnsNamespace", handleCMCreatePrivateDnsNamespace)
	r.Register("Route53AutoNaming_v20170314.CreatePublicDnsNamespace", handleCMCreatePublicDnsNamespace)
	r.Register("Route53AutoNaming_v20170314.CreateHttpNamespace", handleCMCreateHttpNamespace)
	r.Register("Route53AutoNaming_v20170314.GetNamespace", handleCMGetNamespace)
	r.Register("Route53AutoNaming_v20170314.DeleteNamespace", handleCMDeleteNamespace)
	r.Register("Route53AutoNaming_v20170314.UpdateHttpNamespace", handleCMUpdateHttpNamespace)
	r.Register("Route53AutoNaming_v20170314.UpdatePrivateDnsNamespace", handleCMUpdatePrivateDnsNamespace)
	r.Register("Route53AutoNaming_v20170314.UpdatePublicDnsNamespace", handleCMUpdatePublicDnsNamespace)
	r.Register("Route53AutoNaming_v20170314.CreateService", handleCMCreateService)
	r.Register("Route53AutoNaming_v20170314.GetService", handleCMGetService)
	r.Register("Route53AutoNaming_v20170314.UpdateService", handleCMUpdateService)
	r.Register("Route53AutoNaming_v20170314.GetServiceAttributes", handleCMGetServiceAttributes)
	r.Register("Route53AutoNaming_v20170314.UpdateServiceAttributes", handleCMUpdateServiceAttributes)
	r.Register("Route53AutoNaming_v20170314.DeleteServiceAttributes", handleCMDeleteServiceAttributes)
	r.Register("Route53AutoNaming_v20170314.RegisterInstance", handleCMRegisterInstance)
	r.Register("Route53AutoNaming_v20170314.DeregisterInstance", handleCMDeregisterInstance)
	r.Register("Route53AutoNaming_v20170314.GetInstance", handleCMGetInstance)
	r.Register("Route53AutoNaming_v20170314.ListInstances", handleCMListInstances)
	r.Register("Route53AutoNaming_v20170314.UpdateInstanceCustomHealthStatus", handleCMUpdateInstanceCustomHealthStatus)
	r.Register("Route53AutoNaming_v20170314.GetInstancesHealthStatus", handleCMGetInstancesHealthStatus)
	r.Register("Route53AutoNaming_v20170314.DiscoverInstances", handleCMDiscoverInstances)
	r.Register("Route53AutoNaming_v20170314.DiscoverInstancesRevision", handleCMDiscoverInstancesRevision)
	r.Register("Route53AutoNaming_v20170314.GetOperation", handleCMGetOperation)
	r.Register("Route53AutoNaming_v20170314.ListOperations", handleCMListOperations)
	r.Register("Route53AutoNaming_v20170314.ListNamespaces", handleCMListNamespaces)
	r.Register("Route53AutoNaming_v20170314.ListServices", handleCMListServices)
	r.Register("Route53AutoNaming_v20170314.DeleteService", handleCMDeleteService)
	r.Register("Route53AutoNaming_v20170314.ListTagsForResource", handleCMListTagsForResource)
	r.Register("Route53AutoNaming_v20170314.TagResource", handleCMTagResource)
	r.Register("Route53AutoNaming_v20170314.UntagResource", handleCMUntagResource)
}

func handleCMCreatePrivateDnsNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"Name"`
		Vpc         string `json:"Vpc"`
		Description string `json:"Description"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidInput", "Name is required", http.StatusBadRequest)
		return
	}

	nsId := "ns-" + generateUUID()[:16]
	operationId := generateUUID()

	ns := CMNamespace{
		Id:          nsId,
		Arn:         cmArn("namespace", nsId),
		Name:        req.Name,
		Type:        "DNS_PRIVATE",
		Description: req.Description,
		Properties: &CMNamespaceProperties{
			DnsProperties: &CMDnsProperties{
				HostedZoneId: "Z" + generateUUID()[:12],
			},
		},
		CreateDate: time.Now().Unix(),
	}
	cmNamespaces.Put(nsId, ns)
	cmNamespaceVPCs.Put(nsId, req.Vpc)

	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
		Type:        "CREATE_NAMESPACE",
		NamespaceId: nsId,
		Targets:     map[string]string{"NAMESPACE": nsId},
	})

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

// handleCMCreatePublicDnsNamespace creates a DNS_PUBLIC namespace. Public DNS
// namespaces back internet-routable DNS records; the control-plane resource is
// modeled identically to a private one minus the VPC association.
func handleCMCreatePublicDnsNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"Name"`
		Description string `json:"Description"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidInput", "Name is required", http.StatusBadRequest)
		return
	}

	nsId := "ns-" + generateUUID()[:16]
	operationId := generateUUID()

	ns := CMNamespace{
		Id:          nsId,
		Arn:         cmArn("namespace", nsId),
		Name:        req.Name,
		Type:        "DNS_PUBLIC",
		Description: req.Description,
		Properties: &CMNamespaceProperties{
			DnsProperties: &CMDnsProperties{
				HostedZoneId: "Z" + generateUUID()[:12],
			},
		},
		CreateDate: time.Now().Unix(),
	}
	cmNamespaces.Put(nsId, ns)

	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
		Type:        "CREATE_NAMESPACE",
		NamespaceId: nsId,
		Targets:     map[string]string{"NAMESPACE": nsId},
	})

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

// handleCMCreateHttpNamespace creates an HTTP namespace. HTTP namespaces have
// no DNS records — discovery is via the DiscoverInstances API only — and carry
// an HttpProperties.HttpName (defaulting to the namespace name).
func handleCMCreateHttpNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"Name"`
		Description string `json:"Description"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidInput", "Name is required", http.StatusBadRequest)
		return
	}

	nsId := "ns-" + generateUUID()[:16]
	operationId := generateUUID()

	ns := CMNamespace{
		Id:          nsId,
		Arn:         cmArn("namespace", nsId),
		Name:        req.Name,
		Type:        "HTTP",
		Description: req.Description,
		Properties: &CMNamespaceProperties{
			HttpProperties: &CMHttpProperties{HttpName: req.Name},
		},
		CreateDate: time.Now().Unix(),
	}
	cmNamespaces.Put(nsId, ns)

	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
		Type:        "CREATE_NAMESPACE",
		NamespaceId: nsId,
		Targets:     map[string]string{"NAMESPACE": nsId},
	})

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

func handleCMGetNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id string `json:"Id"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}

	ns, ok := cmNamespaces.Get(req.Id)
	if !ok {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", req.Id)
		return
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Namespace": ns,
	})
}

func handleCMDeleteNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id string `json:"Id"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}

	if _, ok := cmNamespaces.Get(req.Id); !ok {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", req.Id)
		return
	}
	if cmNamespaceHasServices(req.Id) {
		sim.AWSErrorf(w, "ResourceInUse", http.StatusBadRequest,
			"Namespace '%s' contains services and can't be deleted", req.Id)
		return
	}

	cmNamespaces.Delete(req.Id)
	cmNamespaceVPCs.Delete(req.Id)
	operationId := generateUUID()
	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
	})

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

// cmUpdateNamespaceOp records a successful UPDATE_NAMESPACE operation and writes
// the OperationId response shared by all three Update*Namespace ops.
func cmUpdateNamespaceOp(w http.ResponseWriter, nsId string) {
	operationId := generateUUID()
	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
		Type:        "UPDATE_NAMESPACE",
		NamespaceId: nsId,
		Targets:     map[string]string{"NAMESPACE": nsId},
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

func handleCMUpdateHttpNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id        string `json:"Id"`
		Namespace struct {
			Description *string `json:"Description"`
		} `json:"Namespace"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}
	if _, ok := cmNamespaces.Get(req.Id); !ok {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", req.Id)
		return
	}
	cmNamespaces.Update(req.Id, func(ns *CMNamespace) {
		if req.Namespace.Description != nil {
			ns.Description = *req.Namespace.Description
		}
	})
	cmUpdateNamespaceOp(w, req.Id)
}

// handleCMUpdatePrivateDnsNamespace updates a DNS_PRIVATE namespace's mutable
// fields: Description and the DNS SOA TTL.
func handleCMUpdatePrivateDnsNamespace(w http.ResponseWriter, r *http.Request) {
	handleCMUpdateDnsNamespace(w, r)
}

// handleCMUpdatePublicDnsNamespace updates a DNS_PUBLIC namespace's mutable
// fields: Description and the DNS SOA TTL.
func handleCMUpdatePublicDnsNamespace(w http.ResponseWriter, r *http.Request) {
	handleCMUpdateDnsNamespace(w, r)
}

func handleCMUpdateDnsNamespace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id        string `json:"Id"`
		Namespace struct {
			Description *string `json:"Description"`
			Properties  *struct {
				DnsProperties *struct {
					SOA *struct {
						TTL int64 `json:"TTL"`
					} `json:"SOA"`
				} `json:"DnsProperties"`
			} `json:"Properties"`
		} `json:"Namespace"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}
	if _, ok := cmNamespaces.Get(req.Id); !ok {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", req.Id)
		return
	}
	cmNamespaces.Update(req.Id, func(ns *CMNamespace) {
		if req.Namespace.Description != nil {
			ns.Description = *req.Namespace.Description
		}
		if p := req.Namespace.Properties; p != nil && p.DnsProperties != nil && p.DnsProperties.SOA != nil {
			if ns.Properties == nil {
				ns.Properties = &CMNamespaceProperties{}
			}
			if ns.Properties.DnsProperties == nil {
				ns.Properties.DnsProperties = &CMDnsProperties{}
			}
			ns.Properties.DnsProperties.SOA = &struct {
				TTL int64 `json:"TTL"`
			}{TTL: p.DnsProperties.SOA.TTL}
		}
	})
	cmUpdateNamespaceOp(w, req.Id)
}

func handleCMCreateService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              string               `json:"Name"`
		NamespaceId       string               `json:"NamespaceId"`
		Description       string               `json:"Description"`
		DnsConfig         *CMDnsConfig         `json:"DnsConfig"`
		HealthCheckConfig *CMHealthCheckConfig `json:"HealthCheckConfig"`
		Type              string               `json:"Type"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidInput", "Name is required", http.StatusBadRequest)
		return
	}

	namespaceId := req.NamespaceId
	if namespaceId == "" && req.DnsConfig != nil {
		namespaceId = req.DnsConfig.NamespaceId
	}
	if namespaceId != "" {
		if _, ok := cmNamespaces.Get(namespaceId); !ok {
			sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
				"Namespace '%s' not found", namespaceId)
			return
		}
	}

	svcId := "srv-" + generateUUID()[:16]
	svc := CMService{
		Id:                svcId,
		Arn:               cmArn("service", svcId),
		Name:              req.Name,
		NamespaceId:       namespaceId,
		Description:       req.Description,
		DnsConfig:         req.DnsConfig,
		HealthCheckConfig: req.HealthCheckConfig,
		Type:              cmServiceType(req.Type, namespaceId, req.DnsConfig),
		CreateDate:        time.Now().Unix(),
	}
	cmServices.Put(svcId, svc)

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Service": svc,
	})
}

func handleCMGetService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id string `json:"Id"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}

	svc, ok := cmServices.Get(req.Id)
	if !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.Id)
		return
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Service": svc,
	})
}

// cmServiceType derives a service's discovery Type from the explicit request
// value (if any), the owning namespace, and whether the service has DNS records
// — mirroring how Cloud Map labels a service DNS / DNS_HTTP / HTTP. A service in
// an HTTP namespace, or one without DNS records, is HTTP; a service in a DNS
// namespace with DNS records is DNS (DNS_HTTP when discoverable both ways).
func cmServiceType(explicit, namespaceId string, dnsConfig *CMDnsConfig) string {
	if explicit != "" {
		return explicit
	}
	hasDNS := dnsConfig != nil && len(dnsConfig.DnsRecords) > 0
	if ns, ok := cmNamespaces.Get(namespaceId); ok {
		switch ns.Type {
		case "HTTP":
			return "HTTP"
		case "DNS_PRIVATE", "DNS_PUBLIC":
			if hasDNS {
				return "DNS_HTTP"
			}
			return "HTTP"
		}
	}
	if hasDNS {
		return "DNS"
	}
	return "HTTP"
}

func handleCMUpdateService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id      string `json:"Id"`
		Service struct {
			Description       *string              `json:"Description"`
			DnsConfig         *CMDnsConfig         `json:"DnsConfig"`
			HealthCheckConfig *CMHealthCheckConfig `json:"HealthCheckConfig"`
		} `json:"Service"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}
	svc, ok := cmServices.Get(req.Id)
	if !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.Id)
		return
	}
	cmServices.Update(req.Id, func(s *CMService) {
		if req.Service.Description != nil {
			s.Description = *req.Service.Description
		}
		if req.Service.DnsConfig != nil {
			s.DnsConfig = req.Service.DnsConfig
		}
		if req.Service.HealthCheckConfig != nil {
			s.HealthCheckConfig = req.Service.HealthCheckConfig
		}
	})

	operationId := generateUUID()
	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
		Type:        "UPDATE_SERVICE",
		NamespaceId: svc.NamespaceId,
		ServiceId:   req.Id,
		Targets:     map[string]string{"SERVICE": req.Id},
	})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

func handleCMGetServiceAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId string `json:"ServiceId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId is required", http.StatusBadRequest)
		return
	}
	svc, ok := cmServices.Get(req.ServiceId)
	if !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	attrs, ok := cmServiceAttributes.Get(req.ServiceId)
	if !ok {
		attrs = map[string]string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"ServiceAttributes": map[string]any{
			"ServiceArn": svc.Arn,
			"Attributes": attrs,
		},
	})
}

func handleCMUpdateServiceAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId  string            `json:"ServiceId"`
		Attributes map[string]string `json:"Attributes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId is required", http.StatusBadRequest)
		return
	}
	if _, ok := cmServices.Get(req.ServiceId); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	attrs, ok := cmServiceAttributes.Get(req.ServiceId)
	if !ok {
		attrs = map[string]string{}
	}
	merged := make(map[string]string, len(attrs)+len(req.Attributes))
	for k, v := range attrs {
		merged[k] = v
	}
	for k, v := range req.Attributes {
		merged[k] = v
	}
	cmServiceAttributes.Put(req.ServiceId, merged)
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCMDeleteServiceAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId  string   `json:"ServiceId"`
		Attributes []string `json:"Attributes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId is required", http.StatusBadRequest)
		return
	}
	if _, ok := cmServices.Get(req.ServiceId); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	if attrs, ok := cmServiceAttributes.Get(req.ServiceId); ok {
		next := make(map[string]string, len(attrs))
		for k, v := range attrs {
			next[k] = v
		}
		for _, k := range req.Attributes {
			delete(next, k)
		}
		cmServiceAttributes.Put(req.ServiceId, next)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCMRegisterInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId  string            `json:"ServiceId"`
		InstanceId string            `json:"InstanceId"`
		Attributes map[string]string `json:"Attributes"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" || req.InstanceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId and InstanceId are required", http.StatusBadRequest)
		return
	}

	svc, ok := cmServices.Get(req.ServiceId)
	if !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	ns, nsOk := cmNamespaces.Get(svc.NamespaceId)
	if !nsOk {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", svc.NamespaceId)
		return
	}

	// Store the instance BEFORE realizing DNS so the realization below sees
	// this registration. Real Cloud Map registers an instance per service
	// (ServiceId+InstanceId), so one container (instance ID) may back several
	// services — i.e. resolve under several DNS names that all point at its IP.
	// The realization paths gather the full set of names for the container.
	instance := CMInstance{
		Id:         req.InstanceId,
		Attributes: req.Attributes,
	}
	key := cmInstanceKey(req.ServiceId, req.InstanceId)
	_, existed := cmInstances.Get(key)
	cmInstances.Put(key, instance)
	if !existed {
		cmServices.Update(req.ServiceId, func(svc *CMService) {
			svc.InstanceCount++
		})
	}
	cmBumpServiceRevision(req.ServiceId)
	rollback := func() {
		if !existed {
			cmInstances.Delete(key)
			cmServices.Update(req.ServiceId, func(svc *CMService) {
				if svc.InstanceCount > 0 {
					svc.InstanceCount--
				}
			})
		}
		cmBumpServiceRevision(req.ServiceId)
	}

	containerName := resolveTaskContainerForInstance(req.InstanceId)
	switch {
	case cmContainerUsesHostEntries(containerName):
		// netns/awsvpc tier: the real ENI occupies eth0, so DNS is realized via
		// /etc/hosts entries (syncCMNamespaceHosts already gathers every service
		// name per instance IP — multi-name aware).
		if err := syncCMNamespaceHosts(svc.NamespaceId); err != nil {
			rollback()
			sim.AWSErrorf(w, "InternalFailure", http.StatusInternalServerError,
				"failed to update Cloud Map task hosts: %v", err)
			return
		}
	case containerName != "":
		// Docker-network tier: realize EVERY service name this container backs
		// as a DNS alias on the namespace network, so siblings resolve it by any
		// of its registered names (e.g. a service alias `redis` AND its task
		// hostname both point at the redis container).
		if err := realizeCMContainerDockerAliases(ns, containerName); err != nil {
			rollback()
			sim.AWSErrorf(w, "InternalFailure", http.StatusInternalServerError,
				"failed to connect task container to Cloud Map namespace network: %v", err)
			return
		}
	}

	operationId := generateUUID()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

// cmDockerAliasesForContainer returns every DNS alias the container must answer
// to on the namespace's Docker network: for each Cloud Map service whose
// registered instance maps to containerName, both the short service name AND
// its `<service>.<namespace>` FQDN (so e.g. `redis` and `redis.skls-net.local`
// both resolve — the runtime's network DNS resolves both as network aliases).
// This is the Docker-network-tier analogue of syncCMNamespaceHosts, which adds
// the same two forms as /etc/hosts entries for the netns tier.
func cmDockerAliasesForContainer(namespaceID, containerName string) []string {
	nsName := ""
	if ns, ok := cmNamespaces.Get(namespaceID); ok {
		nsName = ns.Name
	}
	seen := make(map[string]struct{})
	var aliases []string
	add := func(name string) {
		if name == "" {
			return
		}
		if _, dup := seen[name]; dup {
			return
		}
		seen[name] = struct{}{}
		aliases = append(aliases, name)
	}
	// Resolve every instance ID to its task container in a single combined pass
	// over instances and ECS tasks, so the service×instance loop below is a map
	// lookup instead of re-scanning all instances and all tasks per pair.
	instanceContainers := resolveTaskContainersForInstances()
	for _, svc := range cmServices.List() {
		if svc.NamespaceId != namespaceID {
			continue
		}
		for _, inst := range cmInstances.List() {
			if _, ok := cmInstances.Get(cmInstanceKey(svc.Id, inst.Id)); !ok {
				continue
			}
			if instanceContainers[inst.Id] != containerName {
				continue
			}
			add(svc.Name)
			if nsName != "" {
				add(svc.Name + "." + nsName)
			}
		}
	}
	return aliases
}

// resolveTaskContainersForInstances maps every Cloud Map instance ID to its task
// container name in two single passes — one to index ECS tasks by private IP,
// one over instances — replacing the per-instance re-scan of all instances
// (cmInstanceIPv4) and all ECS tasks (resolveTaskContainerForInstance) that a
// naive nested loop incurs. The per-instance result is identical to
// resolveTaskContainerForInstance(id).
func resolveTaskContainersForInstances() map[string]string {
	containerByIP := make(map[string]string)
	for _, task := range ecsTasks.List() {
		ip := ecsTaskPrivateIP(task)
		if ip == "" {
			continue
		}
		// Derive task UUID from ARN ("arn:…/task/<cluster>/<taskId>").
		taskId := task.TaskArn
		if i := lastSlash(taskId); i >= 0 {
			taskId = taskId[i+1:]
		}
		if len(taskId) < 12 {
			continue
		}
		containerName := "sockerless-sim-aws-task-" + taskId[:12]
		if taskHasENI(task) && ec2ECSRealNetAvailable() {
			containerName += "-pause"
		}
		containerByIP[ip] = containerName
	}
	out := make(map[string]string)
	for _, inst := range cmInstances.List() {
		if _, dup := out[inst.Id]; dup {
			continue
		}
		ip := inst.Attributes["AWS_INSTANCE_IPV4"]
		if ip == "" {
			continue
		}
		if name, ok := containerByIP[ip]; ok {
			out[inst.Id] = name
		}
	}
	return out
}

// realizeCMContainerDockerAliases (re)attaches a task container to its Cloud Map
// namespace network with the full set of service-name aliases it currently
// backs. Docker rejects connecting an already-connected container and can't add
// an alias to a live endpoint, so it re-attaches with the full set (the same
// disconnect-then-connect pattern the azure ACA multi-CNAME path uses). The
// disconnect is best-effort: a not-yet-attached container errors there, which
// the connect corrects; when the container backs no services it stays detached.
func realizeCMContainerDockerAliases(ns CMNamespace, containerName string) error {
	networkName, err := ensureCMNamespaceDockerNetwork(ns)
	if err != nil {
		return err
	}
	aliases := cmDockerAliasesForContainer(ns.Id, containerName)
	_ = sim.DisconnectContainerFromNetwork(containerName, networkName)
	if len(aliases) == 0 {
		return nil
	}
	return sim.ConnectContainerToNetwork(containerName, networkName, aliases)
}

// resolveTaskContainerForInstance maps a Cloud Map instance to the simulator's
// Docker container by the registered `AWS_INSTANCE_IPV4` attribute — the
// faithful Cloud Map mechanism (a client registers an instance with the task's
// IP; the sim matches it to the task carrying that private IP), not any
// sockerless-specific tag. On the netns awsvpc fabric the pause container owns
// the namespace; that tier cannot join Docker's DNS network after the real ENI
// occupies eth0, so Cloud Map uses host entries in the task container instead.
func resolveTaskContainerForInstance(instanceId string) string {
	ip := cmInstanceIPv4(instanceId)
	if ip == "" {
		return ""
	}
	for _, task := range ecsTasks.List() {
		if ecsTaskPrivateIP(task) != ip {
			continue
		}
		// Derive task UUID from ARN ("arn:…/task/<cluster>/<taskId>").
		taskId := task.TaskArn
		if i := lastSlash(taskId); i >= 0 {
			taskId = taskId[i+1:]
		}
		if len(taskId) < 12 {
			return ""
		}
		containerName := "sockerless-sim-aws-task-" + taskId[:12]
		if taskHasENI(task) && ec2ECSRealNetAvailable() {
			return containerName + "-pause"
		}
		return containerName
	}
	return ""
}

// cmInstanceIPv4 returns a registered Cloud Map instance's AWS_INSTANCE_IPV4
// attribute (the standard address attribute), searched across services.
func cmInstanceIPv4(instanceID string) string {
	for _, inst := range cmInstances.List() {
		if inst.Id == instanceID {
			return inst.Attributes["AWS_INSTANCE_IPV4"]
		}
	}
	return ""
}

// ecsTaskPrivateIP returns a task's private IPv4 address from its awsvpc ENI
// attachment's `privateIPv4Address` detail (the value DescribeTasks surfaces and
// the ECS backend registers as the Cloud Map instance's AWS_INSTANCE_IPV4).
func ecsTaskPrivateIP(task ECSTask) string {
	for _, att := range task.Attachments {
		if ip := ecsTaskDetail(att.Details, "privateIPv4Address"); ip != "" {
			return ip
		}
	}
	return ""
}

func cmContainerUsesHostEntries(containerName string) bool {
	return strings.HasSuffix(containerName, "-pause")
}

func cmNamespaceHasHostEntryTargets(namespaceID string) bool {
	if _, ok := cmNamespaces.Get(namespaceID); !ok || !ec2ECSRealNetAvailable() {
		return false
	}
	vpcID, _ := cmNamespaceVPCs.Get(namespaceID)
	for _, task := range ecsTasks.List() {
		if task.LastStatus != ECSTaskStatusRunning || !taskHasENI(task) {
			continue
		}
		if vpcID == "" || taskVPCID(task) == vpcID {
			return true
		}
	}
	return false
}

func cmTaskContainerName(task ECSTask) string {
	taskID := task.TaskArn
	if i := lastSlash(taskID); i >= 0 {
		taskID = taskID[i+1:]
	}
	if len(taskID) < 12 {
		return ""
	}
	return "sockerless-sim-aws-task-" + taskID[:12]
}

func taskVPCID(task ECSTask) string {
	for _, att := range task.Attachments {
		if att.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, d := range att.Details {
			if d.Name != "subnetId" {
				continue
			}
			if subnet, ok := ec2Subnets.Get(d.Value); ok {
				return subnet.VpcId
			}
		}
	}
	return ""
}

func syncCMNamespaceHosts(namespaceID string) error {
	ns, ok := cmNamespaces.Get(namespaceID)
	if !ok {
		return fmt.Errorf("namespace %s not found", namespaceID)
	}
	vpcID, _ := cmNamespaceVPCs.Get(namespaceID)

	var entries []sim.HostEntry
	for _, svc := range cmServices.List() {
		if svc.NamespaceId != namespaceID {
			continue
		}
		for _, inst := range cmInstances.List() {
			key := cmInstanceKey(svc.Id, inst.Id)
			stored, exists := cmInstances.Get(key)
			if !exists {
				continue
			}
			ip := stored.Attributes["AWS_INSTANCE_IPV4"]
			if ip == "" {
				continue
			}
			entries = append(entries, sim.HostEntry{IP: ip, Name: svc.Name})
			if ns.Name != "" {
				entries = append(entries, sim.HostEntry{IP: ip, Name: svc.Name + "." + ns.Name})
			}
		}
	}

	marker := "sockerless-cloudmap-" + namespaceID
	for _, task := range ecsTasks.List() {
		if task.LastStatus != ECSTaskStatusRunning || !taskHasENI(task) {
			continue
		}
		if vpcID != "" && taskVPCID(task) != vpcID {
			continue
		}
		containerName := cmTaskContainerName(task)
		if containerName == "" {
			continue
		}
		if err := sim.SyncContainerHostEntries(containerName, marker, entries); err != nil {
			return fmt.Errorf("sync hosts for %s: %w", containerName, err)
		}
	}
	return nil
}

func taskHasENI(task ECSTask) bool {
	for _, att := range task.Attachments {
		if att.Type == "ElasticNetworkInterface" {
			return true
		}
	}
	return false
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func handleCMDeregisterInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId  string `json:"ServiceId"`
		InstanceId string `json:"InstanceId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" || req.InstanceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId and InstanceId are required", http.StatusBadRequest)
		return
	}

	key := cmInstanceKey(req.ServiceId, req.InstanceId)
	if !cmInstances.Delete(key) {
		sim.AWSErrorf(w, "InstanceNotFound", http.StatusBadRequest,
			"Instance '%s' not found", req.InstanceId)
		return
	}

	// Update service instance count
	cmServices.Update(req.ServiceId, func(svc *CMService) {
		if svc.InstanceCount > 0 {
			svc.InstanceCount--
		}
	})
	cmBumpServiceRevision(req.ServiceId)

	if svc, ok := cmServices.Get(req.ServiceId); ok {
		containerName := resolveTaskContainerForInstance(req.InstanceId)
		if cmContainerUsesHostEntries(containerName) || cmNamespaceHasHostEntryTargets(svc.NamespaceId) {
			if err := syncCMNamespaceHosts(svc.NamespaceId); err != nil {
				sim.AWSErrorf(w, "InternalFailure", http.StatusInternalServerError,
					"failed to update Cloud Map task hosts: %v", err)
				return
			}
		} else if ns, nsOk := cmNamespaces.Get(svc.NamespaceId); nsOk && ns.DockerNetworkName != "" {
			// Re-realize the container's REMAINING aliases: it may still back
			// other services in the namespace, so a plain disconnect would drop
			// names that are still registered. realizeCMContainerDockerAliases
			// reconnects with the reduced set, or detaches when none remain.
			if containerName != "" {
				_ = realizeCMContainerDockerAliases(ns, containerName)
			}
		}
	}

	operationId := generateUUID()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

func handleCMListInstances(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId string `json:"ServiceId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId is required", http.StatusBadRequest)
		return
	}

	instances := cmInstances.Filter(func(inst CMInstance) bool {
		// Since keys are serviceId:instanceId, we filter by checking
		// all instances that belong to this service
		return true
	})

	// Collect matching instances by iterating known instance IDs.
	var result []CMInstance
	seen := make(map[string]bool)
	for _, inst := range instances {
		key := cmInstanceKey(req.ServiceId, inst.Id)
		if _, ok := cmInstances.Get(key); ok && !seen[inst.Id] {
			seen[inst.Id] = true
			result = append(result, inst)
		}
	}

	if result == nil {
		result = []CMInstance{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Instances": result,
	})
}

func handleCMListNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces := cmNamespaces.List()
	if namespaces == nil {
		namespaces = []CMNamespace{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Namespaces": namespaces,
	})
}

func handleCMListServices(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filters []struct {
			Name      string   `json:"Name"`
			Values    []string `json:"Values"`
			Condition string   `json:"Condition"`
		} `json:"Filters"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	services := cmServices.List()

	// Apply filters if provided
	if len(req.Filters) > 0 {
		var filtered []CMService
		for _, svc := range services {
			match := true
			for _, f := range req.Filters {
				switch f.Name {
				case "NAMESPACE_ID":
					if len(f.Values) > 0 {
						found := false
						for _, v := range f.Values {
							if svc.NamespaceId == v {
								found = true
								break
							}
						}
						if f.Condition == "EQ" || f.Condition == "" {
							if !found {
								match = false
							}
						}
					}
				}
			}
			if match {
				filtered = append(filtered, svc)
			}
		}
		services = filtered
	}

	summaries := make([]map[string]any, 0, len(services))
	for _, svc := range services {
		summaries = append(summaries, cmServiceSummary(svc))
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Services": summaries,
	})
}

// cmServiceSummary projects a stored service onto the ServiceSummary
// shape. NamespaceId is a Service (GetService/CreateService) member kept
// in the store for namespace filtering and DNS wiring; ServiceSummary
// has no such member.
func cmServiceSummary(svc CMService) map[string]any {
	out := map[string]any{
		"Id":            svc.Id,
		"Arn":           svc.Arn,
		"Name":          svc.Name,
		"CreateDate":    svc.CreateDate,
		"InstanceCount": svc.InstanceCount,
	}
	if svc.Description != "" {
		out["Description"] = svc.Description
	}
	if svc.DnsConfig != nil {
		out["DnsConfig"] = svc.DnsConfig
	}
	if svc.HealthCheckConfig != nil {
		out["HealthCheckConfig"] = svc.HealthCheckConfig
	}
	if svc.Type != "" {
		out["Type"] = svc.Type
	}
	return out
}

func handleCMDeleteService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Id string `json:"Id"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Id == "" {
		sim.AWSError(w, "InvalidInput", "Id is required", http.StatusBadRequest)
		return
	}

	if _, ok := cmServices.Get(req.Id); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.Id)
		return
	}
	if cmServiceHasInstances(req.Id) {
		sim.AWSErrorf(w, "ResourceInUse", http.StatusBadRequest,
			"Service '%s' contains instances and can't be deleted", req.Id)
		return
	}

	cmServices.Delete(req.Id)

	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func ensureCMNamespaceDockerNetwork(ns CMNamespace) (string, error) {
	if ns.DockerNetworkName != "" {
		return ns.DockerNetworkName, nil
	}
	networkName := "sim-" + ns.Id
	if _, err := sim.EnsureDockerNetwork(networkName); err != nil {
		return "", err
	}
	cmNamespaces.Update(ns.Id, func(stored *CMNamespace) {
		stored.DockerNetworkName = networkName
	})
	return networkName, nil
}

func cmNamespaceHasServices(namespaceId string) bool {
	for _, svc := range cmServices.List() {
		if svc.NamespaceId == namespaceId {
			return true
		}
	}
	return false
}

func cmServiceHasInstances(serviceId string) bool {
	for _, inst := range cmInstances.List() {
		if _, ok := cmInstances.Get(cmInstanceKey(serviceId, inst.Id)); ok {
			return true
		}
	}
	return false
}

func handleCMGetOperation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperationId string `json:"OperationId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}

	op, ok := cmOperations.Get(req.OperationId)
	if !ok {
		// Unknown operations are assumed to be completed
		op = CMOperation{
			OperationId: req.OperationId,
			Status:      "SUCCESS",
		}
	}

	result := map[string]any{
		"Id":     op.OperationId,
		"Status": op.Status,
	}
	if len(op.Targets) > 0 {
		result["Targets"] = op.Targets
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Operation": result,
	})
}

func handleCMListTagsForResource(w http.ResponseWriter, r *http.Request) {
	if err := sim.ReadJSON(r, &struct{}{}); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Tags": []any{},
	})
}

func handleCMTagResource(w http.ResponseWriter, r *http.Request) {
	if err := sim.ReadJSON(r, &struct{}{}); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCMUntagResource(w http.ResponseWriter, r *http.Request) {
	if err := sim.ReadJSON(r, &struct{}{}); err != nil {
		sim.AWSErrorf(w, "InvalidParameterValue", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCMDiscoverInstances(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NamespaceName string `json:"NamespaceName"`
		ServiceName   string `json:"ServiceName"`
		HealthStatus  string `json:"HealthStatus"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.NamespaceName == "" || req.ServiceName == "" {
		sim.AWSError(w, "InvalidInput", "NamespaceName and ServiceName are required", http.StatusBadRequest)
		return
	}

	// Find the namespace by name
	var targetNs *CMNamespace
	for _, ns := range cmNamespaces.List() {
		if ns.Name == req.NamespaceName {
			nsCopy := ns
			targetNs = &nsCopy
			break
		}
	}
	if targetNs == nil {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", req.NamespaceName)
		return
	}

	// Find the service by name in this namespace
	var targetSvc *CMService
	for _, svc := range cmServices.List() {
		if svc.Name == req.ServiceName && svc.NamespaceId == targetNs.Id {
			svcCopy := svc
			targetSvc = &svcCopy
			break
		}
	}
	if targetSvc == nil {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found in namespace '%s'", req.ServiceName, req.NamespaceName)
		return
	}

	// Collect all instances for this service
	// Each instance reports its real health (the AWS_INIT_HEALTH_STATUS attribute
	// it registered with, default HEALTHY) — not a hardcoded value — and the
	// HealthStatus request filter scopes the result the way real Cloud Map does.
	type discovered struct {
		entry  map[string]any
		health string
	}
	var all []discovered
	for _, inst := range cmInstances.List() {
		key := cmInstanceKey(targetSvc.Id, inst.Id)
		if _, ok := cmInstances.Get(key); !ok {
			continue
		}
		health := "HEALTHY"
		if h := inst.Attributes["AWS_INIT_HEALTH_STATUS"]; h != "" {
			health = h
		}
		all = append(all, discovered{
			entry: map[string]any{
				"InstanceId":    inst.Id,
				"NamespaceName": req.NamespaceName,
				"ServiceName":   req.ServiceName,
				"HealthStatus":  health,
				"Attributes":    inst.Attributes,
			},
			health: health,
		})
	}
	pick := func(keep func(string) bool) []map[string]any {
		out := []map[string]any{}
		for _, d := range all {
			if keep(d.health) {
				out = append(out, d.entry)
			}
		}
		return out
	}
	var httpInstances []map[string]any
	switch req.HealthStatus {
	case "HEALTHY":
		httpInstances = pick(func(h string) bool { return h == "HEALTHY" })
	case "UNHEALTHY":
		httpInstances = pick(func(h string) bool { return h == "UNHEALTHY" })
	case "ALL":
		httpInstances = pick(func(string) bool { return true })
	default: // omitted / HEALTHY_OR_ELSE_ALL: healthy, or all if none are healthy
		httpInstances = pick(func(h string) bool { return h == "HEALTHY" })
		if len(httpInstances) == 0 {
			httpInstances = pick(func(string) bool { return true })
		}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Instances": httpInstances,
	})
}

// cmBumpServiceRevision increments a service's instance-list revision. The
// revision starts at 0 and increases on every RegisterInstance and
// DeregisterInstance, matching the Cloud Map InstancesRevision contract
// (health-status updates do NOT bump it).
func cmBumpServiceRevision(serviceId string) {
	cur, _ := cmServiceRevisions.Get(serviceId)
	cmServiceRevisions.Put(serviceId, cur+1)
}

// cmInstanceHealth returns an instance's effective health: the custom health
// status when set (HealthCheckCustomConfig services), otherwise the registered
// AWS_INIT_HEALTH_STATUS attribute, defaulting to HEALTHY.
func cmInstanceHealth(inst CMInstance) string {
	if inst.CustomHealthStatus != "" {
		return inst.CustomHealthStatus
	}
	if h := inst.Attributes["AWS_INIT_HEALTH_STATUS"]; h != "" {
		return h
	}
	return "HEALTHY"
}

func handleCMGetInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId  string `json:"ServiceId"`
		InstanceId string `json:"InstanceId"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" || req.InstanceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId and InstanceId are required", http.StatusBadRequest)
		return
	}
	if _, ok := cmServices.Get(req.ServiceId); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	inst, ok := cmInstances.Get(cmInstanceKey(req.ServiceId, req.InstanceId))
	if !ok {
		sim.AWSErrorf(w, "InstanceNotFound", http.StatusBadRequest,
			"Instance '%s' not found", req.InstanceId)
		return
	}
	attrs := inst.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Instance": map[string]any{
			"Id":         inst.Id,
			"Attributes": attrs,
		},
	})
}

func handleCMUpdateInstanceCustomHealthStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId  string `json:"ServiceId"`
		InstanceId string `json:"InstanceId"`
		Status     string `json:"Status"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" || req.InstanceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId and InstanceId are required", http.StatusBadRequest)
		return
	}
	if req.Status != "HEALTHY" && req.Status != "UNHEALTHY" {
		sim.AWSError(w, "InvalidInput", "Status must be HEALTHY or UNHEALTHY", http.StatusBadRequest)
		return
	}
	if _, ok := cmServices.Get(req.ServiceId); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	key := cmInstanceKey(req.ServiceId, req.InstanceId)
	if _, ok := cmInstances.Get(key); !ok {
		sim.AWSErrorf(w, "InstanceNotFound", http.StatusBadRequest,
			"Instance '%s' not found", req.InstanceId)
		return
	}
	cmInstances.Update(key, func(inst *CMInstance) {
		inst.CustomHealthStatus = req.Status
	})
	// UpdateInstanceCustomHealthStatus has an empty (Unit) response body.
	sim.WriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCMGetInstancesHealthStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServiceId string   `json:"ServiceId"`
		Instances []string `json:"Instances"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ServiceId == "" {
		sim.AWSError(w, "InvalidInput", "ServiceId is required", http.StatusBadRequest)
		return
	}
	if _, ok := cmServices.Get(req.ServiceId); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found", req.ServiceId)
		return
	}
	want := map[string]bool{}
	for _, id := range req.Instances {
		want[id] = true
	}
	status := map[string]string{}
	for _, inst := range cmInstances.List() {
		key := cmInstanceKey(req.ServiceId, inst.Id)
		stored, ok := cmInstances.Get(key)
		if !ok {
			continue
		}
		if len(want) > 0 && !want[inst.Id] {
			continue
		}
		status[inst.Id] = cmInstanceHealth(stored)
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Status": status,
	})
}

func handleCMDiscoverInstancesRevision(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NamespaceName string `json:"NamespaceName"`
		ServiceName   string `json:"ServiceName"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.NamespaceName == "" || req.ServiceName == "" {
		sim.AWSError(w, "InvalidInput", "NamespaceName and ServiceName are required", http.StatusBadRequest)
		return
	}
	var targetNs *CMNamespace
	for _, ns := range cmNamespaces.List() {
		if ns.Name == req.NamespaceName {
			nsCopy := ns
			targetNs = &nsCopy
			break
		}
	}
	if targetNs == nil {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
			"Namespace '%s' not found", req.NamespaceName)
		return
	}
	var targetSvc *CMService
	for _, svc := range cmServices.List() {
		if svc.Name == req.ServiceName && svc.NamespaceId == targetNs.Id {
			svcCopy := svc
			targetSvc = &svcCopy
			break
		}
	}
	if targetSvc == nil {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
			"Service '%s' not found in namespace '%s'", req.ServiceName, req.NamespaceName)
		return
	}
	revision, _ := cmServiceRevisions.Get(targetSvc.Id)
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"InstancesRevision": revision,
	})
}

func handleCMListOperations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filters []struct {
			Name      string   `json:"Name"`
			Values    []string `json:"Values"`
			Condition string   `json:"Condition"`
		} `json:"Filters"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSErrorf(w, "InvalidInput", http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	matches := func(op CMOperation) bool {
		for _, f := range req.Filters {
			var actual string
			switch f.Name {
			case "NAMESPACE_ID":
				actual = op.NamespaceId
			case "SERVICE_ID":
				actual = op.ServiceId
			case "STATUS":
				actual = op.Status
			case "TYPE":
				actual = op.Type
			case "UPDATE_DATE":
				// Date-range filtering is not modeled; treat as a match so a
				// client narrowing by date still sees the synchronous ops.
				continue
			default:
				continue
			}
			found := false
			for _, v := range f.Values {
				if actual == v {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}

	operations := []map[string]any{}
	for _, op := range cmOperations.List() {
		if !matches(op) {
			continue
		}
		operations = append(operations, map[string]any{
			"Id":     op.OperationId,
			"Status": op.Status,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Operations": operations,
	})
}
