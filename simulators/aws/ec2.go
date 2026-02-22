package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// EC2 types

type EC2Vpc struct {
	VpcId              string
	CidrBlock          string
	State              string
	Tags               []EC2Tag
	OwnerId            string
	IsDefault          bool
	EnableDnsSupport   bool
	EnableDnsHostnames bool
}

type EC2Subnet struct {
	SubnetId            string
	VpcId               string
	CidrBlock           string
	AvailabilityZone    string
	State               string
	Tags                []EC2Tag
	MapPublicIpOnLaunch bool
	OwnerId             string
}

type EC2InternetGateway struct {
	InternetGatewayId string
	Attachments       []EC2IGWAttachment
	Tags              []EC2Tag
	OwnerId           string
}

type EC2IGWAttachment struct {
	VpcId string
	State string
}

type EC2NatGateway struct {
	NatGatewayId        string
	SubnetId            string
	AllocationId        string
	VpcId               string
	State               string
	Tags                []EC2Tag
	NatGatewayAddresses []EC2NatGatewayAddress
	CreateTime          string
}

type EC2NatGatewayAddress struct {
	AllocationId       string
	PublicIp           string
	PrivateIp          string
	NetworkInterfaceId string
}

type EC2ElasticIP struct {
	AllocationId string
	PublicIp     string
	Domain       string
	Tags         []EC2Tag
}

type EC2RouteTable struct {
	RouteTableId string
	VpcId        string
	Routes       []EC2Route
	Tags         []EC2Tag
	OwnerId      string
	Associations []EC2RouteTableAssociation
}

type EC2Route struct {
	DestinationCidrBlock string
	GatewayId            string
	NatGatewayId         string
	State                string
	Origin               string
}

type EC2RouteTableAssociation struct {
	AssociationId string
	RouteTableId  string
	SubnetId      string
	Main          bool
}

type EC2SecurityGroup struct {
	GroupId              string
	GroupName            string
	Description          string
	VpcId                string
	Tags                 []EC2Tag
	OwnerId              string
	IpPermissions        []EC2IpPermission
	IpPermissionsEgress  []EC2IpPermission
}

type EC2IpPermission struct {
	IpProtocol       string
	FromPort         int
	ToPort           int
	IpRanges         []EC2IpRange
	UserIdGroupPairs []EC2UserIdGroupPair
}

type EC2IpRange struct {
	CidrIp      string
	Description string
}

type EC2UserIdGroupPair struct {
	GroupId     string
	Description string
}

type EC2SecurityGroupRule struct {
	RuleId      string
	GroupId     string
	GroupOwner  string
	IsEgress   bool
	IpProtocol string
	FromPort   int
	ToPort     int
	CidrIpv4   string
	RefGroupId string
	Description string
}

type EC2Tag struct {
	Key   string
	Value string
}

// State stores
var (
	ec2Vpcs             *sim.StateStore[EC2Vpc]
	ec2Subnets          *sim.StateStore[EC2Subnet]
	ec2InternetGateways *sim.StateStore[EC2InternetGateway]
	ec2NatGateways      *sim.StateStore[EC2NatGateway]
	ec2ElasticIPs       *sim.StateStore[EC2ElasticIP]
	ec2RouteTables      *sim.StateStore[EC2RouteTable]
	ec2SecurityGroups     *sim.StateStore[EC2SecurityGroup]
	ec2SecurityGroupRules *sim.StateStore[EC2SecurityGroupRule]
)

const ec2Owner = "123456789012"

func registerEC2(r *sim.AWSQueryRouter) {
	ec2Vpcs = sim.NewStateStore[EC2Vpc]()
	ec2Subnets = sim.NewStateStore[EC2Subnet]()
	ec2InternetGateways = sim.NewStateStore[EC2InternetGateway]()
	ec2NatGateways = sim.NewStateStore[EC2NatGateway]()
	ec2ElasticIPs = sim.NewStateStore[EC2ElasticIP]()
	ec2RouteTables = sim.NewStateStore[EC2RouteTable]()
	ec2SecurityGroups = sim.NewStateStore[EC2SecurityGroup]()
	ec2SecurityGroupRules = sim.NewStateStore[EC2SecurityGroupRule]()

	// VPC
	r.Register("CreateVpc", handleCreateVpc)
	r.Register("DescribeVpcs", handleDescribeVpcs)
	r.Register("DeleteVpc", handleDeleteVpc)
	r.Register("DescribeVpcAttribute", handleDescribeVpcAttribute)
	r.Register("ModifyVpcAttribute", handleModifyVpcAttribute)

	// Subnet
	r.Register("CreateSubnet", handleCreateSubnet)
	r.Register("DescribeSubnets", handleDescribeSubnets)
	r.Register("DeleteSubnet", handleDeleteSubnet)
	r.Register("ModifySubnetAttribute", handleModifySubnetAttribute)

	// Internet Gateway
	r.Register("CreateInternetGateway", handleCreateInternetGateway)
	r.Register("AttachInternetGateway", handleAttachInternetGateway)
	r.Register("DetachInternetGateway", handleDetachInternetGateway)
	r.Register("DescribeInternetGateways", handleDescribeInternetGateways)
	r.Register("DeleteInternetGateway", handleDeleteInternetGateway)

	// Elastic IP
	r.Register("AllocateAddress", handleAllocateAddress)
	r.Register("DescribeAddresses", handleDescribeAddresses)
	r.Register("DescribeAddressesAttribute", handleDescribeAddressesAttribute)
	r.Register("ReleaseAddress", handleReleaseAddress)

	// NAT Gateway
	r.Register("CreateNatGateway", handleCreateNatGateway)
	r.Register("DescribeNatGateways", handleDescribeNatGateways)
	r.Register("DeleteNatGateway", handleDeleteNatGateway)

	// Route Table
	r.Register("CreateRouteTable", handleCreateRouteTable)
	r.Register("DescribeRouteTables", handleDescribeRouteTables)
	r.Register("DeleteRouteTable", handleDeleteRouteTable)
	r.Register("CreateRoute", handleCreateRoute)
	r.Register("DeleteRoute", handleDeleteRoute)
	r.Register("AssociateRouteTable", handleAssociateRouteTable)
	r.Register("DisassociateRouteTable", handleDisassociateRouteTable)

	// Security Group
	r.Register("CreateSecurityGroup", handleCreateSecurityGroup)
	r.Register("DescribeSecurityGroups", handleDescribeSecurityGroups)
	r.Register("DescribeSecurityGroupRules", handleDescribeSecurityGroupRules)
	r.Register("DeleteSecurityGroup", handleDeleteSecurityGroup)
	r.Register("AuthorizeSecurityGroupIngress", handleAuthorizeSecurityGroupIngress)
	r.Register("AuthorizeSecurityGroupEgress", handleAuthorizeSecurityGroupEgress)
	r.Register("RevokeSecurityGroupIngress", handleRevokeSecurityGroupIngress)
	r.Register("RevokeSecurityGroupEgress", handleRevokeSecurityGroupEgress)

	// Network Interfaces (used during destroy to check ENIs before deleting SGs/subnets)
	r.Register("DescribeNetworkInterfaces", handleDescribeNetworkInterfaces)
}

