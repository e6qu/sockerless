package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Route 53 — REST + XML, path version /2013-04-01/, namespace
// https://route53.amazonaws.com/doc/2013-04-01/. Same protocol family
// as CloudFront and S3 (sim already uses encoding/xml for both).
// CloudFront's reserved hosted-zone ID is Z2FDTNDATAQYW2 — when
// aws_route53_record's alias { name = aws_cloudfront_distribution.x.domain_name,
// zone_id = aws_cloudfront_distribution.x.hosted_zone_id } resolves at
// apply time, that's the Z2FDTNDATAQYW2 the SDK sends.

const (
	r53APIVersion = "2013-04-01"
	r53Namespace  = "https://route53.amazonaws.com/doc/2013-04-01/"
)

// ---------- HostedZone ----------

type R53HostedZone struct {
	XMLName                xml.Name             `xml:"HostedZone"`
	Xmlns                  string               `xml:"xmlns,attr,omitempty"`
	Id                     string               `xml:"Id"`
	Name                   string               `xml:"Name"`
	CallerReference        string               `xml:"CallerReference"`
	Config                 *R53HostedZoneConfig `xml:"Config,omitempty"`
	ResourceRecordSetCount int                  `xml:"ResourceRecordSetCount"`
	LinkedService          *R53LinkedService    `xml:"LinkedService,omitempty"`
}

type R53HostedZoneConfig struct {
	Comment     string `xml:"Comment,omitempty"`
	PrivateZone bool   `xml:"PrivateZone"`
}

type R53LinkedService struct {
	ServicePrincipal string `xml:"ServicePrincipal"`
	Description      string `xml:"Description"`
}

type R53HostedZoneSummary R53HostedZone

type R53HostedZoneList struct {
	XMLName     xml.Name        `xml:"ListHostedZonesResponse"`
	Xmlns       string          `xml:"xmlns,attr,omitempty"`
	HostedZones []R53HostedZone `xml:"HostedZones>HostedZone"`
	Marker      string          `xml:"Marker"`
	IsTruncated bool            `xml:"IsTruncated"`
	MaxItems    string          `xml:"MaxItems"`
	NextMarker  string          `xml:"NextMarker,omitempty"`
}

type R53CreateHostedZoneRequest struct {
	XMLName          xml.Name             `xml:"CreateHostedZoneRequest"`
	Name             string               `xml:"Name"`
	CallerReference  string               `xml:"CallerReference"`
	HostedZoneConfig *R53HostedZoneConfig `xml:"HostedZoneConfig,omitempty"`
	VPC              *R53VPC              `xml:"VPC,omitempty"`
	DelegationSetId  string               `xml:"DelegationSetId,omitempty"`
}

type R53VPC struct {
	VPCRegion string `xml:"VPCRegion"`
	VPCId     string `xml:"VPCId"`
}

type R53CreateHostedZoneResponse struct {
	XMLName       xml.Name         `xml:"CreateHostedZoneResponse"`
	Xmlns         string           `xml:"xmlns,attr,omitempty"`
	HostedZone    R53HostedZone    `xml:"HostedZone"`
	ChangeInfo    R53ChangeInfo    `xml:"ChangeInfo"`
	DelegationSet R53DelegationSet `xml:"DelegationSet"`
}

type R53DelegationSet struct {
	NameServers []string `xml:"NameServers>NameServer"`
}

type R53GetHostedZoneResponse struct {
	XMLName       xml.Name         `xml:"GetHostedZoneResponse"`
	Xmlns         string           `xml:"xmlns,attr,omitempty"`
	HostedZone    R53HostedZone    `xml:"HostedZone"`
	DelegationSet R53DelegationSet `xml:"DelegationSet"`
}

// ---------- ResourceRecordSet ----------

type R53ResourceRecordSet struct {
	Name                    string              `xml:"Name"`
	Type                    string              `xml:"Type"`
	SetIdentifier           string              `xml:"SetIdentifier,omitempty"`
	Weight                  *int64              `xml:"Weight,omitempty"`
	Region                  string              `xml:"Region,omitempty"`
	GeoLocation             *R53GeoLocation     `xml:"GeoLocation,omitempty"`
	Failover                string              `xml:"Failover,omitempty"`
	MultiValueAnswer        *bool               `xml:"MultiValueAnswer,omitempty"`
	TTL                     *int64              `xml:"TTL,omitempty"`
	ResourceRecords         *R53ResourceRecords `xml:"ResourceRecords,omitempty"`
	AliasTarget             *R53AliasTarget     `xml:"AliasTarget,omitempty"`
	HealthCheckId           string              `xml:"HealthCheckId,omitempty"`
	TrafficPolicyInstanceId string              `xml:"TrafficPolicyInstanceId,omitempty"`
	CidrRoutingConfig       *R53CidrRouting     `xml:"CidrRoutingConfig,omitempty"`
}

