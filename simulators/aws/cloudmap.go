package main

import (
	"fmt"
	"net/http"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Map types

type CMNamespace struct {
	Id          string              `json:"Id"`
	Arn         string              `json:"Arn"`
	Name        string              `json:"Name"`
	Type        string              `json:"Type"`
	Description string              `json:"Description,omitempty"`
	Properties  *CMNamespaceProperties `json:"Properties,omitempty"`
	CreateDate  int64               `json:"CreateDate"`
}

type CMNamespaceProperties struct {
	DnsProperties *CMDnsProperties `json:"DnsProperties,omitempty"`
}

type CMDnsProperties struct {
	HostedZoneId string `json:"HostedZoneId"`
	SOA          *struct {
		TTL int64 `json:"TTL"`
	} `json:"SOA,omitempty"`
}

type CMService struct {
	Id           string         `json:"Id"`
	Arn          string         `json:"Arn"`
	Name         string         `json:"Name"`
	NamespaceId  string         `json:"NamespaceId"`
	Description  string         `json:"Description,omitempty"`
	DnsConfig    *CMDnsConfig   `json:"DnsConfig,omitempty"`
	CreateDate   int64          `json:"CreateDate"`
	InstanceCount int           `json:"InstanceCount"`
}

type CMDnsConfig struct {
	NamespaceId  string          `json:"NamespaceId,omitempty"`
	RoutingPolicy string         `json:"RoutingPolicy,omitempty"`
	DnsRecords   []CMDnsRecord   `json:"DnsRecords,omitempty"`
}

type CMDnsRecord struct {
	Type string `json:"Type"`
	TTL  int64  `json:"TTL"`
}

type CMInstance struct {
	Id         string            `json:"Id"`
	Attributes map[string]string `json:"Attributes,omitempty"`
}

type CMOperation struct {
	OperationId string            `json:"OperationId"`
	Status      string            `json:"Status"`
	Targets     map[string]string `json:"Targets,omitempty"`
}

// State stores
var (
	cmNamespaces  *sim.StateStore[CMNamespace]
	cmServices    *sim.StateStore[CMService]
	cmInstances   *sim.StateStore[CMInstance]
	cmOperations  *sim.StateStore[CMOperation]
)

func cmArn(resourceType, id string) string {
	return fmt.Sprintf("arn:aws:servicediscovery:us-east-1:123456789012:%s/%s", resourceType, id)
}

func cmInstanceKey(serviceId, instanceId string) string {
	return serviceId + ":" + instanceId
}

func registerCloudMap(r *sim.AWSRouter, srv *sim.Server) {
	cmNamespaces = sim.NewStateStore[CMNamespace]()
	cmServices = sim.NewStateStore[CMService]()
	cmInstances = sim.NewStateStore[CMInstance]()
	cmOperations = sim.NewStateStore[CMOperation]()

	r.Register("Route53AutoNaming_v20170314.CreatePrivateDnsNamespace", handleCMCreatePrivateDnsNamespace)
	r.Register("Route53AutoNaming_v20170314.GetNamespace", handleCMGetNamespace)
	r.Register("Route53AutoNaming_v20170314.DeleteNamespace", handleCMDeleteNamespace)
	r.Register("Route53AutoNaming_v20170314.CreateService", handleCMCreateService)
	r.Register("Route53AutoNaming_v20170314.GetService", handleCMGetService)
	r.Register("Route53AutoNaming_v20170314.RegisterInstance", handleCMRegisterInstance)
	r.Register("Route53AutoNaming_v20170314.DeregisterInstance", handleCMDeregisterInstance)
	r.Register("Route53AutoNaming_v20170314.ListInstances", handleCMListInstances)
	r.Register("Route53AutoNaming_v20170314.DiscoverInstances", handleCMDiscoverInstances)
	r.Register("Route53AutoNaming_v20170314.GetOperation", handleCMGetOperation)
	r.Register("Route53AutoNaming_v20170314.ListNamespaces", handleCMListNamespaces)
	r.Register("Route53AutoNaming_v20170314.ListServices", handleCMListServices)
	r.Register("Route53AutoNaming_v20170314.ListTagsForResource", handleCMListTagsForResource)
	r.Register("Route53AutoNaming_v20170314.TagResource", handleCMTagResource)
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

	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
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
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusNotFound,
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

	if !cmNamespaces.Delete(req.Id) {
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusNotFound,
			"Namespace '%s' not found", req.Id)
		return
	}

	operationId := generateUUID()
	cmOperations.Put(operationId, CMOperation{
		OperationId: operationId,
		Status:      "SUCCESS",
	})

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
}