// Tag helpers

func parseTags(r *http.Request) []EC2Tag {
	var tags []EC2Tag
	for i := 1; ; i++ {
		key := r.FormValue(fmt.Sprintf("TagSpecification.1.Tag.%d.Key", i))
		if key == "" {
			break
		}
		value := r.FormValue(fmt.Sprintf("TagSpecification.1.Tag.%d.Value", i))
		tags = append(tags, EC2Tag{Key: key, Value: value})
	}
	return tags
}

func writeTagSetXML(tags []EC2Tag) string {
	if len(tags) == 0 {
		return "<tagSet/>"
	}
	var b strings.Builder
	b.WriteString("<tagSet>")
	for _, t := range tags {
		fmt.Fprintf(&b, "<item><key>%s</key><value>%s</value></item>", t.Key, t.Value)
	}
	b.WriteString("</tagSet>")
	return b.String()
}

func ec2ID(prefix string) string {
	return prefix + "-" + generateUUID()[:8]
}

func ec2Xmlns() string {
	return `xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"`
}

// ---- VPC ----

func handleCreateVpc(w http.ResponseWriter, r *http.Request) {
	cidr := r.FormValue("CidrBlock")
	tags := parseTags(r)
	id := ec2ID("vpc")

	vpc := EC2Vpc{
		VpcId:              id,
		CidrBlock:          cidr,
		State:              "available",
		Tags:               tags,
		OwnerId:            ec2Owner,
		IsDefault:          false,
		EnableDnsSupport:   true,
		EnableDnsHostnames: false,
	}
	ec2Vpcs.Put(id, vpc)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateVpcResponse %s>
  <requestId>%s</requestId>
  <vpc>
    <vpcId>%s</vpcId><cidrBlock>%s</cidrBlock><state>available</state>
    <ownerId>%s</ownerId><isDefault>false</isDefault>
    %s
  </vpc>
</CreateVpcResponse>`, ec2Xmlns(), generateUUID(), id, cidr, ec2Owner, writeTagSetXML(tags))
}

func vpcItemXML(vpc EC2Vpc) string {
	return fmt.Sprintf(`<item>
    <vpcId>%s</vpcId><cidrBlock>%s</cidrBlock><state>%s</state>
    <ownerId>%s</ownerId><isDefault>%t</isDefault>
    %s
  </item>`, vpc.VpcId, vpc.CidrBlock, vpc.State, vpc.OwnerId, vpc.IsDefault, writeTagSetXML(vpc.Tags))
}

func handleDescribeVpcs(w http.ResponseWriter, r *http.Request) {
	var vpcs []EC2Vpc
	if id := r.FormValue("VpcId.1"); id != "" {
		if v, ok := ec2Vpcs.Get(id); ok {
			vpcs = append(vpcs, v)
		}
	} else {
		vpcs = ec2Vpcs.List()
	}

	var items strings.Builder
	for _, v := range vpcs {
		items.WriteString(vpcItemXML(v))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeVpcsResponse %s>
  <requestId>%s</requestId>
  <vpcSet>%s</vpcSet>
</DescribeVpcsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDeleteVpc(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("VpcId")
	ec2Vpcs.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteVpcResponse %s>
  <requestId>%s</requestId><return>true</return>
</DeleteVpcResponse>`, ec2Xmlns(), generateUUID())
}

func handleDescribeVpcAttribute(w http.ResponseWriter, r *http.Request) {
	vpcId := r.FormValue("VpcId")
	attr := r.FormValue("Attribute")
	vpc, _ := ec2Vpcs.Get(vpcId)

	w.Header().Set("Content-Type", "text/xml")
	switch attr {
	case "enableDnsSupport":
		fmt.Fprintf(w, `<DescribeVpcAttributeResponse %s>
  <requestId>%s</requestId><vpcId>%s</vpcId>
  <enableDnsSupport><value>%t</value></enableDnsSupport>
</DescribeVpcAttributeResponse>`, ec2Xmlns(), generateUUID(), vpcId, vpc.EnableDnsSupport)
	case "enableDnsHostnames":
		fmt.Fprintf(w, `<DescribeVpcAttributeResponse %s>
  <requestId>%s</requestId><vpcId>%s</vpcId>
  <enableDnsHostnames><value>%t</value></enableDnsHostnames>
</DescribeVpcAttributeResponse>`, ec2Xmlns(), generateUUID(), vpcId, vpc.EnableDnsHostnames)
	case "enableNetworkAddressUsageMetrics":
		fmt.Fprintf(w, `<DescribeVpcAttributeResponse %s>
  <requestId>%s</requestId><vpcId>%s</vpcId>
  <enableNetworkAddressUsageMetrics><value>false</value></enableNetworkAddressUsageMetrics>
</DescribeVpcAttributeResponse>`, ec2Xmlns(), generateUUID(), vpcId)
	default:
		fmt.Fprintf(w, `<DescribeVpcAttributeResponse %s>
  <requestId>%s</requestId><vpcId>%s</vpcId>
</DescribeVpcAttributeResponse>`, ec2Xmlns(), generateUUID(), vpcId)
	}
}