type R53ResourceRecords struct {
	Items []R53ResourceRecord `xml:"ResourceRecord"`
}

type R53ResourceRecord struct {
	Value string `xml:"Value"`
}

type R53AliasTarget struct {
	HostedZoneId         string `xml:"HostedZoneId"`
	DNSName              string `xml:"DNSName"`
	EvaluateTargetHealth bool   `xml:"EvaluateTargetHealth"`
}

type R53GeoLocation struct {
	ContinentCode   string `xml:"ContinentCode,omitempty"`
	CountryCode     string `xml:"CountryCode,omitempty"`
	SubdivisionCode string `xml:"SubdivisionCode,omitempty"`
}

type R53CidrRouting struct {
	CollectionId string `xml:"CollectionId"`
	LocationName string `xml:"LocationName"`
}

type R53Change struct {
	Action            string               `xml:"Action"`
	ResourceRecordSet R53ResourceRecordSet `xml:"ResourceRecordSet"`
}

type R53ChangeBatch struct {
	Comment string      `xml:"Comment,omitempty"`
	Changes []R53Change `xml:"Changes>Change"`
}

type R53ChangeRRSetRequest struct {
	XMLName     xml.Name       `xml:"ChangeResourceRecordSetsRequest"`
	ChangeBatch R53ChangeBatch `xml:"ChangeBatch"`
}

type R53ChangeInfo struct {
	Id          string `xml:"Id"`
	Status      string `xml:"Status"`
	SubmittedAt string `xml:"SubmittedAt"`
	Comment     string `xml:"Comment,omitempty"`
}

type R53ChangeResourceRecordSetsResponse struct {
	XMLName    xml.Name      `xml:"ChangeResourceRecordSetsResponse"`
	Xmlns      string        `xml:"xmlns,attr,omitempty"`
	ChangeInfo R53ChangeInfo `xml:"ChangeInfo"`
}

type R53ListResourceRecordSetsResponse struct {
	XMLName            xml.Name               `xml:"ListResourceRecordSetsResponse"`
	Xmlns              string                 `xml:"xmlns,attr,omitempty"`
	ResourceRecordSets []R53ResourceRecordSet `xml:"ResourceRecordSets>ResourceRecordSet"`
	IsTruncated        bool                   `xml:"IsTruncated"`
	MaxItems           string                 `xml:"MaxItems"`
	NextRecordName     string                 `xml:"NextRecordName,omitempty"`
	NextRecordType     string                 `xml:"NextRecordType,omitempty"`
}

type R53GetChangeResponse struct {
	XMLName    xml.Name      `xml:"GetChangeResponse"`
	Xmlns      string        `xml:"xmlns,attr,omitempty"`
	ChangeInfo R53ChangeInfo `xml:"ChangeInfo"`
}

type R53DeleteHostedZoneResponse struct {
	XMLName    xml.Name      `xml:"DeleteHostedZoneResponse"`
	Xmlns      string        `xml:"xmlns,attr,omitempty"`
	ChangeInfo R53ChangeInfo `xml:"ChangeInfo"`
}

// ---------- Error ----------

type R53ErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	Xmlns     string   `xml:"xmlns,attr,omitempty"`
	Error     R53Error `xml:"Error"`
	RequestId string   `xml:"RequestId"`
}

