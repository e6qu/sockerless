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

var ecClusters sim.Store[ECCluster]

func registerElastiCache(r *sim.AWSQueryRouter, srv *sim.Server) {
	ecClusters = sim.MakeStore[ECCluster](srv.DB(), "elasticache_clusters")
	r.Register("CreateCacheCluster", handleECCreate)
	r.Register("DescribeCacheClusters", handleECDescribe)
	r.Register("ModifyCacheCluster", handleECModify)
	r.Register("DeleteCacheCluster", handleECDelete)
	r.Register("AddTagsToResource", handleECAddTags)
	r.Register("ListTagsForResource", handleECListTags)
	r.Register("RemoveTagsFromResource", handleECRemoveTags)
}

func ecClusterARN(id string) string {
	return fmt.Sprintf("arn:aws:elasticache:%s:%s:cluster:%s", awsRegion(), awsAccountID(), id)
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
	num := atoiOrZero(r.FormValue("NumCacheNodes"))
	if num == 0 {
		num = 1
	}
	c := ECCluster{
		CacheClusterId:         id,
		CacheNodeType:          r.FormValue("CacheNodeType"),
		Engine:                 engine,
		EngineVersion:          r.FormValue("EngineVersion"),
		CacheClusterStatus:     "available",
		NumCacheNodes:          num,
		CacheClusterCreateTime: time.Now().UTC().Format(time.RFC3339),
		ARN:                    ecClusterARN(id),
		Endpoint:               fmt.Sprintf("%s.%s.cache.amazonaws.com", id, awsRegion()),
		Port:                   port,
		Tags:                   map[string]string{},
	}
	ecClusters.Put(id, c)
	ecXMLResponse(w, "CreateCacheCluster", renderECCluster(c), sim.RequestID(r.Context()))
}

func handleECDescribe(w http.ResponseWriter, r *http.Request) {
	wanted := r.FormValue("CacheClusterId")
	var b strings.Builder
	b.WriteString("<CacheClusters>")
	for _, c := range ecClusters.List() {
		if wanted != "" && c.CacheClusterId != wanted {
			continue
		}
		b.WriteString(renderECCluster(c))
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

func handleECAddTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	c, ok := findECByARN(arn)
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecClusters.Update(c.CacheClusterId, func(cc *ECCluster) {
		if cc.Tags == nil {
			cc.Tags = map[string]string{}
		}
		for n := 1; n <= 50; n++ {
			k := r.FormValue(fmt.Sprintf("Tags.Tag.%d.Key", n))
			v := r.FormValue(fmt.Sprintf("Tags.Tag.%d.Value", n))
			if k == "" {
				break
			}
			cc.Tags[k] = v
		}
	})
	ecXMLResponse(w, "AddTagsToResource", "<TagList></TagList>", sim.RequestID(r.Context()))
}

func handleECListTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	c, ok := findECByARN(arn)
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	var b strings.Builder
	b.WriteString("<TagList>")
	for k, v := range c.Tags {
		fmt.Fprintf(&b, "<Tag><Key>%s</Key><Value>%s</Value></Tag>", xmlEscape(k), xmlEscape(v))
	}
	b.WriteString("</TagList>")
	ecXMLResponse(w, "ListTagsForResource", b.String(), sim.RequestID(r.Context()))
}

func handleECRemoveTags(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ResourceName")
	c, ok := findECByARN(arn)
	if !ok {
		ecErrorXML(w, "CacheClusterNotFound", "Resource not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	ecClusters.Update(c.CacheClusterId, func(cc *ECCluster) {
		for n := 1; n <= 50; n++ {
			k := r.FormValue(fmt.Sprintf("TagKeys.member.%d", n))
			if k == "" {
				break
			}
			delete(cc.Tags, k)
		}
	})
	ecXMLResponse(w, "RemoveTagsFromResource", "<TagList></TagList>", sim.RequestID(r.Context()))
}

func findECByARN(arn string) (ECCluster, bool) {
	for _, c := range ecClusters.List() {
		if c.ARN == arn {
			return c, true
		}
	}
	if c, ok := ecClusters.Get(arn); ok {
		return c, true
	}
	return ECCluster{}, false
}
