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

var (
	rdsInstances sim.Store[RDSInstance]
	rdsSnapshots sim.Store[RDSSnapshot]
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
	port := rdsDefaultPort(r.FormValue("Engine"))
	engine := r.FormValue("Engine")
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
		AvailabilityZone:     awsRegion() + "a",
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
	if strings.Contains(arn, ":snapshot:") {
		return "DBSnapshotNotFound"
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