type R53Error struct {
	Type    string `xml:"Type,omitempty"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// ---------- Storage ----------

type r53StoredZone struct {
	Zone    R53HostedZone
	Records []R53ResourceRecordSet // keyed by (Name, Type, SetIdentifier)
}

type r53StoredChange struct {
	Info R53ChangeInfo
}

var (
	r53Zones   sim.Store[r53StoredZone]
	r53Changes sim.Store[r53StoredChange]
	r53Mu      sync.Mutex // serialises ChangeResourceRecordSets
)

// ---------- Helpers ----------

func r53RandomID() string {
	buf := make([]byte, 7)
	_, _ = rand.Read(buf)
	return "Z" + strings.ToUpper(hex.EncodeToString(buf))
}

func r53ChangeID() string {
	buf := make([]byte, 7)
	_, _ = rand.Read(buf)
	return "C" + strings.ToUpper(hex.EncodeToString(buf))
}

func r53NowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func r53ZoneIDFromPath(p string) string {
	// AWS accepts either "Z123..." or "/hostedzone/Z123..."
	if strings.HasPrefix(p, "/hostedzone/") {
		return strings.TrimPrefix(p, "/hostedzone/")
	}
	return p
}

func r53WriteXML(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, xml.Header)
	_ = xml.NewEncoder(w).Encode(v)
}

func r53WriteError(w http.ResponseWriter, status int, code, msg string) {
	r53WriteXML(w, status, R53ErrorResponse{
		Xmlns:     r53Namespace,
		Error:     R53Error{Type: "Sender", Code: code, Message: msg},
		RequestId: r53ChangeID(),
	})
}

// ---------- Registration ----------

func registerRoute53(srv *sim.Server) {
	r53Zones = sim.MakeStore[r53StoredZone](srv.DB(), "route53_zones")
	r53Changes = sim.MakeStore[r53StoredChange](srv.DB(), "route53_changes")

	mux := srv.Mux()
	mux.HandleFunc("POST /"+r53APIVersion+"/hostedzone", handleR53CreateHostedZone)
	mux.HandleFunc("GET /"+r53APIVersion+"/hostedzone", handleR53ListHostedZones)
	mux.HandleFunc("GET /"+r53APIVersion+"/hostedzone/{id}", handleR53GetHostedZone)
	mux.HandleFunc("DELETE /"+r53APIVersion+"/hostedzone/{id}", handleR53DeleteHostedZone)
	mux.HandleFunc("POST /"+r53APIVersion+"/hostedzone/{id}/rrset", handleR53ChangeRRSets)
	mux.HandleFunc("POST /"+r53APIVersion+"/hostedzone/{id}/rrset/", handleR53ChangeRRSets) // CLI uses trailing slash
	mux.HandleFunc("GET /"+r53APIVersion+"/hostedzone/{id}/rrset", handleR53ListRRSets)
	mux.HandleFunc("GET /"+r53APIVersion+"/hostedzone/{id}/rrset/", handleR53ListRRSets)
	mux.HandleFunc("GET /"+r53APIVersion+"/change/{id}", handleR53GetChange)

	// Tagging — path /2013-04-01/tags/{ResourceType}/{ResourceId}
	mux.HandleFunc("GET /"+r53APIVersion+"/tags/{resourceType}/{resourceId}", handleR53ListTagsForResource)
	mux.HandleFunc("POST /"+r53APIVersion+"/tags/{resourceType}/{resourceId}", handleR53ChangeTagsForResource)
}

// ---------- Tag types ----------

type R53Tag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value,omitempty"`
}

type R53ChangeTagsRequest struct {
	XMLName xml.Name `xml:"ChangeTagsForResourceRequest"`
	AddTags struct {
		Items []R53Tag `xml:"Tag"`
	} `xml:"AddTags,omitempty"`
	RemoveTagKeys struct {
		Items []string `xml:"Key"`
	} `xml:"RemoveTagKeys,omitempty"`
}

type R53ResourceTagSet struct {
	ResourceType string   `xml:"ResourceType"`
	ResourceId   string   `xml:"ResourceId"`
	Tags         []R53Tag `xml:"Tags>Tag,omitempty"`
}

type R53ListTagsResponse struct {
	XMLName        xml.Name          `xml:"ListTagsForResourceResponse"`
	Xmlns          string            `xml:"xmlns,attr,omitempty"`
	ResourceTagSet R53ResourceTagSet `xml:"ResourceTagSet"`
}

type R53ChangeTagsResponse struct {
	XMLName xml.Name `xml:"ChangeTagsForResourceResponse"`
	Xmlns   string   `xml:"xmlns,attr,omitempty"`
}

// r53Tags maps "<resourceType>/<resourceId>" → tags. Stored separately
// from the zone struct so the existing zone shape doesn't change.
var r53Tags = sync.Map{}

func r53TagKey(rtype, rid string) string { return rtype + "/" + rid }

