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
// RemoveTagsFromResource. Database engine itself is not simulated;
// the sim returns Status=available immediately on Create.

type RDSInstance struct {
	DBInstanceIdentifier string
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

var rdsInstances sim.Store[RDSInstance]

func registerRDS(r *sim.AWSQueryRouter, srv *sim.Server) {
	rdsInstances = sim.MakeStore[RDSInstance](srv.DB(), "rds_instances")
	r.Register("CreateDBInstance", handleRDSCreate)
	r.Register("DescribeDBInstances", handleRDSDescribe)
	r.Register("ModifyDBInstance", handleRDSModify)
	r.Register("DeleteDBInstance", handleRDSDelete)
	r.Register("AddTagsToResource", handleRDSAddTags)
	r.Register("ListTagsForResource", handleRDSListTags)
	r.Register("RemoveTagsFromResource", handleRDSRemoveTags)
}

func rdsInstanceARN(id string) string {
	return fmt.Sprintf("arn:aws:rds:%s:%s:db:%s", awsRegion(), awsAccountID(), id)
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
	port := 3306
	switch r.FormValue("Engine") {
	case "postgres", "aurora-postgresql":
		port = 5432
	case "mysql", "aurora", "aurora-mysql", "mariadb":
		port = 3306
	case "sqlserver-ex", "sqlserver-se", "sqlserver-ee", "sqlserver-web":
		port = 1433
	case "oracle-ee", "oracle-se2":
		port = 1521
	}
	inst := RDSInstance{
		DBInstanceIdentifier: id,
		DBInstanceClass:      r.FormValue("DBInstanceClass"),
		Engine:               r.FormValue("Engine"),
		EngineVersion:        r.FormValue("EngineVersion"),
		DBInstanceStatus:     "available",
		MasterUsername:       r.FormValue("MasterUsername"),
		DBName:               r.FormValue("DBName"),
		AllocatedStorage:     atoiOrZero(r.FormValue("AllocatedStorage")),
		Endpoint:             fmt.Sprintf("%s.%s.rds.amazonaws.com", id, awsRegion()),
		Port:                 port,
		AvailabilityZone:     awsRegion() + "a",
		InstanceCreateTime:   time.Now().UTC().Format(time.RFC3339),
		ARN:                  rdsInstanceARN(id),
		Tags:                 map[string]string{},
	}
	rdsInstances.Put(id, inst)
	rdsXMLResponse(w, "CreateDBInstance", renderRDSInstance(inst), sim.RequestID(r.Context()))
}

func handleRDSDescribe(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("DBInstanceIdentifier")
	var b strings.Builder
	b.WriteString("<DBInstances>")
	for _, i := range rdsInstances.List() {
		if wanted != "" && i.DBInstanceIdentifier != wanted {
			continue
		}
		b.WriteString(renderRDSInstance(i))
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
	if !ok {
		rdsErrorXML(w, "DBInstanceNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsInstances.Update(inst.DBInstanceIdentifier, func(i *RDSInstance) {
		if i.Tags == nil {
			i.Tags = map[string]string{}
		}
		for n := 1; n <= 50; n++ {
			k := r.FormValue(fmt.Sprintf("Tags.Tag.%d.Key", n))
			v := r.FormValue(fmt.Sprintf("Tags.Tag.%d.Value", n))
			if k == "" {
				break
			}
			i.Tags[k] = v
		}
	})
	rdsXMLResponse(w, "AddTagsToResource", "", sim.RequestID(r.Context()))
}

func handleRDSListTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	inst, ok := findRDSByARN(arn)
	if !ok {
		rdsErrorXML(w, "DBInstanceNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	var b strings.Builder
	b.WriteString("<TagList>")
	for k, v := range inst.Tags {
		fmt.Fprintf(&b, "<Tag><Key>%s</Key><Value>%s</Value></Tag>", xmlEscape(k), xmlEscape(v))
	}
	b.WriteString("</TagList>")
	rdsXMLResponse(w, "ListTagsForResource", b.String(), sim.RequestID(r.Context()))
}

func handleRDSRemoveTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	inst, ok := findRDSByARN(arn)
	if !ok {
		rdsErrorXML(w, "DBInstanceNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	rdsInstances.Update(inst.DBInstanceIdentifier, func(i *RDSInstance) {
		for n := 1; n <= 50; n++ {
			k := r.FormValue(fmt.Sprintf("TagKeys.member.%d", n))
			if k == "" {
				break
			}
			delete(i.Tags, k)
		}
	})
	rdsXMLResponse(w, "RemoveTagsFromResource", "", sim.RequestID(r.Context()))
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
