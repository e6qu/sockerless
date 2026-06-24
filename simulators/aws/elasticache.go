package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// ElastiCache — awsQuery protocol. Surface scoped to the 90th-
// percentile lifecycle: CreateCacheCluster, DescribeCacheClusters
// (waiter), ModifyCacheCluster, DeleteCacheCluster, plus tags.
// Engine itself (Redis / Memcached) is not simulated; the sim
// reports Status=available immediately.

type ECCluster struct {
	CacheClusterId         string
	CacheNodeType          string
	Engine                 string
	EngineVersion          string
	CacheClusterStatus     string
	NumCacheNodes          int
	CacheClusterCreateTime string
	ARN                    string
	Endpoint               string
	Port                   int
	Tags                   map[string]string
}

// ECReplicationGroup models the subset of the ReplicationGroup shape
// the SDK + terraform-provider-aws read back after Create/Modify.
type ECReplicationGroup struct {
	ReplicationGroupId     string
	Description            string
	Status                 string
	CacheNodeType          string
	Engine                 string
	AutomaticFailover      string
	MultiAZ                string
	ClusterEnabled         bool
	MemberClusters         []string
	SnapshotRetentionLimit int
	SnapshotWindow         string
	ConfigEndpointAddress  string
	ConfigEndpointPort     int
	ARN                    string
	CreateTime             string
	Tags                   map[string]string
}

// ECSubnetGroup models the CacheSubnetGroup shape.
type ECSubnetGroup struct {
	Name        string
	Description string
	VpcId       string
	SubnetIds   []string
	ARN         string
	Tags        map[string]string
}

// ECParameterGroup models the CacheParameterGroup shape.
type ECParameterGroup struct {
	Name        string
	Family      string
	Description string
	ARN         string
	Tags        map[string]string
}

var (
	ecClusters    sim.Store[ECCluster]
	ecReplGroups  sim.Store[ECReplicationGroup]
	ecSubnetGrps  sim.Store[ECSubnetGroup]
	ecParamGroups sim.Store[ECParameterGroup]
)

// ecAPIVersion is the canonical AWS ElastiCache API version (Query
// Protocol). Used to disambiguate Action names from other awsQuery
// services in the AWSQueryRouter dispatch.
const ecAPIVersion = "2015-02-02"

func registerElastiCache(r *sim.AWSQueryRouter, srv *sim.Server) {
	ecClusters = sim.MakeStore[ECCluster](srv.DB(), "elasticache_clusters")
	ecReplGroups = sim.MakeStore[ECReplicationGroup](srv.DB(), "elasticache_replication_groups")
	ecSubnetGrps = sim.MakeStore[ECSubnetGroup](srv.DB(), "elasticache_subnet_groups")
	ecParamGroups = sim.MakeStore[ECParameterGroup](srv.DB(), "elasticache_parameter_groups")
	r.RegisterVersioned(ecAPIVersion, "CreateCacheCluster", handleECCreate)
	r.RegisterVersioned(ecAPIVersion, "DescribeCacheClusters", handleECDescribe)
	r.RegisterVersioned(ecAPIVersion, "ModifyCacheCluster", handleECModify)
	r.RegisterVersioned(ecAPIVersion, "DeleteCacheCluster", handleECDelete)
	r.RegisterVersioned(ecAPIVersion, "RebootCacheCluster", handleECReboot)
	r.RegisterVersioned(ecAPIVersion, "CreateReplicationGroup", handleECCreateReplGroup)
	r.RegisterVersioned(ecAPIVersion, "DescribeReplicationGroups", handleECDescribeReplGroups)
	r.RegisterVersioned(ecAPIVersion, "ModifyReplicationGroup", handleECModifyReplGroup)
	r.RegisterVersioned(ecAPIVersion, "DeleteReplicationGroup", handleECDeleteReplGroup)
	r.RegisterVersioned(ecAPIVersion, "CreateCacheSubnetGroup", handleECCreateSubnetGroup)
	r.RegisterVersioned(ecAPIVersion, "DescribeCacheSubnetGroups", handleECDescribeSubnetGroups)
	r.RegisterVersioned(ecAPIVersion, "ModifyCacheSubnetGroup", handleECModifySubnetGroup)
	r.RegisterVersioned(ecAPIVersion, "DeleteCacheSubnetGroup", handleECDeleteSubnetGroup)
	r.RegisterVersioned(ecAPIVersion, "CreateCacheParameterGroup", handleECCreateParamGroup)
	r.RegisterVersioned(ecAPIVersion, "DescribeCacheParameterGroups", handleECDescribeParamGroups)
	r.RegisterVersioned(ecAPIVersion, "DeleteCacheParameterGroup", handleECDeleteParamGroup)
	r.RegisterVersioned(ecAPIVersion, "AddTagsToResource", handleECAddTags)
	r.RegisterVersioned(ecAPIVersion, "ListTagsForResource", handleECListTags)
	r.RegisterVersioned(ecAPIVersion, "RemoveTagsFromResource", handleECRemoveTags)
}