func handleR53ListTagsForResource(w http.ResponseWriter, r *http.Request) {
	rtype := r.PathValue("resourceType")
	rid := r.PathValue("resourceId")
	tagsAny, _ := r53Tags.Load(r53TagKey(rtype, rid))
	tags, _ := tagsAny.([]R53Tag)
	r53WriteXML(w, http.StatusOK, R53ListTagsResponse{
		Xmlns: r53Namespace,
		ResourceTagSet: R53ResourceTagSet{
			ResourceType: rtype, ResourceId: rid, Tags: tags,
		},
	})
}

func handleR53ChangeTagsForResource(w http.ResponseWriter, r *http.Request) {
	rtype := r.PathValue("resourceType")
	rid := r.PathValue("resourceId")
	var req R53ChangeTagsRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		r53WriteError(w, http.StatusBadRequest, "MalformedXML", "Could not decode request: "+err.Error())
		return
	}
	key := r53TagKey(rtype, rid)
	tagsAny, _ := r53Tags.Load(key)
	tags, _ := tagsAny.([]R53Tag)
	tagMap := map[string]string{}
	for _, t := range tags {
		tagMap[t.Key] = t.Value
	}
	for _, t := range req.AddTags.Items {
		tagMap[t.Key] = t.Value
	}
	for _, k := range req.RemoveTagKeys.Items {
		delete(tagMap, k)
	}
	merged := make([]R53Tag, 0, len(tagMap))
	for k, v := range tagMap {
		merged = append(merged, R53Tag{Key: k, Value: v})
	}
	r53Tags.Store(key, merged)
	r53WriteXML(w, http.StatusOK, R53ChangeTagsResponse{Xmlns: r53Namespace})
}

// ---------- HostedZone handlers ----------

func handleR53CreateHostedZone(w http.ResponseWriter, r *http.Request) {
	var req R53CreateHostedZoneRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		r53WriteError(w, http.StatusBadRequest, "MalformedXML", "Could not decode request: "+err.Error())
		return
	}
	if req.Name == "" {
		r53WriteError(w, http.StatusBadRequest, "InvalidDomainName", "Name is required")
		return
	}
	if req.CallerReference == "" {
		r53WriteError(w, http.StatusBadRequest, "InvalidInput", "CallerReference is required")
		return
	}
	// Ensure name has a trailing dot (canonical Route 53 form).
	name := req.Name
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}
	id := r53RandomID()
	zone := R53HostedZone{
		Xmlns:                  r53Namespace,
		Id:                     "/hostedzone/" + id,
		Name:                   name,
		CallerReference:        req.CallerReference,
		Config:                 req.HostedZoneConfig,
		ResourceRecordSetCount: 2, // NS + SOA records auto-created
	}
	// Seed NS + SOA per real Route 53 behavior so ListResourceRecordSets
	// returns them on a zone-create round-trip.
	defaultRecords := []R53ResourceRecordSet{
		{
			Name: name,
			Type: "NS",
			TTL:  ptrInt64(172800),
			ResourceRecords: &R53ResourceRecords{Items: []R53ResourceRecord{
				{Value: "ns-1.awsdns-00.com."},
				{Value: "ns-2.awsdns-01.net."},
				{Value: "ns-3.awsdns-02.org."},
				{Value: "ns-4.awsdns-03.co.uk."},
			}},
		},
		{
			Name: name,
			Type: "SOA",
			TTL:  ptrInt64(900),
			ResourceRecords: &R53ResourceRecords{Items: []R53ResourceRecord{
				{Value: "ns-1.awsdns-00.com. awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400"},
			}},
		},
	}
	r53Zones.Put(id, r53StoredZone{Zone: zone, Records: defaultRecords})
	change := newR53Change("INSYNC", "Hosted zone created")
	r53Changes.Put(strings.TrimPrefix(change.Id, "/change/"), r53StoredChange{Info: change})
	w.Header().Set("Location", "https://route53.amazonaws.com/"+r53APIVersion+"/hostedzone/"+id)
	r53WriteXML(w, http.StatusCreated, R53CreateHostedZoneResponse{
		Xmlns:         r53Namespace,
		HostedZone:    zone,
		ChangeInfo:    change,
		DelegationSet: R53DelegationSet{NameServers: []string{"ns-1.awsdns-00.com", "ns-2.awsdns-01.net", "ns-3.awsdns-02.org", "ns-4.awsdns-03.co.uk"}},
	})
}

