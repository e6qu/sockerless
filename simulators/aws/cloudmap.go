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
	DnsProperties *CMDnsProperties `json:"DnsProperties,omitempty"`
}

type CMDnsProperties struct {
	HostedZoneId string `json:"HostedZoneId"`
	SOA          *struct {
		TTL int64 `json:"TTL"`
	} `json:"SOA,omitempty"`
}

type CMService struct {
	Id            string       `json:"Id"`
	Arn           string       `json:"Arn"`
	Name          string       `json:"Name"`
	NamespaceId   string       `json:"NamespaceId"`
	Description   string       `json:"Description,omitempty"`
	DnsConfig     *CMDnsConfig `json:"DnsConfig,omitempty"`
	CreateDate    int64        `json:"CreateDate"`
	InstanceCount int          `json:"InstanceCount"`
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
}

type CMOperation struct {
	OperationId string            `json:"OperationId"`
	Status      string            `json:"Status"`
	Targets     map[string]string `json:"Targets,omitempty"`
}

// State stores
var (
	cmNamespaces    sim.Store[CMNamespace]
	cmNamespaceVPCs sim.Store[string]
	cmServices      sim.Store[CMService]
	cmInstances     sim.Store[CMInstance]
	cmOperations    sim.Store[CMOperation]
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
	r.Register("Route53AutoNaming_v20170314.DeleteService", handleCMDeleteService)
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
	cmNamespaceVPCs.Put(nsId, req.Vpc)

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
			sim.AWSErrorf(w, "NamespaceNotFound", http.StatusBadRequest,
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
		sim.AWSErrorf(w, "ServiceNotFound", http.StatusBadRequest,
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
	rollback := func() {
		if !existed {
			cmInstances.Delete(key)
			cmServices.Update(req.ServiceId, func(svc *CMService) {
				if svc.InstanceCount > 0 {
					svc.InstanceCount--
				}
			})
		}
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

// cmDockerAliasesForContainer returns every Cloud Map service name in the
// namespace whose registered instance maps to containerName — the full set of
// DNS aliases the container must answer to on the namespace's Docker network.
// This is the Docker-network-tier analogue of syncCMNamespaceHosts (which does
// the same aggregation for the netns/host-entries tier).
func cmDockerAliasesForContainer(namespaceID, containerName string) []string {
	seen := make(map[string]struct{})
	var aliases []string
	for _, svc := range cmServices.List() {
		if svc.NamespaceId != namespaceID {
			continue
		}
		for _, inst := range cmInstances.List() {
			if _, ok := cmInstances.Get(cmInstanceKey(svc.Id, inst.Id)); !ok {
				continue
			}
			if resolveTaskContainerForInstance(inst.Id) != containerName {
				continue
			}
			if _, dup := seen[svc.Name]; dup {
				continue
			}
			seen[svc.Name] = struct{}{}
			aliases = append(aliases, svc.Name)
		}
	}
	return aliases
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

// resolveTaskContainerForInstance maps a Cloud Map instance ID back to the
// simulator's Docker container. Sockerless's ECS backend uses `containerID[:12]`
// as the instance ID and tags each RunTask with `sockerless-container-id: <full
// id>`. On the netns awsvpc fabric the pause container owns the namespace; that
// tier cannot be connected to Docker's DNS network after the real ENI occupies
// eth0, so Cloud Map uses host entries in the task container instead.
func resolveTaskContainerForInstance(instanceId string) string {
	for _, task := range ecsTasks.List() {
		for _, tag := range task.Tags {
			if tag.Key == "sockerless-container-id" && len(tag.Value) >= len(instanceId) && tag.Value[:len(instanceId)] == instanceId {
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
	var httpInstances []map[string]any
	for _, inst := range cmInstances.List() {
		key := cmInstanceKey(targetSvc.Id, inst.Id)
		if _, ok := cmInstances.Get(key); ok {
			httpInstances = append(httpInstances, map[string]any{
				"InstanceId":    inst.Id,
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