func handleModifyVpcAttribute(w http.ResponseWriter, r *http.Request) {
	vpcId := r.FormValue("VpcId")
	ec2Vpcs.Update(vpcId, func(v *EC2Vpc) {
		if val := r.FormValue("EnableDnsSupport.Value"); val != "" {
			v.EnableDnsSupport = val == "true"
		}
		if val := r.FormValue("EnableDnsHostnames.Value"); val != "" {
			v.EnableDnsHostnames = val == "true"
		}
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifyVpcAttributeResponse %s>
  <requestId>%s</requestId><return>true</return>
</ModifyVpcAttributeResponse>`, ec2Xmlns(), generateUUID())
}

// ---- Subnet ----

func handleCreateSubnet(w http.ResponseWriter, r *http.Request) {
	vpcId := r.FormValue("VpcId")
	cidr := r.FormValue("CidrBlock")
	az := r.FormValue("AvailabilityZone")
	tags := parseTags(r)
	id := ec2ID("subnet")

	subnet := EC2Subnet{
		SubnetId:         id,
		VpcId:            vpcId,
		CidrBlock:        cidr,
		AvailabilityZone: az,
		State:            "available",
		Tags:             tags,
		OwnerId:          ec2Owner,
	}
	ec2Subnets.Put(id, subnet)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateSubnetResponse %s>
  <requestId>%s</requestId>
  <subnet>
    <subnetId>%s</subnetId><vpcId>%s</vpcId><cidrBlock>%s</cidrBlock>
    <availabilityZone>%s</availabilityZone><state>available</state>
    <mapPublicIpOnLaunch>false</mapPublicIpOnLaunch><ownerId>%s</ownerId>
    %s
  </subnet>
</CreateSubnetResponse>`, ec2Xmlns(), generateUUID(), id, vpcId, cidr, az, ec2Owner, writeTagSetXML(tags))
}

func subnetItemXML(s EC2Subnet) string {
	return fmt.Sprintf(`<item>
    <subnetId>%s</subnetId><vpcId>%s</vpcId><cidrBlock>%s</cidrBlock>
    <availabilityZone>%s</availabilityZone><state>%s</state>
    <mapPublicIpOnLaunch>%t</mapPublicIpOnLaunch><ownerId>%s</ownerId>
    %s
  </item>`, s.SubnetId, s.VpcId, s.CidrBlock, s.AvailabilityZone, s.State, s.MapPublicIpOnLaunch, s.OwnerId, writeTagSetXML(s.Tags))
}

func handleDescribeSubnets(w http.ResponseWriter, r *http.Request) {
	var subnets []EC2Subnet
	if id := r.FormValue("SubnetId.1"); id != "" {
		if s, ok := ec2Subnets.Get(id); ok {
			subnets = append(subnets, s)
		}
	} else if vpcFilter := r.FormValue("Filter.1.Value.1"); r.FormValue("Filter.1.Name") == "vpc-id" && vpcFilter != "" {
		subnets = ec2Subnets.Filter(func(s EC2Subnet) bool {
			return s.VpcId == vpcFilter
		})
	} else {
		subnets = ec2Subnets.List()
	}

	var items strings.Builder
	for _, s := range subnets {
		items.WriteString(subnetItemXML(s))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeSubnetsResponse %s>
  <requestId>%s</requestId>
  <subnetSet>%s</subnetSet>
</DescribeSubnetsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleModifySubnetAttribute(w http.ResponseWriter, r *http.Request) {
	subnetId := r.FormValue("SubnetId")
	ec2Subnets.Update(subnetId, func(s *EC2Subnet) {
		if val := r.FormValue("MapPublicIpOnLaunch.Value"); val != "" {
			s.MapPublicIpOnLaunch = val == "true"
		}
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifySubnetAttributeResponse %s>
  <requestId>%s</requestId><return>true</return>
</ModifySubnetAttributeResponse>`, ec2Xmlns(), generateUUID())
}

func handleDeleteSubnet(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("SubnetId")
	ec2Subnets.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteSubnetResponse %s>
  <requestId>%s</requestId><return>true</return>
</DeleteSubnetResponse>`, ec2Xmlns(), generateUUID())
}

// ---- Internet Gateway ----

func handleCreateInternetGateway(w http.ResponseWriter, r *http.Request) {
	tags := parseTags(r)
	id := ec2ID("igw")

	igw := EC2InternetGateway{
		InternetGatewayId: id,
		Tags:              tags,
		OwnerId:           ec2Owner,
	}
	ec2InternetGateways.Put(id, igw)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateInternetGatewayResponse %s>
  <requestId>%s</requestId>
  <internetGateway>
    <internetGatewayId>%s</internetGatewayId>
    <attachmentSet/>
    <ownerId>%s</ownerId>
    %s
  </internetGateway>
</CreateInternetGatewayResponse>`, ec2Xmlns(), generateUUID(), id, ec2Owner, writeTagSetXML(tags))
}

func handleAttachInternetGateway(w http.ResponseWriter, r *http.Request) {
	igwId := r.FormValue("InternetGatewayId")
	vpcId := r.FormValue("VpcId")

	ec2InternetGateways.Update(igwId, func(igw *EC2InternetGateway) {
		igw.Attachments = append(igw.Attachments, EC2IGWAttachment{VpcId: vpcId, State: "available"})
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AttachInternetGatewayResponse %s>
  <requestId>%s</requestId><return>true</return>
</AttachInternetGatewayResponse>`, ec2Xmlns(), generateUUID())
}

func handleDetachInternetGateway(w http.ResponseWriter, r *http.Request) {
	igwId := r.FormValue("InternetGatewayId")
	vpcId := r.FormValue("VpcId")

	ec2InternetGateways.Update(igwId, func(igw *EC2InternetGateway) {
		var filtered []EC2IGWAttachment
		for _, a := range igw.Attachments {
			if a.VpcId != vpcId {
				filtered = append(filtered, a)
			}
		}
		igw.Attachments = filtered
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DetachInternetGatewayResponse %s>
  <requestId>%s</requestId><return>true</return>
</DetachInternetGatewayResponse>`, ec2Xmlns(), generateUUID())
}

func igwItemXML(igw EC2InternetGateway) string {
	var attachments strings.Builder
	if len(igw.Attachments) == 0 {
		attachments.WriteString("<attachmentSet/>")
	} else {
		attachments.WriteString("<attachmentSet>")
		for _, a := range igw.Attachments {
			fmt.Fprintf(&attachments, "<item><vpcId>%s</vpcId><state>%s</state></item>", a.VpcId, a.State)
		}
		attachments.WriteString("</attachmentSet>")
	}
	return fmt.Sprintf(`<item>
    <internetGatewayId>%s</internetGatewayId>
    %s<ownerId>%s</ownerId>
    %s
  </item>`, igw.InternetGatewayId, attachments.String(), igw.OwnerId, writeTagSetXML(igw.Tags))
}

func handleDescribeInternetGateways(w http.ResponseWriter, r *http.Request) {
	var igws []EC2InternetGateway
	if id := r.FormValue("InternetGatewayId.1"); id != "" {
		if g, ok := ec2InternetGateways.Get(id); ok {
			igws = append(igws, g)
		}
	} else {
		igws = ec2InternetGateways.List()
	}

	var items strings.Builder
	for _, g := range igws {
		items.WriteString(igwItemXML(g))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeInternetGatewaysResponse %s>
  <requestId>%s</requestId>
  <internetGatewaySet>%s</internetGatewaySet>
</DescribeInternetGatewaysResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDeleteInternetGateway(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("InternetGatewayId")
	ec2InternetGateways.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteInternetGatewayResponse %s>
  <requestId>%s</requestId><return>true</return>
</DeleteInternetGatewayResponse>`, ec2Xmlns(), generateUUID())
}

// ---- Elastic IP ----

func handleAllocateAddress(w http.ResponseWriter, r *http.Request) {
	domain := r.FormValue("Domain")
	if domain == "" {
		domain = "vpc"
	}
	tags := parseTags(r)
	id := ec2ID("eipalloc")
	ip := fmt.Sprintf("203.0.113.%d", ec2ElasticIPs.Len()+1)

	eip := EC2ElasticIP{
		AllocationId: id,
		PublicIp:     ip,
		Domain:       domain,
		Tags:         tags,
	}
	ec2ElasticIPs.Put(id, eip)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AllocateAddressResponse %s>
  <requestId>%s</requestId>
  <allocationId>%s</allocationId><publicIp>%s</publicIp><domain>%s</domain>
</AllocateAddressResponse>`, ec2Xmlns(), generateUUID(), id, ip, domain)
}

func handleDescribeAddresses(w http.ResponseWriter, r *http.Request) {
	var eips []EC2ElasticIP
	if id := r.FormValue("AllocationId.1"); id != "" {
		if e, ok := ec2ElasticIPs.Get(id); ok {
			eips = append(eips, e)
		}
	} else {
		eips = ec2ElasticIPs.List()
	}

	var items strings.Builder
	for _, e := range eips {
		fmt.Fprintf(&items, `<item><allocationId>%s</allocationId><publicIp>%s</publicIp><domain>%s</domain>%s</item>`,
			e.AllocationId, e.PublicIp, e.Domain, writeTagSetXML(e.Tags))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeAddressesResponse %s>
  <requestId>%s</requestId>
  <addressesSet>%s</addressesSet>
</DescribeAddressesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleReleaseAddress(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("AllocationId")
	ec2ElasticIPs.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ReleaseAddressResponse %s>
  <requestId>%s</requestId><return>true</return>
</ReleaseAddressResponse>`, ec2Xmlns(), generateUUID())
}

func handleDescribeAddressesAttribute(w http.ResponseWriter, r *http.Request) {
	allocId := r.FormValue("AllocationId.1")
	attr := r.FormValue("Attribute")
	if attr == "" {
		attr = "domain-name"
	}

	w.Header().Set("Content-Type", "text/xml")
	if allocId != "" {
		fmt.Fprintf(w, `<DescribeAddressesAttributeResponse %s>
  <requestId>%s</requestId>
  <addressSet>
    <item>
      <allocationId>%s</allocationId>
    </item>
  </addressSet>
</DescribeAddressesAttributeResponse>`, ec2Xmlns(), generateUUID(), allocId)
	} else {
		fmt.Fprintf(w, `<DescribeAddressesAttributeResponse %s>
  <requestId>%s</requestId>
  <addressSet/>
</DescribeAddressesAttributeResponse>`, ec2Xmlns(), generateUUID())
	}
}

// ---- NAT Gateway ----

func handleCreateNatGateway(w http.ResponseWriter, r *http.Request) {
	subnetId := r.FormValue("SubnetId")
	allocId := r.FormValue("AllocationId")
	tags := parseTags(r)
	id := ec2ID("nat")

	vpcId := ""
	if s, ok := ec2Subnets.Get(subnetId); ok {
		vpcId = s.VpcId
	}
	publicIp := ""
	if e, ok := ec2ElasticIPs.Get(allocId); ok {
		publicIp = e.PublicIp
	}

	natgw := EC2NatGateway{
		NatGatewayId: id,
		SubnetId:     subnetId,
		AllocationId: allocId,
		VpcId:        vpcId,
		State:        "available",
		Tags:         tags,
		NatGatewayAddresses: []EC2NatGatewayAddress{{
			AllocationId:       allocId,
			PublicIp:           publicIp,
			PrivateIp:          "10.0.0.10",
			NetworkInterfaceId: ec2ID("eni"),
		}},
		CreateTime: time.Now().UTC().Format(time.RFC3339),
	}
	ec2NatGateways.Put(id, natgw)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateNatGatewayResponse %s>
  <requestId>%s</requestId>
  <natGateway>
    <natGatewayId>%s</natGatewayId><subnetId>%s</subnetId>
    <vpcId>%s</vpcId><state>available</state>
    <natGatewayAddressSet>
      <item><allocationId>%s</allocationId><publicIp>%s</publicIp><privateIp>10.0.0.10</privateIp></item>
    </natGatewayAddressSet>
    <createTime>%s</createTime>
    %s
  </natGateway>
</CreateNatGatewayResponse>`, ec2Xmlns(), generateUUID(), id, subnetId, vpcId, allocId, publicIp, natgw.CreateTime, writeTagSetXML(tags))
}

func natgwItemXML(n EC2NatGateway) string {
	var addrs strings.Builder
	addrs.WriteString("<natGatewayAddressSet>")
	for _, a := range n.NatGatewayAddresses {
		fmt.Fprintf(&addrs, "<item><allocationId>%s</allocationId><publicIp>%s</publicIp><privateIp>%s</privateIp></item>",
			a.AllocationId, a.PublicIp, a.PrivateIp)
	}
	addrs.WriteString("</natGatewayAddressSet>")
	return fmt.Sprintf(`<item>
    <natGatewayId>%s</natGatewayId><subnetId>%s</subnetId><vpcId>%s</vpcId>
    <state>%s</state>%s<createTime>%s</createTime>
    %s
  </item>`, n.NatGatewayId, n.SubnetId, n.VpcId, n.State, addrs.String(), n.CreateTime, writeTagSetXML(n.Tags))
}

func handleDescribeNatGateways(w http.ResponseWriter, r *http.Request) {
	var nats []EC2NatGateway
	if id := r.FormValue("NatGatewayId.1"); id != "" {
		if n, ok := ec2NatGateways.Get(id); ok {
			nats = append(nats, n)
		}
	} else if r.FormValue("Filter.1.Name") == "vpc-id" {
		vpcId := r.FormValue("Filter.1.Value.1")
		nats = ec2NatGateways.Filter(func(n EC2NatGateway) bool {
			return n.VpcId == vpcId
		})
	} else {
		nats = ec2NatGateways.List()
	}

	var items strings.Builder
	for _, n := range nats {
		items.WriteString(natgwItemXML(n))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeNatGatewaysResponse %s>
  <requestId>%s</requestId>
  <natGatewaySet>%s</natGatewaySet>
</DescribeNatGatewaysResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDeleteNatGateway(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("NatGatewayId")
	ec2NatGateways.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteNatGatewayResponse %s>
  <requestId>%s</requestId>
  <natGatewayId>%s</natGatewayId>
</DeleteNatGatewayResponse>`, ec2Xmlns(), generateUUID(), id)
}

// ---- Route Table ----

func handleCreateRouteTable(w http.ResponseWriter, r *http.Request) {
	vpcId := r.FormValue("VpcId")
	tags := parseTags(r)
	id := ec2ID("rtb")

	// Look up VPC CIDR for local route
	localCidr := "10.0.0.0/16"
	if v, ok := ec2Vpcs.Get(vpcId); ok {
		localCidr = v.CidrBlock
	}

	rt := EC2RouteTable{
		RouteTableId: id,
		VpcId:        vpcId,
		Routes: []EC2Route{{
			DestinationCidrBlock: localCidr,
			GatewayId:            "local",
			State:                "active",
			Origin:               "CreateRouteTable",
		}},
		Tags:    tags,
		OwnerId: ec2Owner,
	}
	ec2RouteTables.Put(id, rt)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateRouteTableResponse %s>
  <requestId>%s</requestId>
  <routeTable>
    <routeTableId>%s</routeTableId><vpcId>%s</vpcId>
    %s
    <associationSet/>
    %s
  </routeTable>
</CreateRouteTableResponse>`, ec2Xmlns(), generateUUID(), id, vpcId, routeSetXML(rt.Routes), writeTagSetXML(tags))
}

func routeSetXML(routes []EC2Route) string {
	var b strings.Builder
	b.WriteString("<routeSet>")
	for _, route := range routes {
		b.WriteString("<item>")
		fmt.Fprintf(&b, "<destinationCidrBlock>%s</destinationCidrBlock>", route.DestinationCidrBlock)
		if route.GatewayId != "" {
			fmt.Fprintf(&b, "<gatewayId>%s</gatewayId>", route.GatewayId)
		}
		if route.NatGatewayId != "" {
			fmt.Fprintf(&b, "<natGatewayId>%s</natGatewayId>", route.NatGatewayId)
		}
		fmt.Fprintf(&b, "<state>%s</state><origin>%s</origin>", route.State, route.Origin)
		b.WriteString("</item>")
	}
	b.WriteString("</routeSet>")
	return b.String()
}

func assocSetXML(rtId string, assocs []EC2RouteTableAssociation) string {
	var filtered []EC2RouteTableAssociation
	for _, a := range assocs {
		if a.RouteTableId == rtId {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		return "<associationSet/>"
	}
	var b strings.Builder
	b.WriteString("<associationSet>")
	for _, a := range filtered {
		fmt.Fprintf(&b, `<item><routeTableAssociationId>%s</routeTableAssociationId><routeTableId>%s</routeTableId><subnetId>%s</subnetId><main>%t</main></item>`,
			a.AssociationId, a.RouteTableId, a.SubnetId, a.Main)
	}
	b.WriteString("</associationSet>")
	return b.String()
}

func rtItemXML(rt EC2RouteTable) string {
	return fmt.Sprintf(`<item>
    <routeTableId>%s</routeTableId><vpcId>%s</vpcId>
    %s
    %s
    <ownerId>%s</ownerId>
    %s
  </item>`, rt.RouteTableId, rt.VpcId, routeSetXML(rt.Routes), assocSetXML(rt.RouteTableId, rt.Associations), rt.OwnerId, writeTagSetXML(rt.Tags))
}

func handleDescribeRouteTables(w http.ResponseWriter, r *http.Request) {
	var rts []EC2RouteTable
	if id := r.FormValue("RouteTableId.1"); id != "" {
		if rt, ok := ec2RouteTables.Get(id); ok {
			rts = append(rts, rt)
		}
	} else if r.FormValue("Filter.1.Name") == "association.route-table-association-id" {
		assocId := r.FormValue("Filter.1.Value.1")
		for _, rt := range ec2RouteTables.List() {
			for _, a := range rt.Associations {
				if a.AssociationId == assocId {
					rts = append(rts, rt)
					break
				}
			}
		}
	} else {
		rts = ec2RouteTables.List()
	}

	var items strings.Builder
	for _, rt := range rts {
		items.WriteString(rtItemXML(rt))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeRouteTablesResponse %s>
  <requestId>%s</requestId>
  <routeTableSet>%s</routeTableSet>
</DescribeRouteTablesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDeleteRouteTable(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("RouteTableId")
	ec2RouteTables.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteRouteTableResponse %s>
  <requestId>%s</requestId><return>true</return>
</DeleteRouteTableResponse>`, ec2Xmlns(), generateUUID())
}

func handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	rtId := r.FormValue("RouteTableId")
	destCidr := r.FormValue("DestinationCidrBlock")
	gwId := r.FormValue("GatewayId")
	natId := r.FormValue("NatGatewayId")

	ec2RouteTables.Update(rtId, func(rt *EC2RouteTable) {
		rt.Routes = append(rt.Routes, EC2Route{
			DestinationCidrBlock: destCidr,
			GatewayId:            gwId,
			NatGatewayId:         natId,
			State:                "active",
			Origin:               "CreateRoute",
		})
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateRouteResponse %s>
  <requestId>%s</requestId><return>true</return>
</CreateRouteResponse>`, ec2Xmlns(), generateUUID())
}

func handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	rtId := r.FormValue("RouteTableId")
	destCidr := r.FormValue("DestinationCidrBlock")

	ec2RouteTables.Update(rtId, func(rt *EC2RouteTable) {
		var filtered []EC2Route
		for _, route := range rt.Routes {
			if route.DestinationCidrBlock != destCidr {
				filtered = append(filtered, route)
			}
		}
		rt.Routes = filtered
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteRouteResponse %s>
  <requestId>%s</requestId><return>true</return>
</DeleteRouteResponse>`, ec2Xmlns(), generateUUID())
}

func handleAssociateRouteTable(w http.ResponseWriter, r *http.Request) {
	rtId := r.FormValue("RouteTableId")
	subnetId := r.FormValue("SubnetId")
	assocId := ec2ID("rtbassoc")

	ec2RouteTables.Update(rtId, func(rt *EC2RouteTable) {
		rt.Associations = append(rt.Associations, EC2RouteTableAssociation{
			AssociationId: assocId,
			RouteTableId:  rtId,
			SubnetId:      subnetId,
			Main:          false,
		})
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AssociateRouteTableResponse %s>
  <requestId>%s</requestId>
  <associationId>%s</associationId>
</AssociateRouteTableResponse>`, ec2Xmlns(), generateUUID(), assocId)
}

func handleDisassociateRouteTable(w http.ResponseWriter, r *http.Request) {
	assocId := r.FormValue("AssociationId")

	// Find and remove association from its route table
	for _, rt := range ec2RouteTables.List() {
		for _, a := range rt.Associations {
			if a.AssociationId == assocId {
				ec2RouteTables.Update(rt.RouteTableId, func(rt *EC2RouteTable) {
					var filtered []EC2RouteTableAssociation
					for _, a := range rt.Associations {
						if a.AssociationId != assocId {
							filtered = append(filtered, a)
						}
					}
					rt.Associations = filtered
				})
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DisassociateRouteTableResponse %s>
  <requestId>%s</requestId><return>true</return>
</DisassociateRouteTableResponse>`, ec2Xmlns(), generateUUID())
}

// ---- Security Group ----

func handleCreateSecurityGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("GroupName")
	desc := r.FormValue("GroupDescription")
	vpcId := r.FormValue("VpcId")
	tags := parseTags(r)
	id := ec2ID("sg")

	sg := EC2SecurityGroup{
		GroupId:     id,
		GroupName:   name,
		Description: desc,
		VpcId:       vpcId,
		Tags:        tags,
		OwnerId:     ec2Owner,
	}
	ec2SecurityGroups.Put(id, sg)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateSecurityGroupResponse %s>
  <requestId>%s</requestId>
  <groupId>%s</groupId>
  <return>true</return>
</CreateSecurityGroupResponse>`, ec2Xmlns(), generateUUID(), id)
}

func sgItemXML(sg EC2SecurityGroup) string {
	return fmt.Sprintf(`<item>
    <groupId>%s</groupId><groupName>%s</groupName><groupDescription>%s</groupDescription>
    <vpcId>%s</vpcId><ownerId>%s</ownerId>
    %s%s
    %s
  </item>`, sg.GroupId, sg.GroupName, sg.Description, sg.VpcId, sg.OwnerId,
		ipPermsXML("ipPermissions", sg.IpPermissions),
		ipPermsXML("ipPermissionsEgress", sg.IpPermissionsEgress),
		writeTagSetXML(sg.Tags))
}

func ipPermsXML(element string, perms []EC2IpPermission) string {
	if len(perms) == 0 {
		return fmt.Sprintf("<%s/>", element)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<%s>", element)
	for _, p := range perms {
		b.WriteString("<item>")
		fmt.Fprintf(&b, "<ipProtocol>%s</ipProtocol><fromPort>%d</fromPort><toPort>%d</toPort>", p.IpProtocol, p.FromPort, p.ToPort)
		if len(p.IpRanges) > 0 {
			b.WriteString("<ipRanges>")
			for _, r := range p.IpRanges {
				fmt.Fprintf(&b, "<item><cidrIp>%s</cidrIp>", r.CidrIp)
				if r.Description != "" {
					fmt.Fprintf(&b, "<description>%s</description>", r.Description)
				}
				b.WriteString("</item>")
			}
			b.WriteString("</ipRanges>")
		} else {
			b.WriteString("<ipRanges/>")
		}
		if len(p.UserIdGroupPairs) > 0 {
			b.WriteString("<groups>")
			for _, g := range p.UserIdGroupPairs {
				fmt.Fprintf(&b, "<item><groupId>%s</groupId>", g.GroupId)
				if g.Description != "" {
					fmt.Fprintf(&b, "<description>%s</description>", g.Description)
				}
				b.WriteString("</item>")
			}
			b.WriteString("</groups>")
		} else {
			b.WriteString("<groups/>")
		}
		b.WriteString("</item>")
	}
	fmt.Fprintf(&b, "</%s>", element)
	return b.String()
}

func handleDescribeSecurityGroups(w http.ResponseWriter, r *http.Request) {
	var sgs []EC2SecurityGroup
	if id := r.FormValue("GroupId.1"); id != "" {
		if sg, ok := ec2SecurityGroups.Get(id); ok {
			sgs = append(sgs, sg)
		}
	} else {
		sgs = ec2SecurityGroups.List()
	}

	var items strings.Builder
	for _, sg := range sgs {
		items.WriteString(sgItemXML(sg))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeSecurityGroupsResponse %s>
  <requestId>%s</requestId>
  <securityGroupInfo>%s</securityGroupInfo>
</DescribeSecurityGroupsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDeleteSecurityGroup(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("GroupId")
	ec2SecurityGroups.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteSecurityGroupResponse %s>
  <requestId>%s</requestId><return>true</return>
</DeleteSecurityGroupResponse>`, ec2Xmlns(), generateUUID())
}