func ptrInt64(v int64) *int64 { return &v }

func newR53Change(status, comment string) R53ChangeInfo {
	return R53ChangeInfo{
		Id:          "/change/" + r53ChangeID(),
		Status:      status,
		SubmittedAt: r53NowISO(),
		Comment:     comment,
	}
}

func handleR53GetHostedZone(w http.ResponseWriter, r *http.Request) {
	id := r53ZoneIDFromPath(r.PathValue("id"))
	stored, ok := r53Zones.Get(id)
	if !ok {
		r53WriteError(w, http.StatusNotFound, "NoSuchHostedZone", "The specified hosted zone does not exist.")
		return
	}
	r53WriteXML(w, http.StatusOK, R53GetHostedZoneResponse{
		Xmlns:         r53Namespace,
		HostedZone:    stored.Zone,
		DelegationSet: R53DelegationSet{NameServers: []string{"ns-1.awsdns-00.com", "ns-2.awsdns-01.net", "ns-3.awsdns-02.org", "ns-4.awsdns-03.co.uk"}},
	})
}

func handleR53DeleteHostedZone(w http.ResponseWriter, r *http.Request) {
	id := r53ZoneIDFromPath(r.PathValue("id"))
	stored, ok := r53Zones.Get(id)
	if !ok {
		r53WriteError(w, http.StatusNotFound, "NoSuchHostedZone", "The specified hosted zone does not exist.")
		return
	}
	// Real Route 53 requires the zone to have only NS + SOA records.
	userRecords := 0
	for _, rr := range stored.Records {
		if rr.Type != "NS" && rr.Type != "SOA" {
			userRecords++
		}
	}
	if userRecords > 0 {
		r53WriteError(w, http.StatusBadRequest, "HostedZoneNotEmpty",
			fmt.Sprintf("The hosted zone is not empty (%d non-required records).", userRecords))
		return
	}
	r53Zones.Delete(id)
	change := newR53Change("INSYNC", "Hosted zone deleted")
	r53Changes.Put(strings.TrimPrefix(change.Id, "/change/"), r53StoredChange{Info: change})
	r53WriteXML(w, http.StatusOK, R53DeleteHostedZoneResponse{Xmlns: r53Namespace, ChangeInfo: change})
}

func handleR53ListHostedZones(w http.ResponseWriter, r *http.Request) {
	items := []R53HostedZone{}
	for _, stored := range r53Zones.List() {
		items = append(items, stored.Zone)
	}
	r53WriteXML(w, http.StatusOK, R53HostedZoneList{
		Xmlns:       r53Namespace,
		HostedZones: items,
		Marker:      "",
		IsTruncated: false,
		MaxItems:    "100",
	})
}

// ---------- ResourceRecordSet handlers ----------

func handleR53ChangeRRSets(w http.ResponseWriter, r *http.Request) {
	r53Mu.Lock()
	defer r53Mu.Unlock()

	id := r53ZoneIDFromPath(r.PathValue("id"))
	stored, ok := r53Zones.Get(id)
	if !ok {
		r53WriteError(w, http.StatusNotFound, "NoSuchHostedZone", "The specified hosted zone does not exist.")
		return
	}
	var req R53ChangeRRSetRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		r53WriteError(w, http.StatusBadRequest, "MalformedXML", "Could not decode request: "+err.Error())
		return
	}
	for _, ch := range req.ChangeBatch.Changes {
		// Real Route 53 stores all names with a trailing dot. The SDK
		// preserves what the caller sent, so Terraform / aws CLI / SDK
		// can send either form. Normalise on store so the List filter
		// can do exact match.
		if !strings.HasSuffix(ch.ResourceRecordSet.Name, ".") {
			ch.ResourceRecordSet.Name += "."
		}
		switch strings.ToUpper(ch.Action) {
		case "CREATE":
			if rrsetExists(stored.Records, ch.ResourceRecordSet) {
				r53WriteError(w, http.StatusBadRequest, "InvalidChangeBatch",
					"Tried to create resource record set that already exists.")
				return
			}
			stored.Records = append(stored.Records, ch.ResourceRecordSet)
		case "UPSERT":
			stored.Records = rrsetReplace(stored.Records, ch.ResourceRecordSet)
		case "DELETE":
			before := len(stored.Records)
			stored.Records = rrsetDelete(stored.Records, ch.ResourceRecordSet)
			if len(stored.Records) == before {
				r53WriteError(w, http.StatusBadRequest, "InvalidChangeBatch",
					"Tried to delete resource record set that does not exist.")
				return
			}
		default:
			r53WriteError(w, http.StatusBadRequest, "InvalidChangeBatch",
				"Unknown change action: "+ch.Action)
			return
		}
	}
	stored.Zone.ResourceRecordSetCount = len(stored.Records)
	r53Zones.Put(id, stored)
	change := newR53Change("INSYNC", req.ChangeBatch.Comment)
	r53Changes.Put(strings.TrimPrefix(change.Id, "/change/"), r53StoredChange{Info: change})
	r53WriteXML(w, http.StatusOK, R53ChangeResourceRecordSetsResponse{
		Xmlns: r53Namespace, ChangeInfo: change,
	})
}

