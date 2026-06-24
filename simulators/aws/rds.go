package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// RDS — awsQuery protocol. Surface scoped to the 90th-percentile
// terraform-provider-aws + SDK lifecycle: CreateDBInstance,
// DescribeDBInstances (waiter-driven), ModifyDBInstance,
// DeleteDBInstance, AddTagsToResource, ListTagsForResource,
// RemoveTagsFromResource, CreateDBSnapshot, DescribeDBSnapshots,
// DescribeDBSnapshotAttributes, DeleteDBSnapshot, and
// RestoreDBInstanceFromDBSnapshot. Database engine itself is not
// simulated; the sim returns Status=available immediately on Create.

type RDSInstance struct {
	DBInstanceIdentifier string
	DbiResourceId        string
	DBInstanceClass      string
	Engine               string
	EngineVersion        string
	DBInstanceStatus     string
	MasterUsername       string
	DBName               string
	AllocatedStorage     int
	Endpoint             string
	Port                 int
	AvailabilityZone     string
	InstanceCreateTime   string
	ARN                  string
	Tags                 map[string]string
}

// RDSSnapshot models the canonical RDS DB snapshot state machine:
//
//	(CreateDBSnapshot)        → creating
//	(internal-settle on read) → available
//	(DeleteDBSnapshot)        → deleted (removed from store)
//
// The sim collapses the creating→available transition into an
// inline-settle: every snapshot row is written with Status=available
// from the start. See sim-state-machine-completeness skill — when a
// transition is collapsed, document the choice so future maintainers
// don't read "available" and assume the transient state doesn't
// exist on real RDS.
type RDSSnapshot struct {
	DBSnapshotIdentifier string
	DBInstanceIdentifier string
	DbiResourceId        string
	Engine               string
	EngineVersion        string
	Status               string // creating | available | deleting | failed
	AllocatedStorage     int
	MasterUsername       string
	SnapshotCreateTime   string
	SnapshotType         string // manual | automated
	ARN                  string
	Tags                 map[string]string
}

// RDSCluster models a (control-plane only) Aurora/Multi-AZ DB cluster.
// The database engine is not simulated; Status settles to "available"
// inline on Create, matching the sim's instance/snapshot convention.
type RDSCluster struct {
	DBClusterIdentifier        string
	DbClusterResourceId        string
	Engine                     string
	EngineVersion              string
	EngineMode                 string
	Status                     string
	DatabaseName               string
	MasterUsername             string
	Port                       int
	Endpoint                   string
	ReaderEndpoint             string
	DBClusterParameterGroup    string
	DBSubnetGroup              string
	AllocatedStorage           int
	BackupRetentionPeriod      int
	StorageEncrypted           bool
	DeletionProtection         bool
	ClusterCreateTime          string
	AvailabilityZones          []string
	PreferredBackupWindow      string
	PreferredMaintenanceWindow string
	ARN                        string
	Tags                       map[string]string
}

// RDSSubnetGroup models a DB subnet group (a named set of VPC subnets
// RDS places DB instances into).
type RDSSubnetGroup struct {
	DBSubnetGroupName        string
	DBSubnetGroupDescription string
	VpcId                    string
	SubnetGroupStatus        string
	SubnetIds                []string
	ARN                      string
	Tags                     map[string]string
}

// RDSParamGroup models a DB parameter group. Individual parameters are
// not simulated (ModifyDBParameterGroup is out of the supported
// surface); the group itself is a faithful control-plane row.
type RDSParamGroup struct {
	DBParameterGroupName   string
	DBParameterGroupFamily string
	Description            string
	ARN                    string
	Tags                   map[string]string
}

var (
	rdsInstances    sim.Store[RDSInstance]
	rdsSnapshots    sim.Store[RDSSnapshot]
	rdsClusters     sim.Store[RDSCluster]
	rdsSubnetGroups sim.Store[RDSSubnetGroup]
	rdsParamGroups  sim.Store[RDSParamGroup]
)

// rdsAPIVersion is the canonical AWS RDS API version (Query
// Protocol). Used to disambiguate Action names from other awsQuery
// services in the AWSQueryRouter dispatch.
const rdsAPIVersion = "2014-10-31"

func registerRDS(r *sim.AWSQueryRouter, srv *sim.Server) {
	rdsInstances = sim.MakeStore[RDSInstance](srv.DB(), "rds_instances")
	rdsSnapshots = sim.MakeStore[RDSSnapshot](srv.DB(), "rds_snapshots")
	r.RegisterVersioned(rdsAPIVersion, "CreateDBInstance", handleRDSCreate)
	r.RegisterVersioned(rdsAPIVersion, "DescribeDBInstances", handleRDSDescribe)
	r.RegisterVersioned(rdsAPIVersion, "ModifyDBInstance", handleRDSModify)
	r.RegisterVersioned(rdsAPIVersion, "DeleteDBInstance", handleRDSDelete)
	r.RegisterVersioned(rdsAPIVersion, "AddTagsToResource", handleRDSAddTags)
	r.RegisterVersioned(rdsAPIVersion, "ListTagsForResource", handleRDSListTags)
	r.RegisterVersioned(rdsAPIVersion, "RemoveTagsFromResource", handleRDSRemoveTags)
	r.RegisterVersioned(rdsAPIVersion, "CreateDBSnapshot", handleRDSCreateSnapshot)
	r.RegisterVersioned(rdsAPIVersion, "DescribeDBSnapshots", handleRDSDescribeSnapshots)
	r.RegisterVersioned(rdsAPIVersion, "DescribeDBSnapshotAttributes", handleRDSDescribeSnapshotAttributes)
	r.RegisterVersioned(rdsAPIVersion, "DeleteDBSnapshot", handleRDSDeleteSnapshot)
	r.RegisterVersioned(rdsAPIVersion, "RestoreDBInstanceFromDBSnapshot", handleRDSRestoreFromSnapshot)
	r.RegisterVersioned(rdsAPIVersion, "RebootDBInstance", handleRDSReboot)

	rdsClusters = sim.MakeStore[RDSCluster](srv.DB(), "rds_clusters")
	r.RegisterVersioned(rdsAPIVersion, "CreateDBCluster", handleRDSCreateCluster)
	r.RegisterVersioned(rdsAPIVersion, "DescribeDBClusters", handleRDSDescribeClusters)
	r.RegisterVersioned(rdsAPIVersion, "ModifyDBCluster", handleRDSModifyCluster)
	r.RegisterVersioned(rdsAPIVersion, "DeleteDBCluster", handleRDSDeleteCluster)

	rdsSubnetGroups = sim.MakeStore[RDSSubnetGroup](srv.DB(), "rds_subnet_groups")
	r.RegisterVersioned(rdsAPIVersion, "CreateDBSubnetGroup", handleRDSCreateSubnetGroup)
	r.RegisterVersioned(rdsAPIVersion, "DescribeDBSubnetGroups", handleRDSDescribeSubnetGroups)
	r.RegisterVersioned(rdsAPIVersion, "ModifyDBSubnetGroup", handleRDSModifySubnetGroup)
	r.RegisterVersioned(rdsAPIVersion, "DeleteDBSubnetGroup", handleRDSDeleteSubnetGroup)

	rdsParamGroups = sim.MakeStore[RDSParamGroup](srv.DB(), "rds_param_groups")
	r.RegisterVersioned(rdsAPIVersion, "CreateDBParameterGroup", handleRDSCreateParamGroup)
	r.RegisterVersioned(rdsAPIVersion, "DescribeDBParameterGroups", handleRDSDescribeParamGroups)
	r.RegisterVersioned(rdsAPIVersion, "DeleteDBParameterGroup", handleRDSDeleteParamGroup)
}