func parseIpPermission(r *http.Request, prefix string) EC2IpPermission {
	perm := EC2IpPermission{
		IpProtocol: r.FormValue(prefix + ".IpProtocol"),
	}
	if v := r.FormValue(prefix + ".FromPort"); v != "" {
		fmt.Sscanf(v, "%d", &perm.FromPort)
	}
	if v := r.FormValue(prefix + ".ToPort"); v != "" {
		fmt.Sscanf(v, "%d", &perm.ToPort)
	}

	for i := 1; ; i++ {
		cidr := r.FormValue(fmt.Sprintf("%s.IpRanges.%d.CidrIp", prefix, i))
		if cidr == "" {
			break
		}
		desc := r.FormValue(fmt.Sprintf("%s.IpRanges.%d.Description", prefix, i))
		perm.IpRanges = append(perm.IpRanges, EC2IpRange{CidrIp: cidr, Description: desc})
	}

	// Try both "UserIdGroupPairs" (classic) and "Groups" (SDK v2) field names
	for i := 1; ; i++ {
		gid := r.FormValue(fmt.Sprintf("%s.UserIdGroupPairs.%d.GroupId", prefix, i))
		if gid == "" {
			gid = r.FormValue(fmt.Sprintf("%s.Groups.%d.GroupId", prefix, i))
		}
		if gid == "" {
			break
		}
		desc := r.FormValue(fmt.Sprintf("%s.UserIdGroupPairs.%d.Description", prefix, i))
		if desc == "" {
			desc = r.FormValue(fmt.Sprintf("%s.Groups.%d.Description", prefix, i))
		}
		perm.UserIdGroupPairs = append(perm.UserIdGroupPairs, EC2UserIdGroupPair{GroupId: gid, Description: desc})
	}
	return perm
}