func handleCMCreateService(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string       `json:"Name"`
		NamespaceId string       `json:"NamespaceId"`
		Description string       `json:"Description"`
		DnsConfig   *CMDnsConfig `json:"DnsConfig"`
	}
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.AWSError(w, "InvalidInput", "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		sim.AWSError(w, "InvalidInput", "Name is required", http.StatusBadRequest)
		return
	}

	if req.NamespaceId != "" {
		if _, ok := cmNamespaces.Get(req.NamespaceId); !ok {
			sim.AWSErrorf(w, "NamespaceNotFound", http.StatusNotFound,
				"Namespace '%s' not found", req.NamespaceId)
			return
		}
	}

	svcId := "srv-" + generateUUID()[:16]
	svc := CMService{
		Id:          svcId,
		Arn:         cmArn("service", svcId),
		Name:        req.Name,
		NamespaceId: req.NamespaceId,
		Description: req.Description,
		DnsConfig:   req.DnsConfig,
		CreateDate:  time.Now().Unix(),
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
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusNotFound,
			"Service '%s' not found", req.Id)
		return
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Service": svc,
	})
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

	if _, ok := cmServices.Get(req.ServiceId); !ok {
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusNotFound,
			"Service '%s' not found", req.ServiceId)
		return
	}

	instance := CMInstance{
		Id:         req.InstanceId,
		Attributes: req.Attributes,
	}
	key := cmInstanceKey(req.ServiceId, req.InstanceId)
	cmInstances.Put(key, instance)

	// Update service instance count
	cmServices.Update(req.ServiceId, func(svc *CMService) {
		svc.InstanceCount++
	})

	operationId := generateUUID()
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"OperationId": operationId,
	})
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
		sim.AWSErrorf(w, "InstanceNotFound", http.StatusNotFound,
			"Instance '%s' not found", req.InstanceId)
		return
	}

	// Update service instance count
	cmServices.Update(req.ServiceId, func(svc *CMService) {
		if svc.InstanceCount > 0 {
			svc.InstanceCount--
		}
	})

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
	services := cmServices.List()
	if services == nil {
		services = []CMService{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Services": services,
	})
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
	_ = sim.ReadJSON(r, &struct{}{})
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Tags": []any{},
	})
}

func handleCMTagResource(w http.ResponseWriter, r *http.Request) {
	_ = sim.ReadJSON(r, &struct{}{})
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
		sim.AWSErrorf(w, "NamespaceNotFound", http.StatusNotFound,
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
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusNotFound,
			"Service '%s' not found in namespace '%s'", req.ServiceName, req.NamespaceName)
		return
	}

	// Collect all instances for this service
	var httpInstances []map[string]any
	for _, inst := range cmInstances.List() {
		key := cmInstanceKey(targetSvc.Id, inst.Id)
		if _, ok := cmInstances.Get(key); ok {
			httpInstances = append(httpInstances, map[string]any{
				"InstanceId": inst.Id,
				"NamespaceName": req.NamespaceName,
				"ServiceName":   req.ServiceName,
				"HealthStatus":  "HEALTHY",
				"Attributes":    inst.Attributes,
			})
		}
	}
	if httpInstances == nil {
		httpInstances = []map[string]any{}
	}

	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"Instances": httpInstances,
	})
}