func rdsInstanceARN(id string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:db:%s", awsRegion(), awsAccountID(), id)
}

func rdsResourceID() string {
	return "db-" + strings.ToUpper(strings.ReplaceAll(generateUUID(), "-", ""))[:26]
}

func rdsXMLResponse(w http.ResponseWriter, op string, body string, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w,
		`<%sResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/"><%sResult>%s</%sResult><ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata></%sResponse>`,
		op, op, body, op, requestID, op)
}

func rdsErrorXML(w http.ResponseWriter, code, message string, status int, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w,
		`<ErrorResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
		code, message, requestID)
}

func renderRDSInstance(i RDSInstance) string {
	var b strings.Builder
	b.WriteString("<DBInstance>")
	fmt.Fprintf(&b, "<DBInstanceIdentifier>%s</DBInstanceIdentifier>", xmlEscape(i.DBInstanceIdentifier))
	fmt.Fprintf(&b, "<DbiResourceId>%s</DbiResourceId>", xmlEscape(i.DbiResourceId))
	fmt.Fprintf(&b, "<DBInstanceClass>%s</DBInstanceClass>", xmlEscape(i.DBInstanceClass))
	fmt.Fprintf(&b, "<Engine>%s</Engine>", xmlEscape(i.Engine))
	fmt.Fprintf(&b, "<EngineVersion>%s</EngineVersion>", xmlEscape(i.EngineVersion))
	fmt.Fprintf(&b, "<DBInstanceStatus>%s</DBInstanceStatus>", xmlEscape(i.DBInstanceStatus))
	fmt.Fprintf(&b, "<MasterUsername>%s</MasterUsername>", xmlEscape(i.MasterUsername))
	fmt.Fprintf(&b, "<DBName>%s</DBName>", xmlEscape(i.DBName))
	fmt.Fprintf(&b, "<AllocatedStorage>%d</AllocatedStorage>", i.AllocatedStorage)
	fmt.Fprintf(&b, "<AvailabilityZone>%s</AvailabilityZone>", xmlEscape(i.AvailabilityZone))
	fmt.Fprintf(&b, "<InstanceCreateTime>%s</InstanceCreateTime>", xmlEscape(i.InstanceCreateTime))
	fmt.Fprintf(&b, "<DBInstanceArn>%s</DBInstanceArn>", xmlEscape(i.ARN))
	fmt.Fprintf(&b, "<Endpoint><Address>%s</Address><Port>%d</Port></Endpoint>", xmlEscape(i.Endpoint), i.Port)
	b.WriteString("</DBInstance>")
	return b.String()
}

func handleRDSCreate(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBInstanceIdentifier")
	if id == "" {
		rdsErrorXML(w, "MissingParameter", "DBInstanceIdentifier is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := rdsInstances.Get(id); ok {
		rdsErrorXML(w, "DBInstanceAlreadyExists",
			"DB instance already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	engine := r.FormValue("Engine")
	port := rdsDefaultPort(engine)
	if p := atoiOrZero(r.FormValue("Port")); p > 0 {
		port = p
	}
	az := awsRegion() + "a"
	if v := r.FormValue("AvailabilityZone"); v != "" {
		az = v
	}
	engineVersion := r.FormValue("EngineVersion")
	if engineVersion == "" {
		engineVersion = rdsDefaultEngineVersion(engine)
	}
	inst := RDSInstance{
		DBInstanceIdentifier: id,
		DbiResourceId:        rdsResourceID(),
		DBInstanceClass:      r.FormValue("DBInstanceClass"),
		Engine:               engine,
		EngineVersion:        engineVersion,
		DBInstanceStatus:     "available",
		MasterUsername:       r.FormValue("MasterUsername"),
		DBName:               r.FormValue("DBName"),
		AllocatedStorage:     atoiOrZero(r.FormValue("AllocatedStorage")),
		Endpoint:             fmt.Sprintf("%s.%s.rds.amazonaws.com", id, awsRegion()),
		Port:                 port,
		AvailabilityZone:     az,
		InstanceCreateTime:   time.Now().UTC().Format(time.RFC3339),
		ARN:                  rdsInstanceARN(id),
		Tags:                 parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	rdsInstances.Put(id, inst)
	rdsXMLResponse(w, "CreateDBInstance", renderRDSInstance(inst), sim.RequestID(r.Context()))
}

func handleRDSDescribe(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("DBInstanceIdentifier")
	wantedResourceID := rdsFilterValue(r, "dbi-resource-id")
	matched := false
	var b strings.Builder
	b.WriteString("<DBInstances>")
	for _, i := range rdsInstances.List() {
		if wanted != "" && i.DBInstanceIdentifier != wanted && i.DbiResourceId != wanted {
			continue
		}
		if wantedResourceID != "" && i.DbiResourceId != wantedResourceID {
			continue
		}
		matched = true
		b.WriteString(renderRDSInstance(i))
	}
	if wanted != "" && !matched {
		rdsErrorXML(w, "DBInstanceNotFound",
			fmt.Sprintf("DBInstance %q not found", wanted),
			http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</DBInstances>")
	rdsXMLResponse(w, "DescribeDBInstances", b.String(), sim.RequestID(r.Context()))
}

func handleRDSModify(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBInstanceIdentifier")
	if _, ok := rdsInstances.Get(id); !ok {
		rdsErrorXML(w, "DBInstanceNotFound", "DB instance not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsInstances.Update(id, func(i *RDSInstance) {
		if v := r.FormValue("DBInstanceClass"); v != "" {
			i.DBInstanceClass = v
		}
		if v := r.FormValue("AllocatedStorage"); v != "" {
			i.AllocatedStorage = atoiOrZero(v)
		}
		if v := r.FormValue("EngineVersion"); v != "" {
			i.EngineVersion = v
		}
	})
	updated, _ := rdsInstances.Get(id)
	rdsXMLResponse(w, "ModifyDBInstance", renderRDSInstance(updated), sim.RequestID(r.Context()))
}

func handleRDSDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBInstanceIdentifier")
	inst, ok := rdsInstances.Get(id)
	if !ok {
		rdsErrorXML(w, "DBInstanceNotFound", "DB instance not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	inst.DBInstanceStatus = "deleting"
	rdsInstances.Delete(id)
	rdsXMLResponse(w, "DeleteDBInstance", renderRDSInstance(inst), sim.RequestID(r.Context()))
}

func handleRDSAddTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	inst, ok := findRDSByARN(arn)
	if ok {
		rdsInstances.Update(inst.DBInstanceIdentifier, func(i *RDSInstance) {
			i.Tags = mergeTags(i.Tags, parseAWSQueryTagMap(r, "Tags.Tag"))
		})
		rdsXMLResponse(w, "AddTagsToResource", "", sim.RequestID(r.Context()))
		return
	}
	snap, ok := findRDSSnapshotByARN(arn)
	if ok {
		rdsSnapshots.Update(snap.DBSnapshotIdentifier, func(s *RDSSnapshot) {
			s.Tags = mergeTags(s.Tags, parseAWSQueryTagMap(r, "Tags.Tag"))
		})
		rdsXMLResponse(w, "AddTagsToResource", "", sim.RequestID(r.Context()))
		return
	}
	if cl, ok := findRDSClusterByARN(arn); ok {
		rdsClusters.Update(cl.DBClusterIdentifier, func(c *RDSCluster) {
			c.Tags = mergeTags(c.Tags, parseAWSQueryTagMap(r, "Tags.Tag"))
		})
		rdsXMLResponse(w, "AddTagsToResource", "", sim.RequestID(r.Context()))
		return
	}
	if sg, ok := findRDSSubnetGroupByARN(arn); ok {
		rdsSubnetGroups.Update(sg.DBSubnetGroupName, func(g *RDSSubnetGroup) {
			g.Tags = mergeTags(g.Tags, parseAWSQueryTagMap(r, "Tags.Tag"))
		})
		rdsXMLResponse(w, "AddTagsToResource", "", sim.RequestID(r.Context()))
		return
	}
	if pg, ok := findRDSParamGroupByARN(arn); ok {
		rdsParamGroups.Update(pg.DBParameterGroupName, func(g *RDSParamGroup) {
			g.Tags = mergeTags(g.Tags, parseAWSQueryTagMap(r, "Tags.Tag"))
		})
		rdsXMLResponse(w, "AddTagsToResource", "", sim.RequestID(r.Context()))
		return
	}
	rdsErrorXML(w, rdsTagResourceNotFoundCode(arn), "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
}

func handleRDSListTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	inst, ok := findRDSByARN(arn)
	if ok {
		rdsXMLResponse(w, "ListTagsForResource", renderRDSTagList(inst.Tags), sim.RequestID(r.Context()))
		return
	}
	snap, ok := findRDSSnapshotByARN(arn)
	if ok {
		rdsXMLResponse(w, "ListTagsForResource", renderRDSTagList(snap.Tags), sim.RequestID(r.Context()))
		return
	}
	if cl, ok := findRDSClusterByARN(arn); ok {
		rdsXMLResponse(w, "ListTagsForResource", renderRDSTagList(cl.Tags), sim.RequestID(r.Context()))
		return
	}
	if sg, ok := findRDSSubnetGroupByARN(arn); ok {
		rdsXMLResponse(w, "ListTagsForResource", renderRDSTagList(sg.Tags), sim.RequestID(r.Context()))
		return
	}
	if pg, ok := findRDSParamGroupByARN(arn); ok {
		rdsXMLResponse(w, "ListTagsForResource", renderRDSTagList(pg.Tags), sim.RequestID(r.Context()))
		return
	}
	rdsErrorXML(w, rdsTagResourceNotFoundCode(arn), "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
}

func renderRDSTagList(tags map[string]string) string {
	var b strings.Builder
	b.WriteString("<TagList>")
	for k, v := range tags {
		fmt.Fprintf(&b, "<Tag><Key>%s</Key><Value>%s</Value></Tag>", xmlEscape(k), xmlEscape(v))
	}
	b.WriteString("</TagList>")
	return b.String()
}

func handleRDSRemoveTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	inst, ok := findRDSByARN(arn)
	if ok {
		rdsInstances.Update(inst.DBInstanceIdentifier, func(i *RDSInstance) {
			removeAWSQueryTags(i.Tags, r)
		})
		rdsXMLResponse(w, "RemoveTagsFromResource", "", sim.RequestID(r.Context()))
		return
	}
	snap, ok := findRDSSnapshotByARN(arn)
	if ok {
		rdsSnapshots.Update(snap.DBSnapshotIdentifier, func(s *RDSSnapshot) {
			removeAWSQueryTags(s.Tags, r)
		})
		rdsXMLResponse(w, "RemoveTagsFromResource", "", sim.RequestID(r.Context()))
		return
	}
	if cl, ok := findRDSClusterByARN(arn); ok {
		rdsClusters.Update(cl.DBClusterIdentifier, func(c *RDSCluster) {
			removeAWSQueryTags(c.Tags, r)
		})
		rdsXMLResponse(w, "RemoveTagsFromResource", "", sim.RequestID(r.Context()))
		return
	}
	if sg, ok := findRDSSubnetGroupByARN(arn); ok {
		rdsSubnetGroups.Update(sg.DBSubnetGroupName, func(g *RDSSubnetGroup) {
			removeAWSQueryTags(g.Tags, r)
		})
		rdsXMLResponse(w, "RemoveTagsFromResource", "", sim.RequestID(r.Context()))
		return
	}
	if pg, ok := findRDSParamGroupByARN(arn); ok {
		rdsParamGroups.Update(pg.DBParameterGroupName, func(g *RDSParamGroup) {
			removeAWSQueryTags(g.Tags, r)
		})
		rdsXMLResponse(w, "RemoveTagsFromResource", "", sim.RequestID(r.Context()))
		return
	}
	rdsErrorXML(w, rdsTagResourceNotFoundCode(arn), "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
}

func findRDSByARN(arn string) (RDSInstance, bool) {
	for _, i := range rdsInstances.List() {
		if i.ARN == arn {
			return i, true
		}
	}
	// Some callers pass the instance identifier directly.
	if i, ok := rdsInstances.Get(arn); ok {
		return i, true
	}
	return RDSInstance{}, false
}

func findRDSSnapshotByARN(arn string) (RDSSnapshot, bool) {
	for _, s := range rdsSnapshots.List() {
		if s.ARN == arn {
			return s, true
		}
	}
	if s, ok := rdsSnapshots.Get(arn); ok {
		return s, true
	}
	return RDSSnapshot{}, false
}

func rdsTagResourceNotFoundCode(arn string) string {
	switch {
	case strings.Contains(arn, ":snapshot:"):
		return "DBSnapshotNotFound"
	case strings.Contains(arn, ":cluster:"):
		return "DBClusterNotFoundFault"
	case strings.Contains(arn, ":subgrp:"):
		return "DBSubnetGroupNotFoundFault"
	case strings.Contains(arn, ":pg:"):
		return "DBParameterGroupNotFound"
	}
	return "DBInstanceNotFound"
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseAWSQueryTagMap(r *http.Request, prefix string) map[string]string {
	tags := map[string]string{}
	for n := 1; n <= 50; n++ {
		k := r.FormValue(fmt.Sprintf("%s.%d.Key", prefix, n))
		v := r.FormValue(fmt.Sprintf("%s.%d.Value", prefix, n))
		if k == "" {
			break
		}
		tags[k] = v
	}
	return tags
}

func rdsFilterValue(r *http.Request, name string) string {
	for n := 1; n <= 50; n++ {
		prefix := fmt.Sprintf("Filters.Filter.%d", n)
		filterName := r.FormValue(prefix + ".Name")
		if filterName == "" {
			break
		}
		if filterName != name {
			continue
		}
		return r.FormValue(prefix + ".Values.Value.1")
	}
	return ""
}

func mergeTags(dst map[string]string, src map[string]string) map[string]string {
	if dst == nil {
		dst = map[string]string{}
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func removeAWSQueryTags(tags map[string]string, r *http.Request) {
	for n := 1; n <= 50; n++ {
		k := r.FormValue(fmt.Sprintf("TagKeys.member.%d", n))
		if k == "" {
			break
		}
		delete(tags, k)
	}
}

// rdsDefaultEngineVersion returns the engine's current GA major
// version when the request omits EngineVersion. Real RDS resolves
// the default server-side and includes it in the CreateDBInstance
// response; the terraform-provider-aws resource captures the
// resolved version into state, so an empty echo persists as `""`
// and surfaces as state drift on the next plan.
//
// Versions kept current as of mid-2026 GA releases. New majors land
// rarely (1-2× per year); update here when they ship.
// rdsDefaultPort returns the engine's default listener port, matching what RDS
// assigns when no explicit Port is given (and what a snapshot restore inherits
// from the source engine).
func rdsDefaultPort(engine string) int {
	switch engine {
	case "postgres", "aurora-postgresql":
		return 5432
	case "sqlserver-ex", "sqlserver-se", "sqlserver-ee", "sqlserver-web":
		return 1433
	case "oracle-ee", "oracle-se2":
		return 1521
	default: // mysql, aurora, aurora-mysql, mariadb, and unknown engines
		return 3306
	}
}

func rdsDefaultEngineVersion(engine string) string {
	switch engine {
	case "postgres":
		return "17.5"
	case "mysql":
		return "8.0.40"
	case "mariadb":
		return "11.4.4"
	case "aurora-postgresql":
		return "16.6"
	case "aurora-mysql":
		return "8.0.mysql_aurora.3.07.0"
	case "oracle-se2":
		return "19.0.0.0.ru-2024-10.rur-2024-10.r1"
	case "oracle-ee":
		return "19.0.0.0.ru-2024-10.rur-2024-10.r1"
	case "sqlserver-ex", "sqlserver-web", "sqlserver-se", "sqlserver-ee":
		return "16.00.4150.1.v1"
	}
	return ""
}

func rdsSnapshotARN(id string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:snapshot:%s", awsRegion(), awsAccountID(), id)
}

func renderRDSSnapshot(s RDSSnapshot) string {
	var b strings.Builder
	b.WriteString("<DBSnapshot>")
	fmt.Fprintf(&b, "<DBSnapshotIdentifier>%s</DBSnapshotIdentifier>", xmlEscape(s.DBSnapshotIdentifier))
	fmt.Fprintf(&b, "<DBInstanceIdentifier>%s</DBInstanceIdentifier>", xmlEscape(s.DBInstanceIdentifier))
	fmt.Fprintf(&b, "<DbiResourceId>%s</DbiResourceId>", xmlEscape(s.DbiResourceId))
	fmt.Fprintf(&b, "<Engine>%s</Engine>", xmlEscape(s.Engine))
	fmt.Fprintf(&b, "<EngineVersion>%s</EngineVersion>", xmlEscape(s.EngineVersion))
	fmt.Fprintf(&b, "<Status>%s</Status>", xmlEscape(s.Status))
	fmt.Fprintf(&b, "<AllocatedStorage>%d</AllocatedStorage>", s.AllocatedStorage)
	fmt.Fprintf(&b, "<MasterUsername>%s</MasterUsername>", xmlEscape(s.MasterUsername))
	fmt.Fprintf(&b, "<SnapshotCreateTime>%s</SnapshotCreateTime>", xmlEscape(s.SnapshotCreateTime))
	fmt.Fprintf(&b, "<SnapshotType>%s</SnapshotType>", xmlEscape(s.SnapshotType))
	fmt.Fprintf(&b, "<DBSnapshotArn>%s</DBSnapshotArn>", xmlEscape(s.ARN))
	b.WriteString("</DBSnapshot>")
	return b.String()
}

func handleRDSCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("DBSnapshotIdentifier")
	instID := r.FormValue("DBInstanceIdentifier")
	if snapID == "" {
		rdsErrorXML(w, "MissingParameter",
			"DBSnapshotIdentifier is required",
			http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	inst, ok := rdsInstances.Get(instID)
	if !ok {
		rdsErrorXML(w, "DBInstanceNotFound",
			fmt.Sprintf("DBInstance %q not found", instID),
			http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	if _, exists := rdsSnapshots.Get(snapID); exists {
		rdsErrorXML(w, "DBSnapshotAlreadyExists",
			fmt.Sprintf("DBSnapshot %q already exists", snapID),
			http.StatusConflict, sim.RequestID(r.Context()))
		return
	}
	snap := RDSSnapshot{
		DBSnapshotIdentifier: snapID,
		DBInstanceIdentifier: instID,
		DbiResourceId:        inst.DbiResourceId,
		Engine:               inst.Engine,
		EngineVersion:        inst.EngineVersion,
		// Inline-settle: real RDS goes through "creating" briefly; sim
		// emits the steady-state "available" because there's no
		// async work to gate on. State machine is documented in the
		// type's docstring + the aws-rds.md surface table.
		Status:             "available",
		AllocatedStorage:   inst.AllocatedStorage,
		MasterUsername:     inst.MasterUsername,
		SnapshotCreateTime: time.Now().UTC().Format(time.RFC3339),
		SnapshotType:       "manual",
		ARN:                rdsSnapshotARN(snapID),
		Tags:               parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	rdsSnapshots.Put(snapID, snap)
	rdsXMLResponse(w, "CreateDBSnapshot", renderRDSSnapshot(snap), sim.RequestID(r.Context()))
}

func handleRDSDescribeSnapshots(w http.ResponseWriter, r *http.Request) {
	filterID := r.FormValue("DBSnapshotIdentifier")
	filterInst := r.FormValue("DBInstanceIdentifier")
	matched := false
	var b strings.Builder
	b.WriteString("<DBSnapshots>")
	for _, s := range rdsSnapshots.List() {
		if filterID != "" && s.DBSnapshotIdentifier != filterID {
			continue
		}
		if filterInst != "" && s.DBInstanceIdentifier != filterInst {
			continue
		}
		matched = true
		b.WriteString(renderRDSSnapshot(s))
	}
	if filterID != "" && !matched {
		rdsErrorXML(w, "DBSnapshotNotFound",
			fmt.Sprintf("DBSnapshot %q not found", filterID),
			http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</DBSnapshots>")
	rdsXMLResponse(w, "DescribeDBSnapshots", b.String(), sim.RequestID(r.Context()))
}

func handleRDSDescribeSnapshotAttributes(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("DBSnapshotIdentifier")
	if snapID == "" {
		rdsErrorXML(w, "MissingParameter",
			"DBSnapshotIdentifier is required",
			http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	snap, ok := rdsSnapshots.Get(snapID)
	if !ok {
		snap, ok = findRDSSnapshotByARN(snapID)
		if !ok {
			rdsErrorXML(w, "DBSnapshotNotFound",
				fmt.Sprintf("DBSnapshot %q not found", snapID),
				http.StatusNotFound, sim.RequestID(r.Context()))
			return
		}
	}
	body := fmt.Sprintf(
		"<DBSnapshotAttributesResult>"+
			"<DBSnapshotIdentifier>%s</DBSnapshotIdentifier>"+
			"<DBSnapshotAttributes>"+
			"<DBSnapshotAttribute><AttributeName>restore</AttributeName><AttributeValues></AttributeValues></DBSnapshotAttribute>"+
			"</DBSnapshotAttributes>"+
			"</DBSnapshotAttributesResult>",
		xmlEscape(snap.DBSnapshotIdentifier),
	)
	rdsXMLResponse(w, "DescribeDBSnapshotAttributes", body, sim.RequestID(r.Context()))
}

func handleRDSDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("DBSnapshotIdentifier")
	snap, ok := rdsSnapshots.Get(snapID)
	if !ok {
		snap, ok = findRDSSnapshotByARN(snapID)
		if !ok {
			rdsErrorXML(w, "DBSnapshotNotFound",
				fmt.Sprintf("DBSnapshot %q not found", snapID),
				http.StatusNotFound, sim.RequestID(r.Context()))
			return
		}
	}
	rdsSnapshots.Delete(snap.DBSnapshotIdentifier)
	// Real RDS returns the snapshot with Status="deleted" in the
	// response (it's the final state machine transition before
	// removal). Match that.
	snap.Status = "deleted"
	rdsXMLResponse(w, "DeleteDBSnapshot", renderRDSSnapshot(snap), sim.RequestID(r.Context()))
}

func handleRDSRestoreFromSnapshot(w http.ResponseWriter, r *http.Request) {
	newInstID := r.FormValue("DBInstanceIdentifier")
	snapID := r.FormValue("DBSnapshotIdentifier")
	if newInstID == "" || snapID == "" {
		rdsErrorXML(w, "MissingParameter",
			"DBInstanceIdentifier and DBSnapshotIdentifier are required",
			http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	snap, ok := rdsSnapshots.Get(snapID)
	if !ok {
		snap, ok = findRDSSnapshotByARN(snapID)
		if !ok {
			rdsErrorXML(w, "DBSnapshotNotFound",
				fmt.Sprintf("DBSnapshot %q not found", snapID),
				http.StatusNotFound, sim.RequestID(r.Context()))
			return
		}
	}
	if _, exists := rdsInstances.Get(newInstID); exists {
		rdsErrorXML(w, "DBInstanceAlreadyExists",
			fmt.Sprintf("DBInstance %q already exists", newInstID),
			http.StatusConflict, sim.RequestID(r.Context()))
		return
	}
	inst := RDSInstance{
		DBInstanceIdentifier: newInstID,
		DbiResourceId:        rdsResourceID(),
		DBInstanceClass:      r.FormValue("DBInstanceClass"),
		Engine:               snap.Engine,
		EngineVersion:        snap.EngineVersion,
		DBInstanceStatus:     "available",
		MasterUsername:       snap.MasterUsername,
		AllocatedStorage:     snap.AllocatedStorage,
		Endpoint:             fmt.Sprintf("%s.%s.rds.amazonaws.com", newInstID, awsRegion()),
		Port:                 rdsDefaultPort(snap.Engine),
		AvailabilityZone:     awsRegion() + "a",
		InstanceCreateTime:   time.Now().UTC().Format(time.RFC3339),
		ARN:                  rdsInstanceARN(newInstID),
		Tags:                 parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	rdsInstances.Put(newInstID, inst)
	rdsXMLResponse(w, "RestoreDBInstanceFromDBSnapshot", renderRDSInstance(inst), sim.RequestID(r.Context()))
}

func handleRDSReboot(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBInstanceIdentifier")
	inst, ok := rdsInstances.Get(id)
	if !ok {
		rdsErrorXML(w, "DBInstanceNotFound", "DB instance not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	// Real RDS transitions creating→rebooting→available; with no
	// engine to restart, the sim returns the steady-state instance
	// (Status stays "available"). The response carries the full
	// DBInstance, which is what the waiter and SDK consumers read.
	rdsXMLResponse(w, "RebootDBInstance", renderRDSInstance(inst), sim.RequestID(r.Context()))
}

// ----- DB clusters -----

func rdsClusterARN(id string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:cluster:%s", awsRegion(), awsAccountID(), id)
}

func rdsClusterResourceID() string {
	return "cluster-" + strings.ToUpper(strings.ReplaceAll(generateUUID(), "-", ""))[:26]
}

func findRDSClusterByARN(arn string) (RDSCluster, bool) {
	for _, c := range rdsClusters.List() {
		if c.ARN == arn {
			return c, true
		}
	}
	if c, ok := rdsClusters.Get(arn); ok {
		return c, true
	}
	return RDSCluster{}, false
}

func renderRDSCluster(c RDSCluster) string {
	var b strings.Builder
	b.WriteString("<DBCluster>")
	fmt.Fprintf(&b, "<DBClusterIdentifier>%s</DBClusterIdentifier>", xmlEscape(c.DBClusterIdentifier))
	fmt.Fprintf(&b, "<DbClusterResourceId>%s</DbClusterResourceId>", xmlEscape(c.DbClusterResourceId))
	fmt.Fprintf(&b, "<DBClusterArn>%s</DBClusterArn>", xmlEscape(c.ARN))
	fmt.Fprintf(&b, "<Engine>%s</Engine>", xmlEscape(c.Engine))
	fmt.Fprintf(&b, "<EngineVersion>%s</EngineVersion>", xmlEscape(c.EngineVersion))
	fmt.Fprintf(&b, "<EngineMode>%s</EngineMode>", xmlEscape(c.EngineMode))
	fmt.Fprintf(&b, "<Status>%s</Status>", xmlEscape(c.Status))
	fmt.Fprintf(&b, "<DatabaseName>%s</DatabaseName>", xmlEscape(c.DatabaseName))
	fmt.Fprintf(&b, "<MasterUsername>%s</MasterUsername>", xmlEscape(c.MasterUsername))
	fmt.Fprintf(&b, "<Port>%d</Port>", c.Port)
	fmt.Fprintf(&b, "<Endpoint>%s</Endpoint>", xmlEscape(c.Endpoint))
	fmt.Fprintf(&b, "<ReaderEndpoint>%s</ReaderEndpoint>", xmlEscape(c.ReaderEndpoint))
	fmt.Fprintf(&b, "<DBClusterParameterGroup>%s</DBClusterParameterGroup>", xmlEscape(c.DBClusterParameterGroup))
	fmt.Fprintf(&b, "<DBSubnetGroup>%s</DBSubnetGroup>", xmlEscape(c.DBSubnetGroup))
	fmt.Fprintf(&b, "<AllocatedStorage>%d</AllocatedStorage>", c.AllocatedStorage)
	fmt.Fprintf(&b, "<BackupRetentionPeriod>%d</BackupRetentionPeriod>", c.BackupRetentionPeriod)
	fmt.Fprintf(&b, "<StorageEncrypted>%t</StorageEncrypted>", c.StorageEncrypted)
	fmt.Fprintf(&b, "<DeletionProtection>%t</DeletionProtection>", c.DeletionProtection)
	fmt.Fprintf(&b, "<ClusterCreateTime>%s</ClusterCreateTime>", xmlEscape(c.ClusterCreateTime))
	fmt.Fprintf(&b, "<PreferredBackupWindow>%s</PreferredBackupWindow>", xmlEscape(c.PreferredBackupWindow))
	fmt.Fprintf(&b, "<PreferredMaintenanceWindow>%s</PreferredMaintenanceWindow>", xmlEscape(c.PreferredMaintenanceWindow))
	b.WriteString("<AvailabilityZones>")
	for _, az := range c.AvailabilityZones {
		fmt.Fprintf(&b, "<AvailabilityZone>%s</AvailabilityZone>", xmlEscape(az))
	}
	b.WriteString("</AvailabilityZones>")
	b.WriteString("</DBCluster>")
	return b.String()
}

func handleRDSCreateCluster(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBClusterIdentifier")
	if id == "" {
		rdsErrorXML(w, "MissingParameter", "DBClusterIdentifier is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := rdsClusters.Get(id); ok {
		rdsErrorXML(w, "DBClusterAlreadyExistsFault", "DB cluster already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	engine := r.FormValue("Engine")
	port := rdsDefaultPort(engine)
	if p := atoiOrZero(r.FormValue("Port")); p > 0 {
		port = p
	}
	engineVersion := r.FormValue("EngineVersion")
	if engineVersion == "" {
		engineVersion = rdsDefaultEngineVersion(engine)
	}
	engineMode := r.FormValue("EngineMode")
	if engineMode == "" {
		engineMode = "provisioned"
	}
	paramGroup := r.FormValue("DBClusterParameterGroupName")
	if paramGroup == "" {
		paramGroup = "default." + engine
	}
	backupRetention := 1
	if v := r.FormValue("BackupRetentionPeriod"); v != "" {
		backupRetention = atoiOrZero(v)
	}
	cl := RDSCluster{
		DBClusterIdentifier:        id,
		DbClusterResourceId:        rdsClusterResourceID(),
		Engine:                     engine,
		EngineVersion:              engineVersion,
		EngineMode:                 engineMode,
		Status:                     "available",
		DatabaseName:               r.FormValue("DatabaseName"),
		MasterUsername:             r.FormValue("MasterUsername"),
		Port:                       port,
		Endpoint:                   fmt.Sprintf("%s.cluster-%s.%s.rds.amazonaws.com", id, "sim", awsRegion()),
		ReaderEndpoint:             fmt.Sprintf("%s.cluster-ro-%s.%s.rds.amazonaws.com", id, "sim", awsRegion()),
		DBClusterParameterGroup:    paramGroup,
		DBSubnetGroup:              r.FormValue("DBSubnetGroupName"),
		AllocatedStorage:           atoiOrZero(r.FormValue("AllocatedStorage")),
		BackupRetentionPeriod:      backupRetention,
		StorageEncrypted:           r.FormValue("StorageEncrypted") == "true",
		DeletionProtection:         r.FormValue("DeletionProtection") == "true",
		ClusterCreateTime:          time.Now().UTC().Format(time.RFC3339),
		AvailabilityZones:          []string{awsRegion() + "a", awsRegion() + "b", awsRegion() + "c"},
		PreferredBackupWindow:      "07:00-09:00",
		PreferredMaintenanceWindow: "mon:00:00-mon:03:00",
		ARN:                        rdsClusterARN(id),
		Tags:                       parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	rdsClusters.Put(id, cl)
	rdsXMLResponse(w, "CreateDBCluster", renderRDSCluster(cl), sim.RequestID(r.Context()))
}

func handleRDSDescribeClusters(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("DBClusterIdentifier")
	matched := false
	var b strings.Builder
	b.WriteString("<DBClusters>")
	for _, c := range rdsClusters.List() {
		if wanted != "" && c.DBClusterIdentifier != wanted && c.ARN != wanted {
			continue
		}
		matched = true
		b.WriteString(renderRDSCluster(c))
	}
	if wanted != "" && !matched {
		rdsErrorXML(w, "DBClusterNotFoundFault",
			fmt.Sprintf("DBCluster %q not found", wanted),
			http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</DBClusters>")
	rdsXMLResponse(w, "DescribeDBClusters", b.String(), sim.RequestID(r.Context()))
}

func handleRDSModifyCluster(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBClusterIdentifier")
	if _, ok := rdsClusters.Get(id); !ok {
		rdsErrorXML(w, "DBClusterNotFoundFault", "DB cluster not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsClusters.Update(id, func(c *RDSCluster) {
		if v := r.FormValue("EngineVersion"); v != "" {
			c.EngineVersion = v
		}
		if v := r.FormValue("BackupRetentionPeriod"); v != "" {
			c.BackupRetentionPeriod = atoiOrZero(v)
		}
		if v := r.FormValue("PreferredBackupWindow"); v != "" {
			c.PreferredBackupWindow = v
		}
		if v := r.FormValue("PreferredMaintenanceWindow"); v != "" {
			c.PreferredMaintenanceWindow = v
		}
		if v := r.FormValue("DeletionProtection"); v != "" {
			c.DeletionProtection = v == "true"
		}
		if v := r.FormValue("Port"); v != "" {
			if p := atoiOrZero(v); p > 0 {
				c.Port = p
			}
		}
	})
	updated, _ := rdsClusters.Get(id)
	rdsXMLResponse(w, "ModifyDBCluster", renderRDSCluster(updated), sim.RequestID(r.Context()))
}

func handleRDSDeleteCluster(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("DBClusterIdentifier")
	cl, ok := rdsClusters.Get(id)
	if !ok {
		rdsErrorXML(w, "DBClusterNotFoundFault", "DB cluster not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsClusters.Delete(id)
	cl.Status = "deleting"
	rdsXMLResponse(w, "DeleteDBCluster", renderRDSCluster(cl), sim.RequestID(r.Context()))
}

// ----- DB subnet groups -----

func rdsSubnetGroupARN(name string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:subgrp:%s", awsRegion(), awsAccountID(), name)
}

func findRDSSubnetGroupByARN(arn string) (RDSSubnetGroup, bool) {
	for _, g := range rdsSubnetGroups.List() {
		if g.ARN == arn {
			return g, true
		}
	}
	if g, ok := rdsSubnetGroups.Get(arn); ok {
		return g, true
	}
	return RDSSubnetGroup{}, false
}

func parseRDSSubnetIDs(r *http.Request) []string {
	var ids []string
	for n := 1; n <= 50; n++ {
		v := r.FormValue(fmt.Sprintf("SubnetIds.SubnetIdentifier.%d", n))
		if v == "" {
			break
		}
		ids = append(ids, v)
	}
	return ids
}

func renderRDSSubnetGroup(g RDSSubnetGroup) string {
	var b strings.Builder
	b.WriteString("<DBSubnetGroup>")
	fmt.Fprintf(&b, "<DBSubnetGroupName>%s</DBSubnetGroupName>", xmlEscape(g.DBSubnetGroupName))
	fmt.Fprintf(&b, "<DBSubnetGroupDescription>%s</DBSubnetGroupDescription>", xmlEscape(g.DBSubnetGroupDescription))
	fmt.Fprintf(&b, "<VpcId>%s</VpcId>", xmlEscape(g.VpcId))
	fmt.Fprintf(&b, "<SubnetGroupStatus>%s</SubnetGroupStatus>", xmlEscape(g.SubnetGroupStatus))
	fmt.Fprintf(&b, "<DBSubnetGroupArn>%s</DBSubnetGroupArn>", xmlEscape(g.ARN))
	b.WriteString("<Subnets>")
	for i, sid := range g.SubnetIds {
		az := awsRegion() + string(rune('a'+i%3))
		b.WriteString("<Subnet>")
		fmt.Fprintf(&b, "<SubnetIdentifier>%s</SubnetIdentifier>", xmlEscape(sid))
		fmt.Fprintf(&b, "<SubnetAvailabilityZone><Name>%s</Name></SubnetAvailabilityZone>", xmlEscape(az))
		b.WriteString("<SubnetStatus>Active</SubnetStatus>")
		b.WriteString("</Subnet>")
	}
	b.WriteString("</Subnets>")
	b.WriteString("</DBSubnetGroup>")
	return b.String()
}

func handleRDSCreateSubnetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("DBSubnetGroupName")
	if name == "" {
		rdsErrorXML(w, "MissingParameter", "DBSubnetGroupName is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := rdsSubnetGroups.Get(name); ok {
		rdsErrorXML(w, "DBSubnetGroupAlreadyExists", "DB subnet group already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	vpcID := r.FormValue("VpcId")
	if vpcID == "" {
		vpcID = "vpc-" + strings.ReplaceAll(generateUUID(), "-", "")[:17]
	}
	g := RDSSubnetGroup{
		DBSubnetGroupName:        name,
		DBSubnetGroupDescription: r.FormValue("DBSubnetGroupDescription"),
		VpcId:                    vpcID,
		SubnetGroupStatus:        "Complete",
		SubnetIds:                parseRDSSubnetIDs(r),
		ARN:                      rdsSubnetGroupARN(name),
		Tags:                     parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	rdsSubnetGroups.Put(name, g)
	rdsXMLResponse(w, "CreateDBSubnetGroup", renderRDSSubnetGroup(g), sim.RequestID(r.Context()))
}

func handleRDSDescribeSubnetGroups(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("DBSubnetGroupName")
	matched := false
	var b strings.Builder
	b.WriteString("<DBSubnetGroups>")
	for _, g := range rdsSubnetGroups.List() {
		if wanted != "" && g.DBSubnetGroupName != wanted {
			continue
		}
		matched = true
		b.WriteString(renderRDSSubnetGroup(g))
	}
	if wanted != "" && !matched {
		rdsErrorXML(w, "DBSubnetGroupNotFoundFault",
			fmt.Sprintf("DBSubnetGroup %q not found", wanted),
			http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</DBSubnetGroups>")
	rdsXMLResponse(w, "DescribeDBSubnetGroups", b.String(), sim.RequestID(r.Context()))
}

func handleRDSModifySubnetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("DBSubnetGroupName")
	if _, ok := rdsSubnetGroups.Get(name); !ok {
		rdsErrorXML(w, "DBSubnetGroupNotFoundFault", "DB subnet group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsSubnetGroups.Update(name, func(g *RDSSubnetGroup) {
		if v := r.FormValue("DBSubnetGroupDescription"); v != "" {
			g.DBSubnetGroupDescription = v
		}
		if ids := parseRDSSubnetIDs(r); len(ids) > 0 {
			g.SubnetIds = ids
		}
	})
	updated, _ := rdsSubnetGroups.Get(name)
	rdsXMLResponse(w, "ModifyDBSubnetGroup", renderRDSSubnetGroup(updated), sim.RequestID(r.Context()))
}

func handleRDSDeleteSubnetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("DBSubnetGroupName")
	if _, ok := rdsSubnetGroups.Get(name); !ok {
		rdsErrorXML(w, "DBSubnetGroupNotFoundFault", "DB subnet group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsSubnetGroups.Delete(name)
	// DeleteDBSubnetGroup has an empty result body on real RDS.
	rdsXMLResponse(w, "DeleteDBSubnetGroup", "", sim.RequestID(r.Context()))
}

// ----- DB parameter groups -----

func rdsParamGroupARN(name string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:pg:%s", awsRegion(), awsAccountID(), name)
}

func findRDSParamGroupByARN(arn string) (RDSParamGroup, bool) {
	for _, g := range rdsParamGroups.List() {
		if g.ARN == arn {
			return g, true
		}
	}
	if g, ok := rdsParamGroups.Get(arn); ok {
		return g, true
	}
	return RDSParamGroup{}, false
}

func renderRDSParamGroup(g RDSParamGroup) string {
	var b strings.Builder
	b.WriteString("<DBParameterGroup>")
	fmt.Fprintf(&b, "<DBParameterGroupName>%s</DBParameterGroupName>", xmlEscape(g.DBParameterGroupName))
	fmt.Fprintf(&b, "<DBParameterGroupFamily>%s</DBParameterGroupFamily>", xmlEscape(g.DBParameterGroupFamily))
	fmt.Fprintf(&b, "<Description>%s</Description>", xmlEscape(g.Description))
	fmt.Fprintf(&b, "<DBParameterGroupArn>%s</DBParameterGroupArn>", xmlEscape(g.ARN))
	b.WriteString("</DBParameterGroup>")
	return b.String()
}

func handleRDSCreateParamGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("DBParameterGroupName")
	family := r.FormValue("DBParameterGroupFamily")
	if name == "" || family == "" {
		rdsErrorXML(w, "MissingParameter", "DBParameterGroupName and DBParameterGroupFamily are required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	if _, ok := rdsParamGroups.Get(name); ok {
		rdsErrorXML(w, "DBParameterGroupAlreadyExists", "DB parameter group already exists", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	g := RDSParamGroup{
		DBParameterGroupName:   name,
		DBParameterGroupFamily: family,
		Description:            r.FormValue("Description"),
		ARN:                    rdsParamGroupARN(name),
		Tags:                   parseAWSQueryTagMap(r, "Tags.Tag"),
	}
	rdsParamGroups.Put(name, g)
	rdsXMLResponse(w, "CreateDBParameterGroup", renderRDSParamGroup(g), sim.RequestID(r.Context()))
}

func handleRDSDescribeParamGroups(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("DBParameterGroupName")
	matched := false
	var b strings.Builder
	b.WriteString("<DBParameterGroups>")
	for _, g := range rdsParamGroups.List() {
		if wanted != "" && g.DBParameterGroupName != wanted {
			continue
		}
		matched = true
		b.WriteString(renderRDSParamGroup(g))
	}
	if wanted != "" && !matched {
		rdsErrorXML(w, "DBParameterGroupNotFound",
			fmt.Sprintf("DBParameterGroup %q not found", wanted),
			http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	b.WriteString("</DBParameterGroups>")
	rdsXMLResponse(w, "DescribeDBParameterGroups", b.String(), sim.RequestID(r.Context()))
}

func handleRDSDeleteParamGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("DBParameterGroupName")
	if _, ok := rdsParamGroups.Get(name); !ok {
		rdsErrorXML(w, "DBParameterGroupNotFound", "DB parameter group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsParamGroups.Delete(name)
	// DeleteDBParameterGroup has an empty result body on real RDS.
	rdsXMLResponse(w, "DeleteDBParameterGroup", "", sim.RequestID(r.Context()))
}