func sgrItemXML(rule EC2SecurityGroupRule) string {
	var b strings.Builder
	b.WriteString("<item>")
	fmt.Fprintf(&b, "<securityGroupRuleId>%s</securityGroupRuleId>", rule.RuleId)
	fmt.Fprintf(&b, "<groupId>%s</groupId>", rule.GroupId)
	fmt.Fprintf(&b, "<groupOwnerId>%s</groupOwnerId>", rule.GroupOwner)
	fmt.Fprintf(&b, "<isEgress>%t</isEgress>", rule.IsEgress)
	fmt.Fprintf(&b, "<ipProtocol>%s</ipProtocol>", rule.IpProtocol)
	fmt.Fprintf(&b, "<fromPort>%d</fromPort>", rule.FromPort)
	fmt.Fprintf(&b, "<toPort>%d</toPort>", rule.ToPort)
	if rule.CidrIpv4 != "" {
		fmt.Fprintf(&b, "<cidrIpv4>%s</cidrIpv4>", rule.CidrIpv4)
	}
	if rule.RefGroupId != "" {
		fmt.Fprintf(&b, "<referencedGroupInfo><groupId>%s</groupId><userId>%s</userId></referencedGroupInfo>", rule.RefGroupId, rule.GroupOwner)
	}
	if rule.Description != "" {
		fmt.Fprintf(&b, "<description>%s</description>", rule.Description)
	}
	fmt.Fprintf(&b, "<tags/>")
	b.WriteString("</item>")
	return b.String()
}