func ecClusterARN(id string) string {
	return fmt.Sprintf("arn:aws:elasticache:%s:%s:cluster:%s", awsRegion(), awsAccountID(), id)
}

func ecReplGroupARN(id string) string {
	return fmt.Sprintf("arn:aws:elasticache:%s:%s:replicationgroup:%s", awsRegion(), awsAccountID(), id)
}

func ecSubnetGroupARN(name string) string {
	return fmt.Sprintf("arn:aws:elasticache:%s:%s:subnetgroup:%s", awsRegion(), awsAccountID(), name)
}

func ecParamGroupARN(name string) string {
	return fmt.Sprintf("arn:aws:elasticache:%s:%s:parametergroup:%s", awsRegion(), awsAccountID(), name)
}

func ecXMLResponse(w http.ResponseWriter, op string, body string, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w,
		`<%sResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/"><%sResult>%s</%sResult><ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata></%sResponse>`,
		op, op, body, op, requestID, op)
}

func ecErrorXML(w http.ResponseWriter, code, message string, status int, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w,
		`<ErrorResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
		code, message, requestID)
}

func renderECCluster(c ECCluster) string {
	var b strings.Builder
	b.WriteString("<CacheCluster>")
	fmt.Fprintf(&b, "<CacheClusterId>%s</CacheClusterId>", xmlEscape(c.CacheClusterId))
	fmt.Fprintf(&b, "<CacheNodeType>%s</CacheNodeType>", xmlEscape(c.CacheNodeType))
	fmt.Fprintf(&b, "<Engine>%s</Engine>", xmlEscape(c.Engine))
	fmt.Fprintf(&b, "<EngineVersion>%s</EngineVersion>", xmlEscape(c.EngineVersion))
	fmt.Fprintf(&b, "<CacheClusterStatus>%s</CacheClusterStatus>", xmlEscape(c.CacheClusterStatus))
	fmt.Fprintf(&b, "<NumCacheNodes>%d</NumCacheNodes>", c.NumCacheNodes)
	fmt.Fprintf(&b, "<CacheClusterCreateTime>%s</CacheClusterCreateTime>", xmlEscape(c.CacheClusterCreateTime))
	fmt.Fprintf(&b, "<ARN>%s</ARN>", xmlEscape(c.ARN))
	fmt.Fprintf(&b, "<ConfigurationEndpoint><Address>%s</Address><Port>%d</Port></ConfigurationEndpoint>", xmlEscape(c.Endpoint), c.Port)
	b.WriteString("</CacheCluster>")
	return b.String()
}

func handleECCreate(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("CacheClusterId")
	if id == "" {
		ecErrorXML(w, "MissingParameter", "CacheClusterId is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := ecClusters.Get(id); ok {
		ecErrorXML(w, "CacheClusterAlreadyExists", "Cluster already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	engine := r.FormValue("Engine")
	port := 6379
	if engine == "memcached" {
		port = 11211
	}
	if p := atoiOrZero(r.FormValue("Port")); p > 0 {
		port = p
	}
	num := atoiOrZero(r.FormValue("NumCacheNodes"))
	if num == 0 {
		num = 1
	}
	engineVersion := r.FormValue("EngineVersion")
	if engineVersion == "" {
		engineVersion = ecDefaultEngineVersion(engine)
	}
	c := ECCluster{
		CacheClusterId:         id,
		CacheNodeType:          r.FormValue("CacheNodeType"),
		Engine:                 engine,
		EngineVersion:          engineVersion,
		CacheClusterStatus:     "available",
		NumCacheNodes:          num,
		CacheClusterCreateTime: time.Now().UTC().Format(time.RFC3339),
		ARN:                    ecClusterARN(id),
		Endpoint:               fmt.Sprintf("%s.%s.cache.amazonaws.com", id, awsRegion()),
		Port:                   port,
		Tags:                   parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	ecClusters.Put(id, c)
	ecXMLResponse(w, "CreateCacheCluster", renderECCluster(c), sim.RequestID(r.Context()))
}

func handleECDescribe(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("CacheClusterId")
	var b strings.Builder
	b.WriteString("<CacheClusters>")
	matched := false
	for _, c := range ecClusters.List() {
		if wanted != "" && c.CacheClusterId != wanted {
			continue
		}
		matched = true
		b.WriteString(renderECCluster(c))
	}
	if wanted != "" && !matched {
		ecErrorXML(w, "CacheClusterNotFound", fmt.Sprintf("Cache cluster %q not found", wanted), http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</CacheClusters>")
	ecXMLResponse(w, "DescribeCacheClusters", b.String(), sim.RequestID(r.Context()))
}

func handleECModify(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("CacheClusterId")
	if _, ok := ecClusters.Get(id); !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Cluster not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecClusters.Update(id, func(c *ECCluster) {
		if v := r.FormValue("CacheNodeType"); v != "" {
			c.CacheNodeType = v
		}
		if v := r.FormValue("EngineVersion"); v != "" {
			c.EngineVersion = v
		}
		if v := r.FormValue("NumCacheNodes"); v != "" {
			c.NumCacheNodes = atoiOrZero(v)
		}
	})
	updated, _ := ecClusters.Get(id)
	ecXMLResponse(w, "ModifyCacheCluster", renderECCluster(updated), sim.RequestID(r.Context()))
}

func handleECDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("CacheClusterId")
	c, ok := ecClusters.Get(id)
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Cluster not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	c.CacheClusterStatus = "deleting"
	ecClusters.Delete(id)
	ecXMLResponse(w, "DeleteCacheCluster", renderECCluster(c), sim.RequestID(r.Context()))
}

func handleECReboot(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("CacheClusterId")
	if _, ok := ecClusters.Get(id); !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Cluster not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecClusters.Update(id, func(c *ECCluster) {
		c.CacheClusterStatus = "rebooting cache cluster nodes"
	})
	updated, _ := ecClusters.Get(id)
	ecXMLResponse(w, "RebootCacheCluster", renderECCluster(updated), sim.RequestID(r.Context()))
}

func renderECReplGroup(g ECReplicationGroup) string {
	var b strings.Builder
	b.WriteString("<ReplicationGroup>")
	fmt.Fprintf(&b, "<ReplicationGroupId>%s</ReplicationGroupId>", xmlEscape(g.ReplicationGroupId))
	fmt.Fprintf(&b, "<Description>%s</Description>", xmlEscape(g.Description))
	fmt.Fprintf(&b, "<Status>%s</Status>", xmlEscape(g.Status))
	fmt.Fprintf(&b, "<CacheNodeType>%s</CacheNodeType>", xmlEscape(g.CacheNodeType))
	fmt.Fprintf(&b, "<Engine>%s</Engine>", xmlEscape(g.Engine))
	fmt.Fprintf(&b, "<AutomaticFailover>%s</AutomaticFailover>", xmlEscape(g.AutomaticFailover))
	fmt.Fprintf(&b, "<MultiAZ>%s</MultiAZ>", xmlEscape(g.MultiAZ))
	fmt.Fprintf(&b, "<ClusterEnabled>%t</ClusterEnabled>", g.ClusterEnabled)
	fmt.Fprintf(&b, "<SnapshotRetentionLimit>%d</SnapshotRetentionLimit>", g.SnapshotRetentionLimit)
	if g.SnapshotWindow != "" {
		fmt.Fprintf(&b, "<SnapshotWindow>%s</SnapshotWindow>", xmlEscape(g.SnapshotWindow))
	}
	fmt.Fprintf(&b, "<ReplicationGroupCreateTime>%s</ReplicationGroupCreateTime>", xmlEscape(g.CreateTime))
	fmt.Fprintf(&b, "<ARN>%s</ARN>", xmlEscape(g.ARN))
	b.WriteString("<MemberClusters>")
	for _, m := range g.MemberClusters {
		fmt.Fprintf(&b, "<ClusterId>%s</ClusterId>", xmlEscape(m))
	}
	b.WriteString("</MemberClusters>")
	fmt.Fprintf(&b, "<ConfigurationEndpoint><Address>%s</Address><Port>%d</Port></ConfigurationEndpoint>", xmlEscape(g.ConfigEndpointAddress), g.ConfigEndpointPort)
	b.WriteString("</ReplicationGroup>")
	return b.String()
}

func handleECCreateReplGroup(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("ReplicationGroupId")
	if id == "" {
		ecErrorXML(w, "MissingParameter", "ReplicationGroupId is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := ecReplGroups.Get(id); ok {
		ecErrorXML(w, "ReplicationGroupAlreadyExistsFault", "Replication group already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	engine := r.FormValue("Engine")
	if engine == "" {
		engine = "redis"
	}
	port := 6379
	if p := atoiOrZero(r.FormValue("Port")); p > 0 {
		port = p
	}
	failover := "disabled"
	if strings.EqualFold(r.FormValue("AutomaticFailoverEnabled"), "true") {
		failover = "enabled"
	}
	multiAZ := "disabled"
	if strings.EqualFold(r.FormValue("MultiAZEnabled"), "true") {
		multiAZ = "enabled"
	}
	clusterEnabled := strings.EqualFold(r.FormValue("ClusterMode"), "enabled") || atoiOrZero(r.FormValue("NumNodeGroups")) > 1
	num := atoiOrZero(r.FormValue("NumCacheClusters"))
	if num == 0 {
		num = 1
	}
	members := make([]string, 0, num)
	for i := 1; i <= num; i++ {
		members = append(members, fmt.Sprintf("%s-%03d", id, i))
	}
	g := ECReplicationGroup{
		ReplicationGroupId:     id,
		Description:            r.FormValue("ReplicationGroupDescription"),
		Status:                 "available",
		CacheNodeType:          r.FormValue("CacheNodeType"),
		Engine:                 engine,
		AutomaticFailover:      failover,
		MultiAZ:                multiAZ,
		ClusterEnabled:         clusterEnabled,
		MemberClusters:         members,
		SnapshotRetentionLimit: atoiOrZero(r.FormValue("SnapshotRetentionLimit")),
		SnapshotWindow:         r.FormValue("SnapshotWindow"),
		ConfigEndpointAddress:  fmt.Sprintf("clustercfg.%s.%s.cache.amazonaws.com", id, awsRegion()),
		ConfigEndpointPort:     port,
		ARN:                    ecReplGroupARN(id),
		CreateTime:             time.Now().UTC().Format(time.RFC3339),
		Tags:                   parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	ecReplGroups.Put(id, g)
	ecXMLResponse(w, "CreateReplicationGroup", renderECReplGroup(g), sim.RequestID(r.Context()))
}

func handleECDescribeReplGroups(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("ReplicationGroupId")
	var b strings.Builder
	b.WriteString("<ReplicationGroups>")
	matched := false
	for _, g := range ecReplGroups.List() {
		if wanted != "" && g.ReplicationGroupId != wanted {
			continue
		}
		matched = true
		b.WriteString(renderECReplGroup(g))
	}
	if wanted != "" && !matched {
		ecErrorXML(w, "ReplicationGroupNotFoundFault", fmt.Sprintf("Replication group %q not found", wanted), http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</ReplicationGroups>")
	ecXMLResponse(w, "DescribeReplicationGroups", b.String(), sim.RequestID(r.Context()))
}

func handleECModifyReplGroup(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("ReplicationGroupId")
	if _, ok := ecReplGroups.Get(id); !ok {
		ecErrorXML(w, "ReplicationGroupNotFoundFault", "Replication group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecReplGroups.Update(id, func(g *ECReplicationGroup) {
		if v := r.FormValue("ReplicationGroupDescription"); v != "" {
			g.Description = v
		}
		if v := r.FormValue("CacheNodeType"); v != "" {
			g.CacheNodeType = v
		}
		if v := r.FormValue("SnapshotRetentionLimit"); v != "" {
			g.SnapshotRetentionLimit = atoiOrZero(v)
		}
		if v := r.FormValue("SnapshotWindow"); v != "" {
			g.SnapshotWindow = v
		}
		if v := r.FormValue("AutomaticFailoverEnabled"); v != "" {
			if strings.EqualFold(v, "true") {
				g.AutomaticFailover = "enabled"
			} else {
				g.AutomaticFailover = "disabled"
			}
		}
		if v := r.FormValue("MultiAZEnabled"); v != "" {
			if strings.EqualFold(v, "true") {
				g.MultiAZ = "enabled"
			} else {
				g.MultiAZ = "disabled"
			}
		}
	})
	updated, _ := ecReplGroups.Get(id)
	ecXMLResponse(w, "ModifyReplicationGroup", renderECReplGroup(updated), sim.RequestID(r.Context()))
}

func handleECDeleteReplGroup(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("ReplicationGroupId")
	g, ok := ecReplGroups.Get(id)
	if !ok {
		ecErrorXML(w, "ReplicationGroupNotFoundFault", "Replication group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	g.Status = "deleting"
	ecReplGroups.Delete(id)
	ecXMLResponse(w, "DeleteReplicationGroup", renderECReplGroup(g), sim.RequestID(r.Context()))
}

func renderECSubnetGroup(g ECSubnetGroup) string {
	var b strings.Builder
	b.WriteString("<CacheSubnetGroup>")
	fmt.Fprintf(&b, "<CacheSubnetGroupName>%s</CacheSubnetGroupName>", xmlEscape(g.Name))
	fmt.Fprintf(&b, "<CacheSubnetGroupDescription>%s</CacheSubnetGroupDescription>", xmlEscape(g.Description))
	fmt.Fprintf(&b, "<VpcId>%s</VpcId>", xmlEscape(g.VpcId))
	fmt.Fprintf(&b, "<ARN>%s</ARN>", xmlEscape(g.ARN))
	b.WriteString("<Subnets>")
	for _, s := range g.SubnetIds {
		fmt.Fprintf(&b, "<Subnet><SubnetIdentifier>%s</SubnetIdentifier><SubnetAvailabilityZone><Name>%s</Name></SubnetAvailabilityZone></Subnet>", xmlEscape(s), xmlEscape(awsRegion()+"a"))
	}
	b.WriteString("</Subnets>")
	b.WriteString("</CacheSubnetGroup>")
	return b.String()
}

func ecParseSubnetIds(r *http.Request) []string {
	var ids []string
	for n := 1; n <= 50; n++ {
		v := r.FormValue(fmt.Sprintf("SubnetIds.SubnetIdentifier.%d", n))
		if v == "" {
			v = r.FormValue(fmt.Sprintf("SubnetIds.member.%d", n))
		}
		if v == "" {
			break
		}
		ids = append(ids, v)
	}
	return ids
}

func handleECCreateSubnetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("CacheSubnetGroupName")
	if name == "" {
		ecErrorXML(w, "MissingParameter", "CacheSubnetGroupName is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := ecSubnetGrps.Get(name); ok {
		ecErrorXML(w, "CacheSubnetGroupAlreadyExistsFault", "Cache subnet group already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	g := ECSubnetGroup{
		Name:        name,
		Description: r.FormValue("CacheSubnetGroupDescription"),
		VpcId:       "vpc-" + sim.RequestID(r.Context())[:8],
		SubnetIds:   ecParseSubnetIds(r),
		ARN:         ecSubnetGroupARN(name),
		Tags:        parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	ecSubnetGrps.Put(name, g)
	ecXMLResponse(w, "CreateCacheSubnetGroup", renderECSubnetGroup(g), sim.RequestID(r.Context()))
}

func handleECDescribeSubnetGroups(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("CacheSubnetGroupName")
	var b strings.Builder
	b.WriteString("<CacheSubnetGroups>")
	matched := false
	for _, g := range ecSubnetGrps.List() {
		if wanted != "" && g.Name != wanted {
			continue
		}
		matched = true
		b.WriteString(renderECSubnetGroup(g))
	}
	if wanted != "" && !matched {
		ecErrorXML(w, "CacheSubnetGroupNotFoundFault", fmt.Sprintf("Cache subnet group %q not found", wanted), http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</CacheSubnetGroups>")
	ecXMLResponse(w, "DescribeCacheSubnetGroups", b.String(), sim.RequestID(r.Context()))
}

func handleECModifySubnetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("CacheSubnetGroupName")
	if _, ok := ecSubnetGrps.Get(name); !ok {
		ecErrorXML(w, "CacheSubnetGroupNotFoundFault", "Cache subnet group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecSubnetGrps.Update(name, func(g *ECSubnetGroup) {
		if v := r.FormValue("CacheSubnetGroupDescription"); v != "" {
			g.Description = v
		}
		if ids := ecParseSubnetIds(r); len(ids) > 0 {
			g.SubnetIds = ids
		}
	})
	updated, _ := ecSubnetGrps.Get(name)
	ecXMLResponse(w, "ModifyCacheSubnetGroup", renderECSubnetGroup(updated), sim.RequestID(r.Context()))
}

func handleECDeleteSubnetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("CacheSubnetGroupName")
	if _, ok := ecSubnetGrps.Get(name); !ok {
		ecErrorXML(w, "CacheSubnetGroupNotFoundFault", "Cache subnet group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecSubnetGrps.Delete(name)
	ecXMLResponse(w, "DeleteCacheSubnetGroup", "", sim.RequestID(r.Context()))
}

func renderECParamGroup(g ECParameterGroup) string {
	var b strings.Builder
	b.WriteString("<CacheParameterGroup>")
	fmt.Fprintf(&b, "<CacheParameterGroupName>%s</CacheParameterGroupName>", xmlEscape(g.Name))
	fmt.Fprintf(&b, "<CacheParameterGroupFamily>%s</CacheParameterGroupFamily>", xmlEscape(g.Family))
	fmt.Fprintf(&b, "<Description>%s</Description>", xmlEscape(g.Description))
	fmt.Fprintf(&b, "<ARN>%s</ARN>", xmlEscape(g.ARN))
	b.WriteString("</CacheParameterGroup>")
	return b.String()
}

func handleECCreateParamGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("CacheParameterGroupName")
	if name == "" {
		ecErrorXML(w, "MissingParameter", "CacheParameterGroupName is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := ecParamGroups.Get(name); ok {
		ecErrorXML(w, "CacheParameterGroupAlreadyExistsFault", "Cache parameter group already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	g := ECParameterGroup{
		Name:        name,
		Family:      r.FormValue("CacheParameterGroupFamily"),
		Description: r.FormValue("Description"),
		ARN:         ecParamGroupARN(name),
		Tags:        parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	ecParamGroups.Put(name, g)
	ecXMLResponse(w, "CreateCacheParameterGroup", renderECParamGroup(g), sim.RequestID(r.Context()))
}

func handleECDescribeParamGroups(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("CacheParameterGroupName")
	var b strings.Builder
	b.WriteString("<CacheParameterGroups>")
	matched := false
	for _, g := range ecParamGroups.List() {
		if wanted != "" && g.Name != wanted {
			continue
		}
		matched = true
		b.WriteString(renderECParamGroup(g))
	}
	if wanted != "" && !matched {
		ecErrorXML(w, "CacheParameterGroupNotFoundFault", fmt.Sprintf("Cache parameter group %q not found", wanted), http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</CacheParameterGroups>")
	ecXMLResponse(w, "DescribeCacheParameterGroups", b.String(), sim.RequestID(r.Context()))
}

func handleECDeleteParamGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("CacheParameterGroupName")
	if _, ok := ecParamGroups.Get(name); !ok {
		ecErrorXML(w, "CacheParameterGroupNotFoundFault", "Cache parameter group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecParamGroups.Delete(name)
	ecXMLResponse(w, "DeleteCacheParameterGroup", "", sim.RequestID(r.Context()))
}

// ecMutateTags resolves an ElastiCache ARN (cluster, replication
// group, cache subnet group, or cache parameter group) and applies fn
// to that resource's tag map, persisting the change. It returns the
// resulting tag map and whether the resource was found.
func ecMutateTags(arn string, fn func(map[string]string)) (map[string]string, bool) {
	for _, c := range ecClusters.List() {
		if c.ARN == arn {
			ecClusters.Update(c.CacheClusterId, func(cc *ECCluster) {
				if cc.Tags == nil {
					cc.Tags = map[string]string{}
				}
				fn(cc.Tags)
			})
			updated, _ := ecClusters.Get(c.CacheClusterId)
			return updated.Tags, true
		}
	}
	for _, g := range ecReplGroups.List() {
		if g.ARN == arn {
			ecReplGroups.Update(g.ReplicationGroupId, func(gg *ECReplicationGroup) {
				if gg.Tags == nil {
					gg.Tags = map[string]string{}
				}
				fn(gg.Tags)
			})
			updated, _ := ecReplGroups.Get(g.ReplicationGroupId)
			return updated.Tags, true
		}
	}
	for _, g := range ecSubnetGrps.List() {
		if g.ARN == arn {
			ecSubnetGrps.Update(g.Name, func(gg *ECSubnetGroup) {
				if gg.Tags == nil {
					gg.Tags = map[string]string{}
				}
				fn(gg.Tags)
			})
			updated, _ := ecSubnetGrps.Get(g.Name)
			return updated.Tags, true
		}
	}
	for _, g := range ecParamGroups.List() {
		if g.ARN == arn {
			ecParamGroups.Update(g.Name, func(gg *ECParameterGroup) {
				if gg.Tags == nil {
					gg.Tags = map[string]string{}
				}
				fn(gg.Tags)
			})
			updated, _ := ecParamGroups.Get(g.Name)
			return updated.Tags, true
		}
	}
	return nil, false
}

func ecRenderTagList(tags map[string]string) string {
	var b strings.Builder
	b.WriteString("<TagList>")
	for k, v := range tags {
		fmt.Fprintf(&b, "<Tag><Key>%s</Key><Value>%s</Value></Tag>", xmlEscape(k), xmlEscape(v))
	}
	b.WriteString("</TagList>")
	return b.String()
}

func handleECAddTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	tags, ok := ecMutateTags(arn, func(m map[string]string) {
		for n := 1; n <= 50; n++ {
			k := r.FormValue(fmt.Sprintf("Tags.Tag.%d.Key", n))
			v := r.FormValue(fmt.Sprintf("Tags.Tag.%d.Value", n))
			if k == "" {
				break
			}
			m[k] = v
		}
	})
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecXMLResponse(w, "AddTagsToResource", ecRenderTagList(tags), sim.RequestID(r.Context()))
}

func handleECListTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	tags, ok := ecLookupTags(arn)
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecXMLResponse(w, "ListTagsForResource", ecRenderTagList(tags), sim.RequestID(r.Context()))
}

// ecLookupTags resolves an ElastiCache ARN to its tag map without
// mutating it.
func ecLookupTags(arn string) (map[string]string, bool) {
	for _, c := range ecClusters.List() {
		if c.ARN == arn {
			return c.Tags, true
		}
	}
	for _, g := range ecReplGroups.List() {
		if g.ARN == arn {
			return g.Tags, true
		}
	}
	for _, g := range ecSubnetGrps.List() {
		if g.ARN == arn {
			return g.Tags, true
		}
	}
	for _, g := range ecParamGroups.List() {
		if g.ARN == arn {
			return g.Tags, true
		}
	}
	return nil, false
}

func handleECRemoveTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	tags, ok := ecMutateTags(arn, func(m map[string]string) {
		for n := 1; n <= 50; n++ {
			k := r.FormValue(fmt.Sprintf("TagKeys.member.%d", n))
			if k == "" {
				break
			}
			delete(m, k)
		}
	})
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecXMLResponse(w, "RemoveTagsFromResource", ecRenderTagList(tags), sim.RequestID(r.Context()))
}

// ecDefaultEngineVersion returns the engine's current GA major
// version when CreateCacheCluster omits EngineVersion. Real
// ElastiCache populates the default server-side; the
// terraform-provider-aws resource captures the resolved value into
// state, so an empty echo produces drift on next plan.
func ecDefaultEngineVersion(engine string) string {
	switch engine {
	case "redis":
		return "7.1"
	case "valkey":
		return "8.0"
	case "memcached":
		return "1.6.22"
	}
	return ""
}
