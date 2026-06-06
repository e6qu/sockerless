package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

const elbv2APIVersion = "2015-12-01"

type ELBv2LoadBalancer struct {
	Arn            string
	Name           string
	DNSName        string
	CanonicalZone  string
	Scheme         string
	Type           string
	State          string
	VpcID          string
	Subnets        []string
	SecurityGroups []string
	IpAddressType  string
	CreatedTime    string
	Tags           map[string]string
	Attributes     map[string]string
}

type ELBv2TargetGroup struct {
	Arn                     string
	Name                    string
	Protocol                string
	Port                    int
	VpcID                   string
	TargetType              string
	HealthCheckProtocol     string
	HealthCheckPort         string
	HealthCheckPath         string
	HealthCheckEnabled      bool
	HealthCheckInterval     int
	HealthCheckTimeout      int
	HealthyThresholdCount   int
	UnhealthyThresholdCount int
	LoadBalancerArns        []string
	Targets                 []ELBv2TargetDescription
	Tags                    map[string]string
	Attributes              map[string]string
}

type ELBv2Listener struct {
	Arn             string
	LoadBalancerArn string
	Protocol        string
	Port            int
	DefaultActions  []ELBv2Action
	Certificates    []string
	SslPolicy       string
	Attributes      map[string]string
}

type ELBv2Action struct {
	Type           string
	TargetGroupArn string
	Order          int
	FixedResponse  *ELBv2FixedResponseConfig
	Redirect       *ELBv2RedirectConfig
}

type ELBv2FixedResponseConfig struct {
	StatusCode  string
	ContentType string
	MessageBody string
}

type ELBv2RedirectConfig struct {
	Protocol   string
	Port       string
	Host       string
	Path       string
	Query      string
	StatusCode string
}

type ELBv2TargetDescription struct {
	ID               string
	Port             int
	AvailabilityZone string
}

var (
	elbv2LoadBalancers sim.Store[ELBv2LoadBalancer]
	elbv2TargetGroups  sim.Store[ELBv2TargetGroup]
	elbv2Listeners     sim.Store[ELBv2Listener]
)

func registerELBv2(r *sim.AWSQueryRouter, srv *sim.Server) {
	elbv2LoadBalancers = sim.MakeStore[ELBv2LoadBalancer](srv.DB(), "elbv2_load_balancers")
	elbv2TargetGroups = sim.MakeStore[ELBv2TargetGroup](srv.DB(), "elbv2_target_groups")
	elbv2Listeners = sim.MakeStore[ELBv2Listener](srv.DB(), "elbv2_listeners")

	r.RegisterVersioned(elbv2APIVersion, "CreateLoadBalancer", handleELBv2CreateLoadBalancer)
	r.RegisterVersioned(elbv2APIVersion, "DescribeLoadBalancers", handleELBv2DescribeLoadBalancers)
	r.RegisterVersioned(elbv2APIVersion, "DeleteLoadBalancer", handleELBv2DeleteLoadBalancer)
	r.RegisterVersioned(elbv2APIVersion, "ModifyLoadBalancerAttributes", handleELBv2ModifyLoadBalancerAttributes)
	r.RegisterVersioned(elbv2APIVersion, "DescribeLoadBalancerAttributes", handleELBv2DescribeLoadBalancerAttributes)
	r.RegisterVersioned(elbv2APIVersion, "DescribeCapacityReservation", handleELBv2DescribeCapacityReservation)
	r.RegisterVersioned(elbv2APIVersion, "SetSecurityGroups", handleELBv2SetSecurityGroups)
	r.RegisterVersioned(elbv2APIVersion, "SetSubnets", handleELBv2SetSubnets)

	r.RegisterVersioned(elbv2APIVersion, "CreateTargetGroup", handleELBv2CreateTargetGroup)
	r.RegisterVersioned(elbv2APIVersion, "DescribeTargetGroups", handleELBv2DescribeTargetGroups)
	r.RegisterVersioned(elbv2APIVersion, "DeleteTargetGroup", handleELBv2DeleteTargetGroup)
	r.RegisterVersioned(elbv2APIVersion, "ModifyTargetGroup", handleELBv2ModifyTargetGroup)
	r.RegisterVersioned(elbv2APIVersion, "ModifyTargetGroupAttributes", handleELBv2ModifyTargetGroupAttributes)
	r.RegisterVersioned(elbv2APIVersion, "DescribeTargetGroupAttributes", handleELBv2DescribeTargetGroupAttributes)
	r.RegisterVersioned(elbv2APIVersion, "RegisterTargets", handleELBv2RegisterTargets)
	r.RegisterVersioned(elbv2APIVersion, "DeregisterTargets", handleELBv2DeregisterTargets)
	r.RegisterVersioned(elbv2APIVersion, "DescribeTargetHealth", handleELBv2DescribeTargetHealth)

	r.RegisterVersioned(elbv2APIVersion, "CreateListener", handleELBv2CreateListener)
	r.RegisterVersioned(elbv2APIVersion, "DescribeListeners", handleELBv2DescribeListeners)
	r.RegisterVersioned(elbv2APIVersion, "DescribeListenerAttributes", handleELBv2DescribeListenerAttributes)
	r.RegisterVersioned(elbv2APIVersion, "ModifyListenerAttributes", handleELBv2ModifyListenerAttributes)
	r.RegisterVersioned(elbv2APIVersion, "DeleteListener", handleELBv2DeleteListener)

	r.RegisterVersioned(elbv2APIVersion, "AddTags", handleELBv2AddTags)
	r.RegisterVersioned(elbv2APIVersion, "RemoveTags", handleELBv2RemoveTags)
	r.RegisterVersioned(elbv2APIVersion, "DescribeTags", handleELBv2DescribeTags)
	r.RegisterVersioned(elbv2APIVersion, "DescribeAccountLimits", handleELBv2DescribeAccountLimits)

	registerELBv2Rules(r, srv)
}