func createSecurityGroupRules(groupId string, perm EC2IpPermission, isEgress bool) []EC2SecurityGroupRule {
	sg, _ := ec2SecurityGroups.Get(groupId)
	var rules []EC2SecurityGroupRule
	for _, ipr := range perm.IpRanges {
		rule := EC2SecurityGroupRule{
			RuleId:      ec2ID("sgr"),
			GroupId:     groupId,
			GroupOwner:  sg.OwnerId,
			IsEgress:    isEgress,
			IpProtocol:  perm.IpProtocol,
			FromPort:    perm.FromPort,
			ToPort:      perm.ToPort,
			CidrIpv4:    ipr.CidrIp,
			Description: ipr.Description,
		}
		ec2SecurityGroupRules.Put(rule.RuleId, rule)
		rules = append(rules, rule)
	}
	for _, gp := range perm.UserIdGroupPairs {
		rule := EC2SecurityGroupRule{
			RuleId:      ec2ID("sgr"),
			GroupId:     groupId,
			GroupOwner:  sg.OwnerId,
			IsEgress:    isEgress,
			IpProtocol:  perm.IpProtocol,
			FromPort:    perm.FromPort,
			ToPort:      perm.ToPort,
			RefGroupId:  gp.GroupId,
			Description: gp.Description,
		}
		ec2SecurityGroupRules.Put(rule.RuleId, rule)
		rules = append(rules, rule)
	}
	return rules
}