func rrsetKey(rr R53ResourceRecordSet) string {
	return strings.ToLower(rr.Name) + "|" + strings.ToUpper(rr.Type) + "|" + rr.SetIdentifier
}

func rrsetExists(records []R53ResourceRecordSet, rr R53ResourceRecordSet) bool {
	key := rrsetKey(rr)
	for _, r := range records {
		if rrsetKey(r) == key {
			return true
		}
	}
	return false
}

func rrsetReplace(records []R53ResourceRecordSet, rr R53ResourceRecordSet) []R53ResourceRecordSet {
	key := rrsetKey(rr)
	for i, r := range records {
		if rrsetKey(r) == key {
			records[i] = rr
			return records
		}
	}
	return append(records, rr)
}

func rrsetDelete(records []R53ResourceRecordSet, rr R53ResourceRecordSet) []R53ResourceRecordSet {
	key := rrsetKey(rr)
	out := records[:0]
	for _, r := range records {
		if rrsetKey(r) == key {
			continue
		}
		out = append(out, r)
	}
	return out
}

func handleR53ListRRSets(w http.ResponseWriter, r *http.Request) {
	id := r53ZoneIDFromPath(r.PathValue("id"))
	stored, ok := r53Zones.Get(id)
	if !ok {
		r53WriteError(w, http.StatusNotFound, "NoSuchHostedZone", "The specified hosted zone does not exist.")
		return
	}
	// Real Route 53 ListResourceRecordSets honors StartRecordName +
	// StartRecordType as a pagination cursor — records BEFORE the start
	// position are not returned. The Terraform aws_route53_record
	// resource's Read function uses this as a precise filter and treats
	// "first record returned doesn't match my name/type" as "record not
	// found." Sim implements the filter so cross-resource reads work.
	startName := strings.ToLower(r.URL.Query().Get("name"))
	startType := strings.ToUpper(r.URL.Query().Get("type"))
	if startName != "" && !strings.HasSuffix(startName, ".") {
		startName += "."
	}
	records := stored.Records
	if startName != "" {
		out := make([]R53ResourceRecordSet, 0, len(records))
		startIdx := -1
		for i, rr := range records {
			rn := strings.ToLower(rr.Name)
			if rn < startName {
				continue
			}
			if rn == startName {
				if startType != "" && strings.ToUpper(rr.Type) < startType {
					continue
				}
			}
			startIdx = i
			break
		}
		if startIdx >= 0 {
			out = append(out, records[startIdx:]...)
		}
		records = out
	}
	r53WriteXML(w, http.StatusOK, R53ListResourceRecordSetsResponse{
		Xmlns:              r53Namespace,
		ResourceRecordSets: records,
		IsTruncated:        false,
		MaxItems:           "100",
	})
}

func handleR53GetChange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.PathValue("id"), "/change/")
	stored, ok := r53Changes.Get(id)
	if !ok {
		// Real Route 53 returns INSYNC for any unknown change ID after a
		// short window (changes are short-lived). Sim does the same so
		// Terraform's apply-then-poll converges.
		r53WriteXML(w, http.StatusOK, R53GetChangeResponse{
			Xmlns:      r53Namespace,
			ChangeInfo: R53ChangeInfo{Id: "/change/" + id, Status: "INSYNC", SubmittedAt: r53NowISO()},
		})
		return
	}
	r53WriteXML(w, http.StatusOK, R53GetChangeResponse{Xmlns: r53Namespace, ChangeInfo: stored.Info})
}