func elbv2XMLResponse(w http.ResponseWriter, op string, body string, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w,
		`<%sResponse xmlns="http://elasticloadbalancing.amazonaws.com/doc/2015-12-01/"><%sResult>%s</%sResult><ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata></%sResponse>`,
		op, op, body, op, requestID, op)
}

func elbv2ErrorXML(w http.ResponseWriter, code, message string, status int, requestID string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w,
		`<ErrorResponse xmlns="http://elasticloadbalancing.amazonaws.com/doc/2015-12-01/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
		xmlEscape(code), xmlEscape(message), xmlEscape(requestID))
}

func handleELBv2CreateLoadBalancer(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("Name")
	if name == "" {
		elbv2ErrorXML(w, "ValidationError", "Name is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	lbType := r.FormValue("Type")
	if lbType == "" {
		lbType = "application"
	}
	scheme := r.FormValue("Scheme")
	if scheme == "" {
		scheme = "internet-facing"
	}
	ipType := r.FormValue("IpAddressType")
	if ipType == "" {
		ipType = "ipv4"
	}
	id := generateUUID()[:12]
	resourceKind := "app"
	if lbType == "network" {
		resourceKind = "net"
	}
	arn := fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:loadbalancer/%s/%s/%s", awsRegion(), awsAccountID(), resourceKind, name, id)
	lb := ELBv2LoadBalancer{
		Arn:            arn,
		Name:           name,
		DNSName:        fmt.Sprintf("%s-%s.elb.%s.amazonaws.com", name, id[:8], awsRegion()),
		CanonicalZone:  "Z35SXDOTRQ7X7K",
		Scheme:         scheme,
		Type:           lbType,
		State:          "active",
		VpcID:          elbv2VPCFromSubnets(queryList(r, "Subnets")),
		Subnets:        queryList(r, "Subnets"),
		SecurityGroups: queryList(r, "SecurityGroups"),
		IpAddressType:  ipType,
		CreatedTime:    time.Now().UTC().Format(time.RFC3339),
		Tags:           parseELBv2Tags(r, "Tags"),
		Attributes:     defaultELBv2LoadBalancerAttributes(),
	}
	elbv2LoadBalancers.Put(arn, lb)
	elbv2XMLResponse(w, "CreateLoadBalancer", "<LoadBalancers>"+elbv2LoadBalancerXML(lb)+"</LoadBalancers>", sim.RequestID(r.Context()))
}

func handleELBv2DescribeLoadBalancers(w http.ResponseWriter, r *http.Request) {
	lbs := filterELBv2LoadBalancers(r)
	var b strings.Builder
	b.WriteString("<LoadBalancers>")
	for _, lb := range lbs {
		b.WriteString(elbv2LoadBalancerXML(lb))
	}
	b.WriteString("</LoadBalancers>")
	elbv2XMLResponse(w, "DescribeLoadBalancers", b.String(), sim.RequestID(r.Context()))
}

func handleELBv2DeleteLoadBalancer(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("LoadBalancerArn")
	elbv2LoadBalancers.Delete(arn)
	for _, listener := range elbv2Listeners.Filter(func(l ELBv2Listener) bool { return l.LoadBalancerArn == arn }) {
		elbv2Listeners.Delete(listener.Arn)
	}
	for _, tg := range elbv2TargetGroups.List() {
		tg.LoadBalancerArns = removeString(tg.LoadBalancerArns, arn)
		elbv2TargetGroups.Put(tg.Arn, tg)
	}
	elbv2XMLResponse(w, "DeleteLoadBalancer", "", sim.RequestID(r.Context()))
}

func handleELBv2ModifyLoadBalancerAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("LoadBalancerArn")
	if !elbv2LoadBalancers.Update(arn, func(lb *ELBv2LoadBalancer) {
		for k, v := range parseELBv2Attributes(r, "Attributes") {
			lb.Attributes[k] = v
		}
	}) {
		elbv2ErrorXML(w, "LoadBalancerNotFound", "Load balancer not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	attrs := defaultELBv2LoadBalancerAttributes()
	if lb, ok := elbv2LoadBalancers.Get(arn); ok {
		attrs = lb.Attributes
	}
	elbv2XMLResponse(w, "ModifyLoadBalancerAttributes", elbv2AttributesXML("Attributes", attrs), sim.RequestID(r.Context()))
}

func handleELBv2DescribeLoadBalancerAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("LoadBalancerArn")
	lb, ok := elbv2LoadBalancers.Get(arn)
	if !ok {
		elbv2ErrorXML(w, "LoadBalancerNotFound", "Load balancer not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "DescribeLoadBalancerAttributes", elbv2AttributesXML("Attributes", lb.Attributes), sim.RequestID(r.Context()))
}

func handleELBv2DescribeCapacityReservation(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("LoadBalancerArn")
	lb, ok := elbv2LoadBalancers.Get(arn)
	if !ok {
		elbv2ErrorXML(w, "LoadBalancerNotFound", "Load balancer not found", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	var states strings.Builder
	states.WriteString("<CapacityReservationState>")
	for _, subnet := range lb.Subnets {
		az := awsAvailabilityZone()
		if s, ok := ec2Subnets.Get(subnet); ok && s.AvailabilityZone != "" {
			az = s.AvailabilityZone
		}
		fmt.Fprintf(&states, "<member><State><Code>provisioned</Code></State><AvailabilityZone>%s</AvailabilityZone><EffectiveCapacityUnits>0</EffectiveCapacityUnits></member>", xmlEscape(az))
	}
	states.WriteString("</CapacityReservationState>")
	// MinimumLoadBalancerCapacity is omitted unless a minimum was actually
	// configured (via ModifyCapacityReservation, which the sim doesn't model).
	// Emitting CapacityUnits=0 makes the provider read a configured 0 and plan
	// "capacity_units = 0 -> null" on every idempotency check.
	body := fmt.Sprintf("<LastModifiedTime>%s</LastModifiedTime><DecreaseRequestsRemaining>10</DecreaseRequestsRemaining>%s",
		xmlEscape(lb.CreatedTime), states.String())
	elbv2XMLResponse(w, "DescribeCapacityReservation", body, sim.RequestID(r.Context()))
}

func handleELBv2SetSecurityGroups(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("LoadBalancerArn")
	groups := queryList(r, "SecurityGroups")
	if !elbv2LoadBalancers.Update(arn, func(lb *ELBv2LoadBalancer) {
		lb.SecurityGroups = groups
	}) {
		elbv2ErrorXML(w, "LoadBalancerNotFound", "Load balancer not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "SetSecurityGroups", elbv2StringMembersXML("SecurityGroupIds", groups), sim.RequestID(r.Context()))
}

func handleELBv2SetSubnets(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("LoadBalancerArn")
	subnets := queryList(r, "Subnets")
	if !elbv2LoadBalancers.Update(arn, func(lb *ELBv2LoadBalancer) {
		lb.Subnets = subnets
		lb.VpcID = elbv2VPCFromSubnets(subnets)
	}) {
		elbv2ErrorXML(w, "LoadBalancerNotFound", "Load balancer not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "SetSubnets", "<AvailabilityZones>"+elbv2AvailabilityZonesXML(subnets)+"</AvailabilityZones>", sim.RequestID(r.Context()))
}

func handleELBv2CreateTargetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("Name")
	if name == "" {
		elbv2ErrorXML(w, "ValidationError", "Name is required", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	port := atoiDefault(r.FormValue("Port"), 80)
	protocol := r.FormValue("Protocol")
	if protocol == "" {
		protocol = "HTTP"
	}
	targetType := r.FormValue("TargetType")
	if targetType == "" {
		targetType = "instance"
	}
	id := generateUUID()[:12]
	arn := fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:targetgroup/%s/%s", awsRegion(), awsAccountID(), name, id)
	tg := ELBv2TargetGroup{
		Arn:                     arn,
		Name:                    name,
		Protocol:                protocol,
		Port:                    port,
		VpcID:                   r.FormValue("VpcId"),
		TargetType:              targetType,
		HealthCheckProtocol:     firstNonEmpty(r.FormValue("HealthCheckProtocol"), protocol),
		HealthCheckPort:         firstNonEmpty(r.FormValue("HealthCheckPort"), "traffic-port"),
		HealthCheckPath:         firstNonEmpty(r.FormValue("HealthCheckPath"), "/"),
		HealthCheckEnabled:      true,
		HealthCheckInterval:     atoiDefault(r.FormValue("HealthCheckIntervalSeconds"), 30),
		HealthCheckTimeout:      atoiDefault(r.FormValue("HealthCheckTimeoutSeconds"), 5),
		HealthyThresholdCount:   atoiDefault(r.FormValue("HealthyThresholdCount"), 5),
		UnhealthyThresholdCount: atoiDefault(r.FormValue("UnhealthyThresholdCount"), 2),
		Tags:                    parseELBv2Tags(r, "Tags"),
		Attributes:              defaultELBv2TargetGroupAttributes(),
	}
	if r.FormValue("HealthCheckEnabled") == "false" {
		tg.HealthCheckEnabled = false
	}
	elbv2TargetGroups.Put(arn, tg)
	elbv2XMLResponse(w, "CreateTargetGroup", "<TargetGroups>"+elbv2TargetGroupXML(tg)+"</TargetGroups>", sim.RequestID(r.Context()))
}

func handleELBv2DescribeTargetGroups(w http.ResponseWriter, r *http.Request) {
	tgs := filterELBv2TargetGroups(r)
	var b strings.Builder
	b.WriteString("<TargetGroups>")
	for _, tg := range tgs {
		b.WriteString(elbv2TargetGroupXML(tg))
	}
	b.WriteString("</TargetGroups>")
	elbv2XMLResponse(w, "DescribeTargetGroups", b.String(), sim.RequestID(r.Context()))
}

func handleELBv2DeleteTargetGroup(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	elbv2TargetGroups.Delete(arn)
	for _, listener := range elbv2Listeners.List() {
		changed := false
		for i := range listener.DefaultActions {
			if listener.DefaultActions[i].TargetGroupArn == arn {
				listener.DefaultActions[i].TargetGroupArn = ""
				changed = true
			}
		}
		if changed {
			elbv2Listeners.Put(listener.Arn, listener)
		}
	}
	elbv2XMLResponse(w, "DeleteTargetGroup", "", sim.RequestID(r.Context()))
}

func handleELBv2ModifyTargetGroup(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	if !elbv2TargetGroups.Update(arn, func(tg *ELBv2TargetGroup) {
		if v := r.FormValue("HealthCheckProtocol"); v != "" {
			tg.HealthCheckProtocol = v
		}
		if v := r.FormValue("HealthCheckPort"); v != "" {
			tg.HealthCheckPort = v
		}
		if v := r.FormValue("HealthCheckPath"); v != "" {
			tg.HealthCheckPath = v
		}
		if v := r.FormValue("HealthCheckIntervalSeconds"); v != "" {
			tg.HealthCheckInterval = atoiDefault(v, tg.HealthCheckInterval)
		}
		if v := r.FormValue("HealthCheckTimeoutSeconds"); v != "" {
			tg.HealthCheckTimeout = atoiDefault(v, tg.HealthCheckTimeout)
		}
		if v := r.FormValue("HealthyThresholdCount"); v != "" {
			tg.HealthyThresholdCount = atoiDefault(v, tg.HealthyThresholdCount)
		}
		if v := r.FormValue("UnhealthyThresholdCount"); v != "" {
			tg.UnhealthyThresholdCount = atoiDefault(v, tg.UnhealthyThresholdCount)
		}
	}) {
		elbv2ErrorXML(w, "TargetGroupNotFound", "Target group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	tg, _ := elbv2TargetGroups.Get(arn)
	elbv2XMLResponse(w, "ModifyTargetGroup", "<TargetGroups>"+elbv2TargetGroupXML(tg)+"</TargetGroups>", sim.RequestID(r.Context()))
}

func handleELBv2ModifyTargetGroupAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	if !elbv2TargetGroups.Update(arn, func(tg *ELBv2TargetGroup) {
		for k, v := range parseELBv2Attributes(r, "Attributes") {
			tg.Attributes[k] = v
		}
	}) {
		elbv2ErrorXML(w, "TargetGroupNotFound", "Target group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	tg, _ := elbv2TargetGroups.Get(arn)
	elbv2XMLResponse(w, "ModifyTargetGroupAttributes", elbv2AttributesXML("Attributes", tg.Attributes), sim.RequestID(r.Context()))
}

func handleELBv2DescribeTargetGroupAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	tg, ok := elbv2TargetGroups.Get(arn)
	if !ok {
		elbv2ErrorXML(w, "TargetGroupNotFound", "Target group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "DescribeTargetGroupAttributes", elbv2AttributesXML("Attributes", tg.Attributes), sim.RequestID(r.Context()))
}

func handleELBv2RegisterTargets(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	targets := parseELBv2Targets(r)
	if !elbv2TargetGroups.Update(arn, func(tg *ELBv2TargetGroup) {
		for _, incoming := range targets {
			replaced := false
			for i := range tg.Targets {
				if tg.Targets[i].ID == incoming.ID && tg.Targets[i].Port == incoming.Port {
					tg.Targets[i] = incoming
					replaced = true
					break
				}
			}
			if !replaced {
				tg.Targets = append(tg.Targets, incoming)
			}
		}
	}) {
		elbv2ErrorXML(w, "TargetGroupNotFound", "Target group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "RegisterTargets", "", sim.RequestID(r.Context()))
}

func handleELBv2DeregisterTargets(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	targets := parseELBv2Targets(r)
	if !elbv2TargetGroups.Update(arn, func(tg *ELBv2TargetGroup) {
		filtered := tg.Targets[:0]
		for _, existing := range tg.Targets {
			remove := false
			for _, t := range targets {
				if existing.ID == t.ID && (t.Port == 0 || existing.Port == t.Port) {
					remove = true
					break
				}
			}
			if !remove {
				filtered = append(filtered, existing)
			}
		}
		tg.Targets = filtered
	}) {
		elbv2ErrorXML(w, "TargetGroupNotFound", "Target group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "DeregisterTargets", "", sim.RequestID(r.Context()))
}

func handleELBv2DescribeTargetHealth(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("TargetGroupArn")
	tg, ok := elbv2TargetGroups.Get(arn)
	if !ok {
		elbv2ErrorXML(w, "TargetGroupNotFound", "Target group not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	filter := parseELBv2Targets(r)
	targets := tg.Targets
	if len(filter) > 0 {
		targets = nil
		for _, existing := range tg.Targets {
			for _, wanted := range filter {
				if existing.ID == wanted.ID && (wanted.Port == 0 || existing.Port == wanted.Port) {
					targets = append(targets, existing)
					break
				}
			}
		}
	}
	var b strings.Builder
	b.WriteString("<TargetHealthDescriptions>")
	for _, target := range targets {
		state := elbv2ProbeTarget(r.Context(), tg, target)
		fmt.Fprintf(&b, `<member><Target>%s</Target><TargetHealth><State>%s</State></TargetHealth></member>`, elbv2TargetXML(target), state)
	}
	b.WriteString("</TargetHealthDescriptions>")
	elbv2XMLResponse(w, "DescribeTargetHealth", b.String(), sim.RequestID(r.Context()))
}

func handleELBv2CreateListener(w http.ResponseWriter, r *http.Request) {
	lbArn := r.FormValue("LoadBalancerArn")
	lb, ok := elbv2LoadBalancers.Get(lbArn)
	if !ok {
		elbv2ErrorXML(w, "LoadBalancerNotFound", "Load balancer not found", http.StatusNotFound, sim.RequestID(r.Context()))
		return
	}
	protocol := r.FormValue("Protocol")
	if protocol == "" {
		protocol = "HTTP"
	}
	port := atoiDefault(r.FormValue("Port"), 80)
	id := generateUUID()[:12]
	arn := fmt.Sprintf("arn:aws:elasticloadbalancing:%s:%s:listener/%s/%s/%s/%s", awsRegion(), awsAccountID(), elbv2LoadBalancerKind(lb), lb.Name, elbv2LoadBalancerID(lb.Arn), id)
	listener := ELBv2Listener{
		Arn:             arn,
		LoadBalancerArn: lbArn,
		Protocol:        protocol,
		Port:            port,
		DefaultActions:  parseELBv2Actions(r),
		Certificates:    parseELBv2Certificates(r),
		SslPolicy:       r.FormValue("SslPolicy"),
		Attributes:      defaultELBv2ListenerAttributes(lb.Type),
	}
	elbv2Listeners.Put(arn, listener)
	for _, action := range listener.DefaultActions {
		if action.TargetGroupArn != "" {
			elbv2TargetGroups.Update(action.TargetGroupArn, func(tg *ELBv2TargetGroup) {
				tg.LoadBalancerArns = appendUnique(tg.LoadBalancerArns, lbArn)
			})
		}
	}
	elbv2XMLResponse(w, "CreateListener", "<Listeners>"+elbv2ListenerXML(listener)+"</Listeners>", sim.RequestID(r.Context()))
}

func handleELBv2DescribeListeners(w http.ResponseWriter, r *http.Request) {
	listeners := filterELBv2Listeners(r)
	var b strings.Builder
	b.WriteString("<Listeners>")
	for _, listener := range listeners {
		b.WriteString(elbv2ListenerXML(listener))
	}
	b.WriteString("</Listeners>")
	elbv2XMLResponse(w, "DescribeListeners", b.String(), sim.RequestID(r.Context()))
}

func handleELBv2DescribeListenerAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ListenerArn")
	listener, ok := elbv2Listeners.Get(arn)
	if !ok {
		elbv2ErrorXML(w, "ListenerNotFound", "Listener not found", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	elbv2XMLResponse(w, "DescribeListenerAttributes", elbv2AttributesXML("Attributes", elbv2ListenerAttributes(listener)), sim.RequestID(r.Context()))
}

func handleELBv2ModifyListenerAttributes(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ListenerArn")
	if !elbv2Listeners.Update(arn, func(listener *ELBv2Listener) {
		if listener.Attributes == nil {
			listener.Attributes = elbv2ListenerAttributes(*listener)
		}
		for k, v := range parseELBv2Attributes(r, "Attributes") {
			listener.Attributes[k] = v
		}
	}) {
		elbv2ErrorXML(w, "ListenerNotFound", "Listener not found", http.StatusBadRequest, sim.RequestID(r.Context()))
		return
	}
	listener, _ := elbv2Listeners.Get(arn)
	elbv2XMLResponse(w, "ModifyListenerAttributes", elbv2AttributesXML("Attributes", elbv2ListenerAttributes(listener)), sim.RequestID(r.Context()))
}

func handleELBv2DeleteListener(w http.ResponseWriter, r *http.Request) {
	arn := r.FormValue("ListenerArn")
	elbv2Listeners.Delete(arn)
	elbv2XMLResponse(w, "DeleteListener", "", sim.RequestID(r.Context()))
}

func handleELBv2AddTags(w http.ResponseWriter, r *http.Request) {
	tags := parseELBv2Tags(r, "Tags")
	for _, arn := range queryList(r, "ResourceArns") {
		elbv2SetResourceTags(arn, tags, false)
	}
	elbv2XMLResponse(w, "AddTags", "", sim.RequestID(r.Context()))
}

func handleELBv2RemoveTags(w http.ResponseWriter, r *http.Request) {
	keys := queryList(r, "TagKeys")
	for _, arn := range queryList(r, "ResourceArns") {
		elbv2SetResourceTags(arn, keysToMap(keys), true)
	}
	elbv2XMLResponse(w, "RemoveTags", "", sim.RequestID(r.Context()))
}

func handleELBv2DescribeTags(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("<TagDescriptions>")
	for _, arn := range queryList(r, "ResourceArns") {
		fmt.Fprintf(&b, "<member><ResourceArn>%s</ResourceArn><Tags>", xmlEscape(arn))
		for k, v := range elbv2ResourceTags(arn) {
			fmt.Fprintf(&b, "<member><Key>%s</Key><Value>%s</Value></member>", xmlEscape(k), xmlEscape(v))
		}
		b.WriteString("</Tags></member>")
	}
	b.WriteString("</TagDescriptions>")
	elbv2XMLResponse(w, "DescribeTags", b.String(), sim.RequestID(r.Context()))
}

func handleELBv2DescribeAccountLimits(w http.ResponseWriter, r *http.Request) {
	body := `<Limits><member><Name>application-load-balancers</Name><Max>50</Max></member><member><Name>target-groups</Name><Max>3000</Max></member><member><Name>listeners-per-application-load-balancer</Name><Max>50</Max></member></Limits>`
	elbv2XMLResponse(w, "DescribeAccountLimits", body, sim.RequestID(r.Context()))
}

func filterELBv2LoadBalancers(r *http.Request) []ELBv2LoadBalancer {
	if arns := queryList(r, "LoadBalancerArns"); len(arns) > 0 {
		var out []ELBv2LoadBalancer
		for _, arn := range arns {
			if lb, ok := elbv2LoadBalancers.Get(arn); ok {
				out = append(out, lb)
			}
		}
		return out
	}
	if names := queryList(r, "Names"); len(names) > 0 {
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[n] = true
		}
		return elbv2LoadBalancers.Filter(func(lb ELBv2LoadBalancer) bool { return nameSet[lb.Name] })
	}
	return elbv2LoadBalancers.List()
}

func filterELBv2TargetGroups(r *http.Request) []ELBv2TargetGroup {
	if arns := queryList(r, "TargetGroupArns"); len(arns) > 0 {
		var out []ELBv2TargetGroup
		for _, arn := range arns {
			if tg, ok := elbv2TargetGroups.Get(arn); ok {
				out = append(out, tg)
			}
		}
		return out
	}
	if names := queryList(r, "Names"); len(names) > 0 {
		nameSet := make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[n] = true
		}
		return elbv2TargetGroups.Filter(func(tg ELBv2TargetGroup) bool { return nameSet[tg.Name] })
	}
	if lbArn := r.FormValue("LoadBalancerArn"); lbArn != "" {
		return elbv2TargetGroups.Filter(func(tg ELBv2TargetGroup) bool {
			return containsString(tg.LoadBalancerArns, lbArn)
		})
	}
	return elbv2TargetGroups.List()
}

func filterELBv2Listeners(r *http.Request) []ELBv2Listener {
	if arns := queryList(r, "ListenerArns"); len(arns) > 0 {
		var out []ELBv2Listener
		for _, arn := range arns {
			if listener, ok := elbv2Listeners.Get(arn); ok {
				out = append(out, listener)
			}
		}
		return out
	}
	if lbArn := r.FormValue("LoadBalancerArn"); lbArn != "" {
		return elbv2Listeners.Filter(func(l ELBv2Listener) bool { return l.LoadBalancerArn == lbArn })
	}
	return elbv2Listeners.List()
}

func elbv2LoadBalancerXML(lb ELBv2LoadBalancer) string {
	return fmt.Sprintf(`<member><LoadBalancerArn>%s</LoadBalancerArn><DNSName>%s</DNSName><CanonicalHostedZoneId>%s</CanonicalHostedZoneId><CreatedTime>%s</CreatedTime><LoadBalancerName>%s</LoadBalancerName><Scheme>%s</Scheme><VpcId>%s</VpcId><State><Code>%s</Code></State><Type>%s</Type><AvailabilityZones>%s</AvailabilityZones><SecurityGroups>%s</SecurityGroups><IpAddressType>%s</IpAddressType></member>`,
		xmlEscape(lb.Arn), xmlEscape(lb.DNSName), xmlEscape(lb.CanonicalZone), xmlEscape(lb.CreatedTime), xmlEscape(lb.Name),
		xmlEscape(lb.Scheme), xmlEscape(lb.VpcID), xmlEscape(lb.State), xmlEscape(lb.Type), elbv2AvailabilityZonesXML(lb.Subnets),
		elbv2StringMembersXMLInner(lb.SecurityGroups), xmlEscape(lb.IpAddressType))
}

func elbv2AvailabilityZonesXML(subnets []string) string {
	var b strings.Builder
	for _, subnet := range subnets {
		az := awsAvailabilityZone()
		if s, ok := ec2Subnets.Get(subnet); ok && s.AvailabilityZone != "" {
			az = s.AvailabilityZone
		}
		fmt.Fprintf(&b, "<member><ZoneName>%s</ZoneName><SubnetId>%s</SubnetId></member>", xmlEscape(az), xmlEscape(subnet))
	}
	return b.String()
}

func elbv2TargetGroupXML(tg ELBv2TargetGroup) string {
	return fmt.Sprintf(`<member><TargetGroupArn>%s</TargetGroupArn><TargetGroupName>%s</TargetGroupName><Protocol>%s</Protocol><Port>%d</Port><VpcId>%s</VpcId><HealthCheckProtocol>%s</HealthCheckProtocol><HealthCheckPort>%s</HealthCheckPort><HealthCheckEnabled>%t</HealthCheckEnabled><HealthCheckIntervalSeconds>%d</HealthCheckIntervalSeconds><HealthCheckTimeoutSeconds>%d</HealthCheckTimeoutSeconds><HealthyThresholdCount>%d</HealthyThresholdCount><UnhealthyThresholdCount>%d</UnhealthyThresholdCount><HealthCheckPath>%s</HealthCheckPath><Matcher><HttpCode>200</HttpCode></Matcher><LoadBalancerArns>%s</LoadBalancerArns><TargetType>%s</TargetType></member>`,
		xmlEscape(tg.Arn), xmlEscape(tg.Name), xmlEscape(tg.Protocol), tg.Port, xmlEscape(tg.VpcID),
		xmlEscape(tg.HealthCheckProtocol), xmlEscape(tg.HealthCheckPort), tg.HealthCheckEnabled,
		tg.HealthCheckInterval, tg.HealthCheckTimeout, tg.HealthyThresholdCount, tg.UnhealthyThresholdCount,
		xmlEscape(tg.HealthCheckPath), elbv2StringMembersXMLInner(tg.LoadBalancerArns), xmlEscape(tg.TargetType))
}

func elbv2ListenerXML(listener ELBv2Listener) string {
	var certs strings.Builder
	if len(listener.Certificates) > 0 {
		certs.WriteString("<Certificates>")
		for _, c := range listener.Certificates {
			fmt.Fprintf(&certs, "<member><CertificateArn>%s</CertificateArn></member>", xmlEscape(c))
		}
		certs.WriteString("</Certificates>")
	}
	var sslPolicy string
	if listener.SslPolicy != "" {
		sslPolicy = fmt.Sprintf("<SslPolicy>%s</SslPolicy>", xmlEscape(listener.SslPolicy))
	}
	return fmt.Sprintf(`<member><ListenerArn>%s</ListenerArn><LoadBalancerArn>%s</LoadBalancerArn><Port>%d</Port><Protocol>%s</Protocol>%s%s%s</member>`,
		xmlEscape(listener.Arn), xmlEscape(listener.LoadBalancerArn), listener.Port, xmlEscape(listener.Protocol),
		elbv2ActionsXML("DefaultActions", listener.DefaultActions), certs.String(), sslPolicy)
}

func elbv2TargetXML(target ELBv2TargetDescription) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<Id>%s</Id>", xmlEscape(target.ID))
	if target.Port != 0 {
		fmt.Fprintf(&b, "<Port>%d</Port>", target.Port)
	}
	if target.AvailabilityZone != "" {
		fmt.Fprintf(&b, "<AvailabilityZone>%s</AvailabilityZone>", xmlEscape(target.AvailabilityZone))
	}
	return b.String()
}

func elbv2AttributesXML(wrapper string, attrs map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<%s>", wrapper)
	for k, v := range attrs {
		fmt.Fprintf(&b, "<member><Key>%s</Key><Value>%s</Value></member>", xmlEscape(k), xmlEscape(v))
	}
	fmt.Fprintf(&b, "</%s>", wrapper)
	return b.String()
}

func parseELBv2Tags(r *http.Request, prefix string) map[string]string {
	tags := map[string]string{}
	for i := 1; ; i++ {
		k := r.FormValue(fmt.Sprintf("%s.member.%d.Key", prefix, i))
		if k == "" {
			break
		}
		tags[k] = r.FormValue(fmt.Sprintf("%s.member.%d.Value", prefix, i))
	}
	return tags
}

func parseELBv2Attributes(r *http.Request, prefix string) map[string]string {
	attrs := map[string]string{}
	for i := 1; ; i++ {
		k := r.FormValue(fmt.Sprintf("%s.member.%d.Key", prefix, i))
		if k == "" {
			break
		}
		attrs[k] = r.FormValue(fmt.Sprintf("%s.member.%d.Value", prefix, i))
	}
	return attrs
}

func parseELBv2Targets(r *http.Request) []ELBv2TargetDescription {
	var targets []ELBv2TargetDescription
	for i := 1; ; i++ {
		id := r.FormValue(fmt.Sprintf("Targets.member.%d.Id", i))
		if id == "" {
			break
		}
		targets = append(targets, ELBv2TargetDescription{
			ID:               id,
			Port:             atoiDefault(r.FormValue(fmt.Sprintf("Targets.member.%d.Port", i)), 0),
			AvailabilityZone: r.FormValue(fmt.Sprintf("Targets.member.%d.AvailabilityZone", i)),
		})
	}
	return targets
}

func parseELBv2Actions(r *http.Request) []ELBv2Action {
	return parseELBv2ActionsPrefix(r, "DefaultActions")
}

// parseELBv2ActionsPrefix parses an action list flattened under prefix
// (`DefaultActions` for listeners, `Actions` for rules), including the typed
// fixed-response / redirect configs so they round-trip back to Terraform and
// the SDK.
func parseELBv2ActionsPrefix(r *http.Request, prefix string) []ELBv2Action {
	var actions []ELBv2Action
	for i := 1; ; i++ {
		base := fmt.Sprintf("%s.member.%d", prefix, i)
		actionType := r.FormValue(base + ".Type")
		if actionType == "" {
			break
		}
		action := ELBv2Action{
			Type:           actionType,
			TargetGroupArn: r.FormValue(base + ".TargetGroupArn"),
			Order:          atoiDefault(r.FormValue(base+".Order"), 0),
		}
		if sc := r.FormValue(base + ".FixedResponseConfig.StatusCode"); sc != "" {
			action.FixedResponse = &ELBv2FixedResponseConfig{
				StatusCode:  sc,
				ContentType: r.FormValue(base + ".FixedResponseConfig.ContentType"),
				MessageBody: r.FormValue(base + ".FixedResponseConfig.MessageBody"),
			}
		}
		if r.FormValue(base+".RedirectConfig.StatusCode") != "" {
			action.Redirect = &ELBv2RedirectConfig{
				Protocol:   r.FormValue(base + ".RedirectConfig.Protocol"),
				Port:       r.FormValue(base + ".RedirectConfig.Port"),
				Host:       r.FormValue(base + ".RedirectConfig.Host"),
				Path:       r.FormValue(base + ".RedirectConfig.Path"),
				Query:      r.FormValue(base + ".RedirectConfig.Query"),
				StatusCode: r.FormValue(base + ".RedirectConfig.StatusCode"),
			}
		}
		actions = append(actions, action)
	}
	return actions
}

// elbv2ActionsXML renders an action list under the given wrapper element
// (DefaultActions / Actions), emitting the typed fixed-response / redirect
// configs when present.
func elbv2ActionsXML(wrapper string, actions []ELBv2Action) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<%s>", wrapper)
	for _, action := range actions {
		fmt.Fprintf(&b, "<member><Type>%s</Type>", xmlEscape(action.Type))
		if action.TargetGroupArn != "" {
			fmt.Fprintf(&b, "<TargetGroupArn>%s</TargetGroupArn>", xmlEscape(action.TargetGroupArn))
		}
		if action.Order != 0 {
			fmt.Fprintf(&b, "<Order>%d</Order>", action.Order)
		}
		if fr := action.FixedResponse; fr != nil {
			b.WriteString("<FixedResponseConfig>")
			fmt.Fprintf(&b, "<StatusCode>%s</StatusCode>", xmlEscape(fr.StatusCode))
			if fr.ContentType != "" {
				fmt.Fprintf(&b, "<ContentType>%s</ContentType>", xmlEscape(fr.ContentType))
			}
			if fr.MessageBody != "" {
				fmt.Fprintf(&b, "<MessageBody>%s</MessageBody>", xmlEscape(fr.MessageBody))
			}
			b.WriteString("</FixedResponseConfig>")
		}
		if rd := action.Redirect; rd != nil {
			b.WriteString("<RedirectConfig>")
			fmt.Fprintf(&b, "<StatusCode>%s</StatusCode>", xmlEscape(rd.StatusCode))
			for _, kv := range []struct{ name, val string }{
				{"Protocol", rd.Protocol}, {"Port", rd.Port}, {"Host", rd.Host},
				{"Path", rd.Path}, {"Query", rd.Query},
			} {
				if kv.val != "" {
					fmt.Fprintf(&b, "<%s>%s</%s>", kv.name, xmlEscape(kv.val), kv.name)
				}
			}
			b.WriteString("</RedirectConfig>")
		}
		b.WriteString("</member>")
	}
	fmt.Fprintf(&b, "</%s>", wrapper)
	return b.String()
}

func queryList(r *http.Request, name string) []string {
	var values []string
	for i := 1; ; i++ {
		v := r.FormValue(fmt.Sprintf("%s.member.%d", name, i))
		if v == "" {
			break
		}
		values = append(values, v)
	}
	return values
}

func elbv2StringMembersXML(wrapper string, values []string) string {
	return fmt.Sprintf("<%s>%s</%s>", wrapper, elbv2StringMembersXMLInner(values), wrapper)
}

func elbv2StringMembersXMLInner(values []string) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "<member>%s</member>", xmlEscape(v))
	}
	return b.String()
}

func defaultELBv2LoadBalancerAttributes() map[string]string {
	return map[string]string{
		"deletion_protection.enabled":                     "false",
		"load_balancing.cross_zone.enabled":               "false",
		"access_logs.s3.enabled":                          "false",
		"idle_timeout.timeout_seconds":                    "60",
		"routing.http2.enabled":                           "true",
		"routing.http.drop_invalid_header_fields.enabled": "false",
		"routing.http.preserve_host_header.enabled":       "false",
	}
}

func defaultELBv2TargetGroupAttributes() map[string]string {
	return map[string]string{
		"deregistration_delay.timeout_seconds": "300",
		"stickiness.enabled":                   "false",
		"load_balancing.cross_zone.enabled":    "use_load_balancer_configuration",
	}
}

func defaultELBv2ListenerAttributes(lbType string) map[string]string {
	if lbType == "network" || lbType == "gateway" {
		return map[string]string{
			"tcp.idle_timeout.seconds": "350",
		}
	}
	return map[string]string{
		"routing.http.response.server.enabled": "true",
	}
}

func elbv2ListenerAttributes(listener ELBv2Listener) map[string]string {
	if listener.Attributes != nil {
		return listener.Attributes
	}
	if lb, ok := elbv2LoadBalancers.Get(listener.LoadBalancerArn); ok {
		return defaultELBv2ListenerAttributes(lb.Type)
	}
	return defaultELBv2ListenerAttributes("application")
}

func elbv2VPCFromSubnets(subnets []string) string {
	for _, subnet := range subnets {
		if s, ok := ec2Subnets.Get(subnet); ok {
			return s.VpcId
		}
	}
	return ""
}

func elbv2LoadBalancerKind(lb ELBv2LoadBalancer) string {
	if lb.Type == "network" {
		return "net"
	}
	return "app"
}

func elbv2LoadBalancerID(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) == 0 {
		return generateUUID()[:12]
	}
	return parts[len(parts)-1]
}

func elbv2SetResourceTags(arn string, entries map[string]string, remove bool) {
	if elbv2LoadBalancers.Update(arn, func(lb *ELBv2LoadBalancer) {
		if lb.Tags == nil {
			lb.Tags = map[string]string{}
		}
		for k, v := range entries {
			if remove {
				delete(lb.Tags, k)
			} else {
				lb.Tags[k] = v
			}
		}
	}) {
		return
	}
	if elbv2TargetGroups.Update(arn, func(tg *ELBv2TargetGroup) {
		if tg.Tags == nil {
			tg.Tags = map[string]string{}
		}
		for k, v := range entries {
			if remove {
				delete(tg.Tags, k)
			} else {
				tg.Tags[k] = v
			}
		}
	}) {
		return
	}
}

func elbv2ResourceTags(arn string) map[string]string {
	if lb, ok := elbv2LoadBalancers.Get(arn); ok {
		return lb.Tags
	}
	if tg, ok := elbv2TargetGroups.Get(arn); ok {
		return tg.Tags
	}
	return map[string]string{}
}

func keysToMap(keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = ""
	}
	return out
}

func appendUnique(values []string, incoming string) []string {
	if incoming == "" || containsString(values, incoming) {
		return values
	}
	return append(values, incoming)
}

func containsString(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}

func removeString(values []string, remove string) []string {
	filtered := values[:0]
	for _, v := range values {
		if v != remove {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