func handleAuthorizeSecurityGroupIngress(w http.ResponseWriter, r *http.Request) {
	groupId := r.FormValue("GroupId")
	perm := parseIpPermission(r, "IpPermissions.1")

	ec2SecurityGroups.Update(groupId, func(sg *EC2SecurityGroup) {
		sg.IpPermissions = append(sg.IpPermissions, perm)
	})

	rules := createSecurityGroupRules(groupId, perm, false)
	var ruleSetXML strings.Builder
	for _, rule := range rules {
		ruleSetXML.WriteString(sgrItemXML(rule))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AuthorizeSecurityGroupIngressResponse %s>
  <requestId>%s</requestId><return>true</return>
  <securityGroupRuleSet>%s</securityGroupRuleSet>
</AuthorizeSecurityGroupIngressResponse>`, ec2Xmlns(), generateUUID(), ruleSetXML.String())
}

func handleAuthorizeSecurityGroupEgress(w http.ResponseWriter, r *http.Request) {
	groupId := r.FormValue("GroupId")
	perm := parseIpPermission(r, "IpPermissions.1")

	ec2SecurityGroups.Update(groupId, func(sg *EC2SecurityGroup) {
		sg.IpPermissionsEgress = append(sg.IpPermissionsEgress, perm)
	})

	rules := createSecurityGroupRules(groupId, perm, true)
	var ruleSetXML strings.Builder
	for _, rule := range rules {
		ruleSetXML.WriteString(sgrItemXML(rule))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AuthorizeSecurityGroupEgressResponse %s>
  <requestId>%s</requestId><return>true</return>
  <securityGroupRuleSet>%s</securityGroupRuleSet>
</AuthorizeSecurityGroupEgressResponse>`, ec2Xmlns(), generateUUID(), ruleSetXML.String())
}

func handleRevokeSecurityGroupIngress(w http.ResponseWriter, r *http.Request) {
	groupId := r.FormValue("GroupId")
	perm := parseIpPermission(r, "IpPermissions.1")

	ec2SecurityGroups.Update(groupId, func(sg *EC2SecurityGroup) {
		sg.IpPermissions = removePermission(sg.IpPermissions, perm)
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<RevokeSecurityGroupIngressResponse %s>
  <requestId>%s</requestId><return>true</return>
</RevokeSecurityGroupIngressResponse>`, ec2Xmlns(), generateUUID())
}

func handleRevokeSecurityGroupEgress(w http.ResponseWriter, r *http.Request) {
	groupId := r.FormValue("GroupId")
	perm := parseIpPermission(r, "IpPermissions.1")

	ec2SecurityGroups.Update(groupId, func(sg *EC2SecurityGroup) {
		sg.IpPermissionsEgress = removePermission(sg.IpPermissionsEgress, perm)
	})

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<RevokeSecurityGroupEgressResponse %s>
  <requestId>%s</requestId><return>true</return>
</RevokeSecurityGroupEgressResponse>`, ec2Xmlns(), generateUUID())
}

func handleDescribeSecurityGroupRules(w http.ResponseWriter, r *http.Request) {
	// Check for direct SecurityGroupRuleId params
	var ruleIds []string
	for i := 1; ; i++ {
		id := r.FormValue(fmt.Sprintf("SecurityGroupRuleId.%d", i))
		if id == "" {
			break
		}
		ruleIds = append(ruleIds, id)
	}

	// Check for filters
	var groupId string
	for i := 1; ; i++ {
		name := r.FormValue(fmt.Sprintf("Filter.%d.Name", i))
		if name == "" {
			break
		}
		if name == "group-id" {
			groupId = r.FormValue(fmt.Sprintf("Filter.%d.Value.1", i))
		}
	}

	var rules []EC2SecurityGroupRule
	if len(ruleIds) > 0 {
		for _, id := range ruleIds {
			if rule, ok := ec2SecurityGroupRules.Get(id); ok {
				rules = append(rules, rule)
			}
		}
	} else if groupId != "" {
		rules = ec2SecurityGroupRules.Filter(func(rule EC2SecurityGroupRule) bool {
			return rule.GroupId == groupId
		})
	} else {
		rules = ec2SecurityGroupRules.List()
	}

	var items strings.Builder
	for _, rule := range rules {
		items.WriteString(sgrItemXML(rule))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeSecurityGroupRulesResponse %s>
  <requestId>%s</requestId>
  <securityGroupRuleSet>%s</securityGroupRuleSet>
</DescribeSecurityGroupRulesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

// ---- Network Interfaces ----

func handleDescribeNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	// The simulator doesn't create real ENIs. Return an empty set so
	// terraform can proceed with deleting security groups and subnets.
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeNetworkInterfacesResponse %s>
  <requestId>%s</requestId>
  <networkInterfaceSet/>
</DescribeNetworkInterfacesResponse>`, ec2Xmlns(), generateUUID())
}

func removePermission(perms []EC2IpPermission, target EC2IpPermission) []EC2IpPermission {
	var result []EC2IpPermission
	for _, p := range perms {
		if p.IpProtocol == target.IpProtocol && p.FromPort == target.FromPort && p.ToPort == target.ToPort {
			continue
		}
		result = append(result, p)
	}
	return result
}
