package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
	realexec "github.com/sockerless/simulator-realexec"
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
	GroupId             string
	GroupName           string
	Description         string
	VpcId               string
	Tags                []EC2Tag
	OwnerId             string
	IpPermissions       []EC2IpPermission
	IpPermissionsEgress []EC2IpPermission
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
	IsEgress    bool
	IpProtocol  string
	FromPort    int
	ToPort      int
	CidrIpv4    string
	RefGroupId  string
	Description string
}

type EC2Tag struct {
	Key   string
	Value string
}

type EC2Instance struct {
	InstanceId         string
	ReservationId      string
	ImageId            string
	InstanceType       string
	SubnetId           string
	VpcId              string
	State              string
	PrivateIpAddress   string
	PublicIpAddress    string
	SecurityGroupIds   []string
	Tags               []EC2Tag
	LaunchTime         string
	KeyName            string
	Architecture       string
	RootDeviceName     string
	NetworkInterfaceId string
}

type EC2NetworkInterface struct {
	NetworkInterfaceId  string
	SubnetId            string
	VpcId               string
	PrivateIpAddress    string
	Status              string
	AttachmentId        string
	InstanceId          string
	DeviceIndex         int
	DeleteOnTermination bool
	SourceDestDisabled  bool
	Description         string
	SecondaryPrivateIps []string
	SecurityGroupIds    []string
	Tags                []EC2Tag
	OwnerId             string
}

type EC2Volume struct {
	VolumeId         string
	Size             int
	SnapshotId       string
	AvailabilityZone string
	State            string
	CreateTime       string
	VolumeType       string
	Encrypted        bool
	Tags             []EC2Tag
	Attachments      []EC2VolumeAttachment
	HostPath         string
	DockerVolumeName string
	Data             []byte
}

type EC2VolumeAttachment struct {
	VolumeId            string
	InstanceId          string
	Device              string
	State               string
	AttachTime          string
	DeleteOnTermination bool
}

type EC2Snapshot struct {
	SnapshotId       string
	VolumeId         string
	VolumeSize       int
	State            string
	StartTime        string
	CompletionDue    string
	Progress         string
	Description      string
	OwnerId          string
	Tags             []EC2Tag
	HostPath         string
	DockerVolumeName string
	VolumeData       []byte
}

// State stores
var (
	ec2Vpcs               sim.Store[EC2Vpc]
	ec2Subnets            sim.Store[EC2Subnet]
	ec2InternetGateways   sim.Store[EC2InternetGateway]
	ec2NatGateways        sim.Store[EC2NatGateway]
	ec2ElasticIPs         sim.Store[EC2ElasticIP]
	ec2RouteTables        sim.Store[EC2RouteTable]
	ec2SecurityGroups     sim.Store[EC2SecurityGroup]
	ec2SecurityGroupRules sim.Store[EC2SecurityGroupRule]
	ec2Instances          sim.Store[EC2Instance]
	ec2NetworkInterfaces  sim.Store[EC2NetworkInterface]
	ec2Volumes            sim.Store[EC2Volume]
	ec2Snapshots          sim.Store[EC2Snapshot]
	// ec2SubnetIPCursor tracks the next host octet to hand out per
	// subnet for AllocateSubnetIP. Real EC2 maintains a per-subnet
	// allocation pool; we approximate with a monotonic counter that
	// starts at .4 (real AWS reserves .0/.1/.2/.3 + last for broadcast).
	ec2SubnetIPCursor   = make(map[string]uint32)
	ec2SubnetIPCursorMu sync.Mutex
)

// AllocateSubnetIP picks the next available host address from a
// subnet's CIDR block. Returns an error if the subnet isn't registered
// (matches real AWS, where RunTask / CreateNetworkInterface against a
// non-existent subnet returns InvalidSubnetID.NotFound). The first four
// addresses (network + AWS-reserved router/DNS/future) and the last
// (broadcast) are skipped, mirroring AWS's reserved-host convention.
func AllocateSubnetIP(subnetID string) (string, error) {
	subnet, ok := ec2Subnets.Get(subnetID)
	if !ok {
		return "", fmt.Errorf("subnet %q not found", subnetID)
	}
	_, cidr, perr := net.ParseCIDR(subnet.CidrBlock)
	if perr != nil {
		return "", fmt.Errorf("subnet %q has invalid CidrBlock %q: %v", subnetID, subnet.CidrBlock, perr)
	}
	ec2SubnetIPCursorMu.Lock()
	defer ec2SubnetIPCursorMu.Unlock()
	cursor, ok := ec2SubnetIPCursor[subnetID]
	if !ok {
		// AWS reserves the first 4 host addresses in every subnet
		// (.0 network, .1 router, .2 DNS, .3 future use) and the
		// final .255 for broadcast. Start handing out at .4.
		cursor = 4
	}
	ones, bits := cidr.Mask.Size()
	hostBits := bits - ones
	if hostBits < 3 {
		return "", fmt.Errorf("subnet %q CIDR %q too small for AWS host reservations", subnetID, subnet.CidrBlock)
	}
	maxHosts := uint32(1) << uint32(hostBits)
	if cursor >= maxHosts-1 {
		return "", fmt.Errorf("subnet %q exhausted: no host addresses left in %q", subnetID, subnet.CidrBlock)
	}
	base := cidr.IP.To4()
	if base == nil {
		return "", fmt.Errorf("subnet %q CidrBlock %q is not IPv4", subnetID, subnet.CidrBlock)
	}
	ip := make(net.IP, 4)
	copy(ip, base)
	hostInt := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	hostInt += cursor
	ip[0] = byte(hostInt >> 24)
	ip[1] = byte(hostInt >> 16)
	ip[2] = byte(hostInt >> 8)
	ip[3] = byte(hostInt)
	ec2SubnetIPCursor[subnetID] = cursor + 1
	return ip.String(), nil
}

// ec2Owner() returns the EC2 resource owner — same as the AWS account
// ID. Tracks awsAccountID() so a SOCKERLESS_AWS_ACCOUNT_ID override
// propagates through every VPC/Subnet/SG OwnerId.
func ec2Owner() string { return awsAccountID() }

// ensureSimDefaults creates `vpc-sim` and `subnet-0123456789abcdef0` entries if they
// don't already exist. Called on simulator startup. Idempotent.
func ensureSimDefaults() {
	if _, ok := ec2Vpcs.Get("vpc-sim"); !ok {
		ec2Vpcs.Put("vpc-sim", EC2Vpc{
			VpcId:              "vpc-sim",
			CidrBlock:          "10.0.0.0/16",
			State:              "available",
			OwnerId:            ec2Owner(),
			IsDefault:          true,
			EnableDnsSupport:   true,
			EnableDnsHostnames: true,
		})
	}
	if _, ok := ec2Subnets.Get("subnet-0123456789abcdef0"); !ok {
		ec2Subnets.Put("subnet-0123456789abcdef0", EC2Subnet{
			SubnetId:            "subnet-0123456789abcdef0",
			VpcId:               "vpc-sim",
			CidrBlock:           "10.0.1.0/24",
			AvailabilityZone:    awsAvailabilityZone(),
			State:               "available",
			OwnerId:             ec2Owner(),
			MapPublicIpOnLaunch: false,
		})
	}
}

func registerEC2(r *sim.AWSQueryRouter, srv *sim.Server) {
	ec2Vpcs = sim.MakeStore[EC2Vpc](srv.DB(), "ec2_vpcs")
	ec2Subnets = sim.MakeStore[EC2Subnet](srv.DB(), "ec2_subnets")
	ec2InternetGateways = sim.MakeStore[EC2InternetGateway](srv.DB(), "ec2_internet_gateways")
	ec2NatGateways = sim.MakeStore[EC2NatGateway](srv.DB(), "ec2_nat_gateways")
	ec2ElasticIPs = sim.MakeStore[EC2ElasticIP](srv.DB(), "ec2_elastic_ips")
	ec2RouteTables = sim.MakeStore[EC2RouteTable](srv.DB(), "ec2_route_tables")
	ec2SecurityGroups = sim.MakeStore[EC2SecurityGroup](srv.DB(), "ec2_security_groups")
	ec2SecurityGroupRules = sim.MakeStore[EC2SecurityGroupRule](srv.DB(), "ec2_security_group_rules")
	ec2Instances = sim.MakeStore[EC2Instance](srv.DB(), "ec2_instances")
	ec2NetworkInterfaces = sim.MakeStore[EC2NetworkInterface](srv.DB(), "ec2_network_interfaces")
	ec2Volumes = sim.MakeStore[EC2Volume](srv.DB(), "ec2_volumes")
	ec2Snapshots = sim.MakeStore[EC2Snapshot](srv.DB(), "ec2_snapshots")

	// VPC
	r.Register("DescribeAccountAttributes", handleDescribeAccountAttributes)
	r.Register("DescribeAvailabilityZones", handleDescribeAvailabilityZones)
	r.Register("DescribeRegions", handleDescribeRegions)
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
	r.Register("ModifySecurityGroupRules", handleModifySecurityGroupRules)
	r.Register("DeleteSecurityGroup", handleDeleteSecurityGroup)
	r.Register("AuthorizeSecurityGroupIngress", handleAuthorizeSecurityGroupIngress)
	r.Register("AuthorizeSecurityGroupEgress", handleAuthorizeSecurityGroupEgress)
	r.Register("RevokeSecurityGroupIngress", handleRevokeSecurityGroupIngress)
	r.Register("RevokeSecurityGroupEgress", handleRevokeSecurityGroupEgress)

	// Instances
	r.Register("RunInstances", handleRunInstances)
	r.Register("DescribeInstances", handleDescribeInstances)
	r.Register("TerminateInstances", handleTerminateInstances)
	r.Register("StopInstances", handleStopInstances)
	r.Register("StartInstances", handleStartInstances)
	r.Register("DescribeInstanceStatus", handleDescribeInstanceStatus)
	r.Register("DescribeInstanceAttribute", handleDescribeInstanceAttribute)
	r.Register("ModifyInstanceAttribute", handleModifyInstanceAttribute)
	r.Register("CreateTags", handleCreateTags)
	r.Register("DeleteTags", handleDeleteTags)
	r.Register("DescribeTags", handleDescribeTags)
	r.Register("DescribeVolumes", handleDescribeVolumes)
	r.Register("CreateVolume", handleCreateVolume)
	r.Register("AttachVolume", handleAttachVolume)
	r.Register("DetachVolume", handleDetachVolume)
	r.Register("DeleteVolume", handleDeleteVolume)
	r.Register("ModifyVolume", handleModifyVolume)
	r.Register("CreateSnapshot", handleCreateSnapshot)
	r.Register("DescribeSnapshots", handleDescribeSnapshots)
	r.Register("DeleteSnapshot", handleDeleteSnapshot)
	r.Register("DescribeImages", handleDescribeImages)
	r.Register("DescribeInstanceTypes", handleDescribeInstanceTypes)
	r.Register("DescribeInstanceTypeOfferings", handleDescribeInstanceTypeOfferings)
	r.Register("DescribeKeyPairs", handleDescribeKeyPairs)

	// Pre-register a default `vpc-sim` + `subnet-0123456789abcdef0` so harnesses that
	// hardcode those IDs (smoke-tests/run.sh, backend config examples)
	// can call DescribeSubnets / DescribeVpcs without first provisioning.
	// Real AWS would never have these exact IDs; they're a simulator
	// convention. Backends resolve subnet → VPC on network create, so
	// without this pre-registration Cloud Map namespace setup fails
	// silently.
	ensureSimDefaults()

	// Network Interfaces (used during destroy to check ENIs before deleting SGs/subnets)
	r.Register("DescribeNetworkInterfaces", handleDescribeNetworkInterfaces)
	r.Register("CreateNetworkInterface", handleCreateNetworkInterface)
	r.Register("AttachNetworkInterface", handleAttachNetworkInterface)
	r.Register("DetachNetworkInterface", handleDetachNetworkInterface)
	r.Register("DeleteNetworkInterface", handleDeleteNetworkInterface)
	r.Register("ModifyNetworkInterfaceAttribute", handleModifyNetworkInterfaceAttribute)
	r.Register("AssignPrivateIpAddresses", handleAssignPrivateIpAddresses)

	registerEC2LaunchTemplates(r, srv)
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

func handleDescribeAccountAttributes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeAccountAttributesResponse %s>
  <requestId>%s</requestId>
  <accountAttributeSet>
    <item><attributeName>supported-platforms</attributeName><attributeValueSet><item><attributeValue>VPC</attributeValue></item></attributeValueSet></item>
    <item><attributeName>default-vpc</attributeName><attributeValueSet><item><attributeValue>vpc-sim</attributeValue></item></attributeValueSet></item>
  </accountAttributeSet>
</DescribeAccountAttributesResponse>`, ec2Xmlns(), generateUUID())
}

func handleDescribeAvailabilityZones(w http.ResponseWriter, r *http.Request) {
	region := awsRegion()
	zone := awsAvailabilityZone()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeAvailabilityZonesResponse %s>
  <requestId>%s</requestId>
  <availabilityZoneInfo><item><zoneName>%s</zoneName><zoneId>%s-az1</zoneId><zoneType>availability-zone</zoneType><regionName>%s</regionName><zoneState>available</zoneState><groupName>%s</groupName><networkBorderGroup>%s</networkBorderGroup></item></availabilityZoneInfo>
</DescribeAvailabilityZonesResponse>`, ec2Xmlns(), generateUUID(), zone, region, region, region, region)
}

func handleDescribeRegions(w http.ResponseWriter, r *http.Request) {
	region := awsRegion()
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeRegionsResponse %s>
  <requestId>%s</requestId>
  <regionInfo><item><regionName>%s</regionName><regionEndpoint>ec2.%s.amazonaws.com</regionEndpoint><optInStatus>opt-in-not-required</optInStatus></item></regionInfo>
</DescribeRegionsResponse>`, ec2Xmlns(), generateUUID(), region, region)
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
		OwnerId:            ec2Owner(),
		IsDefault:          false,
		EnableDnsSupport:   true,
		EnableDnsHostnames: false,
	}
	ec2Vpcs.Put(id, vpc)
	if err := realexec.DetectNetworkCapabilities().Require(); err == nil {
		if err2 := ec2CreateRealVPC(r.Context(), vpc); err2 != nil {
			fmt.Fprintf(os.Stderr, "sim: real VPC %s network fabric unavailable: %v\n", id, err2)
		}
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateVpcResponse %s>
  <requestId>%s</requestId>
  <vpc>
    <vpcId>%s</vpcId><cidrBlock>%s</cidrBlock><state>available</state>
    <ownerId>%s</ownerId><isDefault>false</isDefault>
    %s
  </vpc>
</CreateVpcResponse>`, ec2Xmlns(), generateUUID(), id, cidr, ec2Owner(), writeTagSetXML(tags))
}

func vpcItemXML(vpc EC2Vpc) string {
	// Real DescribeVpcs lists the primary CIDR in cidrBlockAssociationSet as
	// well as in cidrBlock; data.aws_vpc reads cidr_block_associations from it.
	// The sim synthesizes a stable association id from the VPC id.
	assocID := "vpc-cidr-assoc-" + strings.TrimPrefix(vpc.VpcId, "vpc-")
	cidrAssoc := fmt.Sprintf(`<cidrBlockAssociationSet><item><associationId>%s</associationId><cidrBlock>%s</cidrBlock><cidrBlockState><state>associated</state></cidrBlockState></item></cidrBlockAssociationSet>`,
		assocID, vpc.CidrBlock)
	return fmt.Sprintf(`<item><vpcId>%s</vpcId><cidrBlock>%s</cidrBlock><state>%s</state><ownerId>%s</ownerId><isDefault>%t</isDefault><instanceTenancy>default</instanceTenancy>%s%s</item>`,
		vpc.VpcId, vpc.CidrBlock, vpc.State, vpc.OwnerId, vpc.IsDefault, cidrAssoc, writeTagSetXML(vpc.Tags))
}

func handleDescribeVpcs(w http.ResponseWriter, r *http.Request) {
	ids := ec2ParamList(r, "VpcId")
	for _, id := range ids {
		if _, ok := ec2Vpcs.Get(id); !ok {
			ec2ErrorXML(w, "InvalidVpcID.NotFound", fmt.Sprintf("The vpc ID '%s' does not exist", id), http.StatusBadRequest)
			return
		}
	}
	filters := ec2Filters(r)

	var items strings.Builder
	for _, v := range ec2Vpcs.List() {
		if len(ids) > 0 && !ec2StrInValues(v.VpcId, ids) {
			continue
		}
		if !ec2VpcMatchesFilters(v, filters) {
			continue
		}
		items.WriteString(vpcItemXML(v))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeVpcsResponse %s>
  <requestId>%s</requestId>
  <vpcSet>%s</vpcSet>
</DescribeVpcsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func ec2VpcMatchesFilters(v EC2Vpc, filters map[string][]string) bool {
	for name, vals := range filters {
		switch name {
		case "vpc-id":
			if !ec2StrInValues(v.VpcId, vals) {
				return false
			}
		case "cidr", "cidr-block-association.cidr-block":
			if !ec2StrInValues(v.CidrBlock, vals) {
				return false
			}
		case "state":
			if !ec2StrInValues(v.State, vals) {
				return false
			}
		case "is-default":
			if v.IsDefault != ec2StrInValues("true", vals) {
				return false
			}
		default:
			if handled, match := ec2TagFilterMatch(name, vals, v.Tags); handled && !match {
				return false
			}
		}
	}
	return true
}

// ec2TagFilterMatch evaluates the EC2 tag filter forms (`tag:<Key>` and
// `tag-key`). Returns (handled, match): handled=false means the filter name
// isn't a tag filter and the caller should decide.
func ec2TagFilterMatch(name string, vals []string, tags []EC2Tag) (handled, match bool) {
	switch {
	case strings.HasPrefix(name, "tag:"):
		key := strings.TrimPrefix(name, "tag:")
		for _, t := range tags {
			if t.Key == key && ec2StrInValues(t.Value, vals) {
				return true, true
			}
		}
		return true, false
	case name == "tag-key":
		for _, t := range tags {
			if ec2StrInValues(t.Key, vals) {
				return true, true
			}
		}
		return true, false
	}
	return false, false
}

func handleDeleteVpc(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("VpcId")
	ec2Vpcs.Delete(id)
	if err := ec2DeleteRealVPC(r.Context(), id); err != nil {
		ec2ErrorXML(w, "DependencyViolation", fmt.Sprintf("failed to delete real VPC network fabric: %v", err), http.StatusServiceUnavailable)
		return
	}

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
		OwnerId:          ec2Owner(),
	}
	ec2Subnets.Put(id, subnet)
	if err := realexec.DetectNetworkCapabilities().Require(); err == nil {
		if err2 := ec2CreateRealSubnet(r.Context(), subnet); err2 != nil {
			fmt.Fprintf(os.Stderr, "sim: real subnet %s network fabric unavailable: %v\n", id, err2)
		}
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateSubnetResponse %s>
  <requestId>%s</requestId>
  <subnet>
    <subnetId>%s</subnetId><vpcId>%s</vpcId><cidrBlock>%s</cidrBlock>
    <availabilityZone>%s</availabilityZone><state>available</state>
    <mapPublicIpOnLaunch>false</mapPublicIpOnLaunch><ownerId>%s</ownerId>
    %s
  </subnet>
</CreateSubnetResponse>`, ec2Xmlns(), generateUUID(), id, vpcId, cidr, az, ec2Owner(), writeTagSetXML(tags))
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
	} else if vpcIDs := ec2Filters(r)["vpc-id"]; len(vpcIDs) > 0 {
		subnets = ec2Subnets.Filter(func(s EC2Subnet) bool {
			return ec2StrInValues(s.VpcId, vpcIDs)
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
	if err := ec2DeleteRealSubnet(r.Context(), id); err != nil {
		ec2ErrorXML(w, "DependencyViolation", fmt.Sprintf("failed to delete real subnet network fabric: %v", err), http.StatusServiceUnavailable)
		return
	}

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
		OwnerId:           ec2Owner(),
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
</CreateInternetGatewayResponse>`, ec2Xmlns(), generateUUID(), id, ec2Owner(), writeTagSetXML(tags))
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
	ip, err := realexec.ReserveAWSPublicIPv4(id, nil)
	if err != nil {
		ec2ErrorXML(w, "AddressLimitExceeded", fmt.Sprintf("failed to reserve real public IPv4 lease: %v", err), http.StatusServiceUnavailable)
		return
	}

	eip := EC2ElasticIP{
		AllocationId: id,
		PublicIp:     ip.String(),
		Domain:       domain,
		Tags:         tags,
	}
	ec2ElasticIPs.Put(id, eip)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AllocateAddressResponse %s>
  <requestId>%s</requestId>
  <allocationId>%s</allocationId><publicIp>%s</publicIp><domain>%s</domain>
</AllocateAddressResponse>`, ec2Xmlns(), generateUUID(), id, ip.String(), domain)
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
	if eip, ok := ec2ElasticIPs.Get(id); ok {
		realexec.ReleasePublicIPv4(net.ParseIP(eip.PublicIp))
	}
	ec2ElasticIPs.Delete(id)

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ReleaseAddressResponse %s>
  <requestId>%s</requestId><return>true</return>
</ReleaseAddressResponse>`, ec2Xmlns(), generateUUID())
}

func handleDescribeAddressesAttribute(w http.ResponseWriter, r *http.Request) {
	allocId := r.FormValue("AllocationId.1")

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
	privateIP, err := AllocateSubnetIP(subnetId)
	if err != nil {
		ec2ErrorXML(w, "InsufficientFreeAddressesInSubnet", fmt.Sprintf("failed to allocate NAT gateway private IP: %v", err), http.StatusBadRequest)
		return
	}
	eniID := ec2ID("eni")

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
			PrivateIp:          privateIP,
			NetworkInterfaceId: eniID,
		}},
		CreateTime: time.Now().UTC().Format(time.RFC3339),
	}
	// A NAT gateway is a pure control-plane object from the API's perspective,
	// so it is always modeled (State:"available", describable) — exactly like
	// handleCreateVpc. Real NAT fabric is programmed opportunistically, only
	// when the host actually has the network capabilities; its absence must not
	// fail the API call (IaC/control-plane testing in SIM_RUNTIME=process runs
	// on hosts without CAP_NET_ADMIN/nft).
	ec2NatGateways.Put(id, natgw)
	if err := realexec.DetectNetworkCapabilities().Require(); err == nil {
		if err2 := ec2CreateRealNATGateway(r.Context(), natgw); err2 != nil {
			fmt.Fprintf(os.Stderr, "sim: real NAT gateway %s network fabric unavailable: %v\n", id, err2)
		}
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateNatGatewayResponse %s>
  <requestId>%s</requestId>
  <natGateway>
    <natGatewayId>%s</natGatewayId><subnetId>%s</subnetId>
    <vpcId>%s</vpcId><state>available</state>
    <natGatewayAddressSet>
      <item><allocationId>%s</allocationId><publicIp>%s</publicIp><privateIp>%s</privateIp></item>
    </natGatewayAddressSet>
    <createTime>%s</createTime>
    %s
  </natGateway>
</CreateNatGatewayResponse>`, ec2Xmlns(), generateUUID(), id, subnetId, vpcId, allocId, publicIp, privateIP, natgw.CreateTime, writeTagSetXML(tags))
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
	} else if vpcIDs := ec2Filters(r)["vpc-id"]; len(vpcIDs) > 0 {
		nats = ec2NatGateways.Filter(func(n EC2NatGateway) bool {
			return ec2StrInValues(n.VpcId, vpcIDs)
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
	if err := ec2DeleteRealNATGateway(r.Context(), id); err != nil {
		ec2ErrorXML(w, "DependencyViolation", fmt.Sprintf("failed to delete real NAT gateway fabric: %v", err), http.StatusServiceUnavailable)
		return
	}

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
		OwnerId: ec2Owner(),
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
	} else if assocIDs := ec2Filters(r)["association.route-table-association-id"]; len(assocIDs) > 0 {
		for _, rt := range ec2RouteTables.List() {
			for _, a := range rt.Associations {
				if ec2StrInValues(a.AssociationId, assocIDs) {
					rts = append(rts, rt)
					break
				}
			}
		}
	} else if vpcIDs := ec2Filters(r)["vpc-id"]; len(vpcIDs) > 0 {
		rts = ec2RouteTables.Filter(func(rt EC2RouteTable) bool {
			return ec2StrInValues(rt.VpcId, vpcIDs)
		})
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
	if natId != "" {
		// The route is always modeled below; programming the real NAT route is
		// opportunistic (only when the host has network capabilities) and must
		// not fail the API call. Mirrors handleCreateNatGateway.
		if err := realexec.DetectNetworkCapabilities().Require(); err == nil {
			if err2 := ec2ConfigureRealNATRoute(r.Context(), rtId, destCidr, natId); err2 != nil {
				fmt.Fprintf(os.Stderr, "sim: real NAT route to %s unavailable: %v\n", natId, err2)
			}
		}
	}

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
		OwnerId:     ec2Owner(),
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
	ids := ec2ParamList(r, "GroupId")
	names := ec2ParamList(r, "GroupName")
	filters := ec2Filters(r)

	var items strings.Builder
	for _, sg := range ec2SecurityGroups.List() {
		if len(ids) > 0 && !ec2StrInValues(sg.GroupId, ids) {
			continue
		}
		if len(names) > 0 && !ec2StrInValues(sg.GroupName, names) {
			continue
		}
		if !ec2SecurityGroupMatchesFilters(sg, filters) {
			continue
		}
		items.WriteString(sgItemXML(sg))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeSecurityGroupsResponse %s>
  <requestId>%s</requestId>
  <securityGroupInfo>%s</securityGroupInfo>
</DescribeSecurityGroupsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func ec2SecurityGroupMatchesFilters(sg EC2SecurityGroup, filters map[string][]string) bool {
	for name, vals := range filters {
		switch name {
		case "vpc-id":
			if !ec2StrInValues(sg.VpcId, vals) {
				return false
			}
		case "group-id":
			if !ec2StrInValues(sg.GroupId, vals) {
				return false
			}
		case "group-name":
			if !ec2StrInValues(sg.GroupName, vals) {
				return false
			}
		case "description":
			if !ec2StrInValues(sg.Description, vals) {
				return false
			}
		default:
			if handled, match := ec2TagFilterMatch(name, vals, sg.Tags); handled && !match {
				return false
			}
		}
	}
	return true
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
	if err := ec2ReapplyRealSecurityGroup(r.Context(), groupId); err != nil {
		ec2ErrorXML(w, "DependencyViolation", fmt.Sprintf("failed to program real security group ingress rules: %v", err), http.StatusServiceUnavailable)
		return
	}
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
	if err := ec2ReapplyRealSecurityGroup(r.Context(), groupId); err != nil {
		ec2ErrorXML(w, "DependencyViolation", fmt.Sprintf("failed to program real security group ingress rules: %v", err), http.StatusServiceUnavailable)
		return
	}

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

// handleModifySecurityGroupRules updates existing rule attributes in place.
// terraform-provider-aws v6 calls this for an in-place change to an
// aws_vpc_security_group_{ingress,egress}_rule instead of revoke + authorize.
func handleModifySecurityGroupRules(w http.ResponseWriter, r *http.Request) {
	for i := 1; ; i++ {
		base := fmt.Sprintf("SecurityGroupRule.%d", i)
		ruleID := r.FormValue(base + ".SecurityGroupRuleId")
		if ruleID == "" {
			break
		}
		rule, ok := ec2SecurityGroupRules.Get(ruleID)
		if !ok {
			ec2ErrorXML(w, "InvalidSecurityGroupRuleId.NotFound",
				fmt.Sprintf("The security group rule ID %q does not exist", ruleID), http.StatusBadRequest)
			return
		}
		sr := base + ".SecurityGroupRule"
		if v := r.FormValue(sr + ".Description"); v != "" {
			rule.Description = v
		}
		if v := r.FormValue(sr + ".IpProtocol"); v != "" {
			rule.IpProtocol = v
		}
		if v := r.FormValue(sr + ".FromPort"); v != "" {
			fmt.Sscanf(v, "%d", &rule.FromPort)
		}
		if v := r.FormValue(sr + ".ToPort"); v != "" {
			fmt.Sscanf(v, "%d", &rule.ToPort)
		}
		if v := r.FormValue(sr + ".CidrIpv4"); v != "" {
			rule.CidrIpv4 = v
		}
		if v := r.FormValue(sr + ".ReferencedGroupId"); v != "" {
			rule.RefGroupId = v
		}
		ec2SecurityGroupRules.Put(ruleID, rule)
	}
	ec2WriteSimpleResponse(w, "ModifySecurityGroupRulesResponse")
}

// ---- Instances ----

func ec2ParamList(r *http.Request, prefix string) []string {
	var values []string
	for i := 1; ; i++ {
		v := r.FormValue(fmt.Sprintf("%s.%d", prefix, i))
		if v == "" {
			break
		}
		values = append(values, v)
	}
	return values
}

func ec2ErrorXML(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<Response><Errors><Error><Code>%s</Code><Message>%s</Message></Error></Errors><RequestID>%s</RequestID></Response>`,
		code, message, generateUUID())
}

func ec2Filters(r *http.Request) map[string][]string {
	filters := map[string][]string{}
	for i := 1; ; i++ {
		name := r.FormValue(fmt.Sprintf("Filter.%d.Name", i))
		if name == "" {
			break
		}
		for j := 1; ; j++ {
			value := r.FormValue(fmt.Sprintf("Filter.%d.Value.%d", i, j))
			if value == "" {
				break
			}
			filters[name] = append(filters[name], value)
		}
	}
	return filters
}

// ec2StrInValues reports whether s is one of vals (a filter's values are OR'd
// in real EC2).
func ec2StrInValues(s string, vals []string) bool {
	for _, v := range vals {
		if v == s {
			return true
		}
	}
	return false
}

func instanceStateCode(state string) int {
	switch state {
	case "pending":
		return 0
	case "running":
		return 16
	case "shutting-down":
		return 32
	case "terminated":
		return 48
	case "stopping":
		return 64
	case "stopped":
		return 80
	default:
		return 0
	}
}

func ec2InstanceXML(inst EC2Instance) string {
	var groups strings.Builder
	for _, groupID := range inst.SecurityGroupIds {
		name := groupID
		if sg, ok := ec2SecurityGroups.Get(groupID); ok {
			name = sg.GroupName
		}
		fmt.Fprintf(&groups, "<item><groupId>%s</groupId><groupName>%s</groupName></item>", groupID, name)
	}
	if groups.Len() == 0 {
		groups.WriteString("")
	}
	var ni strings.Builder
	if inst.NetworkInterfaceId != "" {
		fmt.Fprintf(&ni, `<networkInterfaceSet><item>
      <networkInterfaceId>%s</networkInterfaceId>
      <subnetId>%s</subnetId>
      <vpcId>%s</vpcId>
      <description/>
      <ownerId>%s</ownerId>
      <status>in-use</status>
      <macAddress>02:00:00:00:00:01</macAddress>
      <privateIpAddress>%s</privateIpAddress>
      <privateDnsName>ip-%s.%s.compute.internal</privateDnsName>
      <sourceDestCheck>true</sourceDestCheck>
      <groupSet>%s</groupSet>
      <attachment><attachmentId>eni-attach-%s</attachmentId><deviceIndex>0</deviceIndex><status>attached</status><attachTime>%s</attachTime><deleteOnTermination>true</deleteOnTermination></attachment>
    </item></networkInterfaceSet>`,
			inst.NetworkInterfaceId, inst.SubnetId, inst.VpcId, ec2Owner(), inst.PrivateIpAddress,
			strings.ReplaceAll(inst.PrivateIpAddress, ".", "-"), awsRegion(), groups.String(), inst.NetworkInterfaceId, inst.LaunchTime)
	} else {
		ni.WriteString("<networkInterfaceSet/>")
	}
	return fmt.Sprintf(`<item>
    <instanceId>%s</instanceId>
    <imageId>%s</imageId>
    <instanceState><code>%d</code><name>%s</name></instanceState>
    <privateDnsName>ip-%s.%s.compute.internal</privateDnsName>
    <dnsName/>
    <reason/>
    <amiLaunchIndex>0</amiLaunchIndex>
    <productCodes/>
    <instanceType>%s</instanceType>
    <launchTime>%s</launchTime>
    <placement><availabilityZone>%s</availabilityZone><groupName/><tenancy>default</tenancy></placement>
    <monitoring><state>disabled</state></monitoring>
    <subnetId>%s</subnetId>
    <vpcId>%s</vpcId>
    <privateIpAddress>%s</privateIpAddress>
    <sourceDestCheck>true</sourceDestCheck>
    <groupSet>%s</groupSet>
    <architecture>%s</architecture>
    <rootDeviceType>ebs</rootDeviceType>
    <rootDeviceName>%s</rootDeviceName>
    <blockDeviceMapping><item><deviceName>%s</deviceName><ebs><volumeId>%s</volumeId><status>attached</status><attachTime>%s</attachTime><deleteOnTermination>true</deleteOnTermination></ebs></item></blockDeviceMapping>
    <virtualizationType>hvm</virtualizationType>
    <clientToken/>
    %s
    %s
  </item>`,
		inst.InstanceId, inst.ImageId, instanceStateCode(inst.State), inst.State,
		strings.ReplaceAll(inst.PrivateIpAddress, ".", "-"), awsRegion(), inst.InstanceType, inst.LaunchTime,
		awsAvailabilityZone(), inst.SubnetId, inst.VpcId, inst.PrivateIpAddress, groups.String(),
		inst.Architecture, inst.RootDeviceName, inst.RootDeviceName, "vol-"+strings.TrimPrefix(inst.InstanceId, "i-"), inst.LaunchTime,
		writeTagSetXML(inst.Tags), ni.String())
}

func runInstancesSecurityGroups(r *http.Request) []string {
	var groups []string
	for i := 1; ; i++ {
		v := r.FormValue(fmt.Sprintf("SecurityGroupId.%d", i))
		if v == "" {
			break
		}
		groups = append(groups, v)
	}
	for i := 1; ; i++ {
		v := r.FormValue(fmt.Sprintf("NetworkInterface.1.SecurityGroupId.%d", i))
		if v == "" {
			break
		}
		groups = append(groups, v)
	}
	return groups
}

func handleRunInstances(w http.ResponseWriter, r *http.Request) {
	minCount, maxCount, ok := runInstancesCounts(w, r)
	if !ok {
		return
	}
	imageID := r.FormValue("ImageId")
	if imageID == "" {
		imageID = "ami-simulated"
	}
	instanceType := r.FormValue("InstanceType")
	if instanceType == "" {
		instanceType = "t3.micro"
	}
	subnetID := r.FormValue("SubnetId")
	if subnetID == "" {
		subnetID = r.FormValue("NetworkInterface.1.SubnetId")
	}
	if subnetID == "" {
		subnetID = "subnet-0123456789abcdef0"
	}
	subnet, ok := ec2Subnets.Get(subnetID)
	if !ok {
		sim.AWSErrorf(w, "InvalidSubnetID.NotFound", http.StatusBadRequest, "The subnet ID %q does not exist", subnetID)
		return
	}
	// The instance is always modeled at the control plane (reaches "running",
	// describable) — like VPC/subnet/NAT. A real Firecracker VM is booted
	// opportunistically in ec2TransitionInstanceToRunning only when the host
	// has VM capabilities; their absence must not fail RunInstances, so
	// IaC/control-plane testing works in SIM_RUNTIME=process.
	reservationID := ec2ID("r")
	sgIDs := runInstancesSecurityGroups(r)
	if len(sgIDs) == 0 {
		for _, sg := range ec2SecurityGroups.Filter(func(sg EC2SecurityGroup) bool {
			return sg.VpcId == subnet.VpcId && sg.GroupName == "default"
		}) {
			sgIDs = append(sgIDs, sg.GroupId)
			break
		}
	}
	launchTime := time.Now().UTC().Format(time.RFC3339)
	tags := parseTags(r)
	var instances []EC2Instance
	for i := 0; i < maxCount; i++ {
		privateIP := ""
		if i == 0 {
			privateIP = r.FormValue("PrivateIpAddress")
			if privateIP == "" {
				privateIP = r.FormValue("NetworkInterface.1.PrivateIpAddress")
			}
		}
		if privateIP == "" {
			ip, err := AllocateSubnetIP(subnetID)
			if err != nil {
				if i < minCount {
					sim.AWSError(w, "InsufficientFreeAddressesInSubnet", err.Error(), http.StatusBadRequest)
					return
				}
				break
			}
			privateIP = ip
		}
		inst, err := ec2CreateInstance(EC2InstanceCreateSpec{
			Context:          r.Context(),
			ReservationId:    reservationID,
			ImageId:          imageID,
			InstanceType:     instanceType,
			Subnet:           subnet,
			SubnetId:         subnetID,
			PrivateIP:        privateIP,
			SecurityGroupIds: sgIDs,
			Tags:             tags,
			LaunchTime:       launchTime,
			KeyName:          r.FormValue("KeyName"),
			State:            "pending",
		})
		if err != nil {
			if i < minCount {
				ec2ErrorXML(w, "InsufficientFreeAddressesInSubnet", fmt.Sprintf("failed to attach real EC2 network interface: %v", err), http.StatusServiceUnavailable)
				return
			}
			break
		}
		instances = append(instances, inst)
		go ec2TransitionInstanceToRunning(inst.InstanceId)
	}

	var instanceItems strings.Builder
	for _, inst := range instances {
		instanceItems.WriteString(ec2InstanceXML(inst))
	}

	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<RunInstancesResponse %s>
  <requestId>%s</requestId>
  <reservationId>%s</reservationId>
  <ownerId>%s</ownerId>
  <groupSet/>
  <instancesSet>%s</instancesSet>
</RunInstancesResponse>`, ec2Xmlns(), generateUUID(), reservationID, ec2Owner(), instanceItems.String())
}

func runInstancesCounts(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	minCount, maxCount := 1, 1
	if v := r.FormValue("MinCount"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &minCount); err != nil || minCount < 1 {
			ec2ErrorXML(w, "InvalidParameterValue", "MinCount must be greater than 0", http.StatusBadRequest)
			return 0, 0, false
		}
	}
	if v := r.FormValue("MaxCount"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &maxCount); err != nil || maxCount < 1 {
			ec2ErrorXML(w, "InvalidParameterValue", "MaxCount must be greater than 0", http.StatusBadRequest)
			return 0, 0, false
		}
	}
	if minCount > maxCount {
		ec2ErrorXML(w, "InvalidParameterCombination", "MinCount cannot be greater than MaxCount", http.StatusBadRequest)
		return 0, 0, false
	}
	return minCount, maxCount, true
}

type EC2InstanceCreateSpec struct {
	Context          context.Context
	ReservationId    string
	ImageId          string
	InstanceType     string
	Subnet           EC2Subnet
	SubnetId         string
	PrivateIP        string
	SecurityGroupIds []string
	Tags             []EC2Tag
	LaunchTime       string
	KeyName          string
	State            string
}

func ec2CreateInstance(spec EC2InstanceCreateSpec) (EC2Instance, error) {
	if spec.State == "" {
		spec.State = "pending"
	}
	if spec.Context == nil {
		spec.Context = context.Background()
	}
	instanceID := ec2ID("i")
	eniID := ec2ID("eni")
	rootDevice := "/dev/sda1"
	inst := EC2Instance{
		InstanceId:         instanceID,
		ReservationId:      spec.ReservationId,
		ImageId:            spec.ImageId,
		InstanceType:       spec.InstanceType,
		SubnetId:           spec.SubnetId,
		VpcId:              spec.Subnet.VpcId,
		State:              spec.State,
		PrivateIpAddress:   spec.PrivateIP,
		SecurityGroupIds:   spec.SecurityGroupIds,
		Tags:               spec.Tags,
		LaunchTime:         spec.LaunchTime,
		KeyName:            spec.KeyName,
		Architecture:       "x86_64",
		RootDeviceName:     rootDevice,
		NetworkInterfaceId: eniID,
	}
	ec2Instances.Put(instanceID, inst)
	ec2NetworkInterfaces.Put(eniID, EC2NetworkInterface{
		NetworkInterfaceId:  eniID,
		SubnetId:            spec.SubnetId,
		VpcId:               spec.Subnet.VpcId,
		PrivateIpAddress:    spec.PrivateIP,
		Status:              "in-use",
		AttachmentId:        "eni-attach-" + eniID,
		InstanceId:          instanceID,
		DeviceIndex:         0,
		DeleteOnTermination: true,
		SecurityGroupIds:    spec.SecurityGroupIds,
		Tags:                spec.Tags,
		OwnerId:             ec2Owner(),
	})
	rootVolumeID := "vol-" + strings.TrimPrefix(instanceID, "i-")
	rootVolume := EC2Volume{
		VolumeId:         rootVolumeID,
		Size:             8,
		SnapshotId:       "snap-" + strings.TrimPrefix(spec.ImageId, "ami-"),
		AvailabilityZone: spec.Subnet.AvailabilityZone,
		State:            "in-use",
		CreateTime:       spec.LaunchTime,
		VolumeType:       "gp3",
		Tags:             spec.Tags,
		Attachments: []EC2VolumeAttachment{{
			VolumeId:            rootVolumeID,
			InstanceId:          instanceID,
			Device:              rootDevice,
			State:               "attached",
			AttachTime:          spec.LaunchTime,
			DeleteOnTermination: true,
		}},
		Data: []byte{},
	}
	rootVolume.HostPath = EBSVolumeHostDir(rootVolumeID)
	ec2Volumes.Put(rootVolumeID, rootVolume)
	return inst, nil
}

func ec2TransitionInstanceToRunning(instanceID string) {
	inst, ok := ec2Instances.Get(instanceID)
	if !ok {
		return
	}
	// On a real-execution host, boot a real Firecracker VM; a boot failure
	// there is a genuine error, so the instance settles to "stopped". On an
	// API-only host (no VM capabilities) the instance is modeled as "running"
	// at the control plane — the same modeling tier VPC/subnet/NAT use. See
	if ec2RealVMHostAvailable() {
		if err := ec2StartRealVM(context.Background(), inst); err != nil {
			fmt.Fprintf(os.Stderr, "failed to boot real EC2 instance %s: %v\n", instanceID, err)
			ec2Instances.Update(instanceID, func(inst *EC2Instance) {
				if inst.State == "pending" {
					inst.State = "stopped"
				}
			})
			return
		}
	}
	ec2Instances.Update(instanceID, func(inst *EC2Instance) {
		if inst.State == "pending" {
			inst.State = "running"
		}
	})
}

func handleDescribeInstances(w http.ResponseWriter, r *http.Request) {
	instanceIDs := ec2ParamList(r, "InstanceId")
	var instances []EC2Instance
	if len(instanceIDs) > 0 {
		for _, id := range instanceIDs {
			inst, ok := ec2Instances.Get(id)
			if !ok {
				sim.AWSErrorf(w, "InvalidInstanceID.NotFound", http.StatusBadRequest, "The instance ID %q does not exist", id)
				return
			}
			instances = append(instances, inst)
		}
	} else {
		instances = ec2Instances.List()
	}
	// Reconcile reported state against real VM liveness only on a real-execution
	// host. On an API-only host there is no real VM behind a modeled "running"
	// instance, so the stored control-plane state is authoritative.
	if ec2RealVMHostAvailable() {
		for i := range instances {
			if instances[i].State == "running" && !ec2RealVMAlive(instances[i].InstanceId) {
				instances[i].State = "stopped"
				ec2Instances.Put(instances[i].InstanceId, instances[i])
			}
		}
	}
	filters := ec2Filters(r)
	if len(filters) > 0 {
		var err error
		instances, err = filterEC2Instances(instances, filters)
		if err != nil {
			ec2ErrorXML(w, "InvalidParameterValue", err.Error(), http.StatusBadRequest)
			return
		}
	}
	var reservations strings.Builder
	for _, inst := range instances {
		fmt.Fprintf(&reservations, `<item><reservationId>%s</reservationId><ownerId>%s</ownerId><groupSet/><instancesSet>%s</instancesSet></item>`,
			inst.ReservationId, ec2Owner(), ec2InstanceXML(inst))
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeInstancesResponse %s>
  <requestId>%s</requestId>
  <reservationSet>%s</reservationSet>
</DescribeInstancesResponse>`, ec2Xmlns(), generateUUID(), reservations.String())
}

func filterEC2Instances(instances []EC2Instance, filters map[string][]string) ([]EC2Instance, error) {
	out := make([]EC2Instance, 0, len(instances))
	for _, inst := range instances {
		matches, err := ec2InstanceMatchesFilters(inst, filters)
		if err != nil {
			return nil, err
		}
		if matches {
			out = append(out, inst)
		}
	}
	return out, nil
}

func ec2InstanceMatchesFilters(inst EC2Instance, filters map[string][]string) (bool, error) {
	for name, values := range filters {
		matched := false
		for _, value := range values {
			switch {
			case name == "instance-id":
				matched = inst.InstanceId == value
			case name == "instance-state-name":
				matched = inst.State == value
			case name == "image-id":
				matched = inst.ImageId == value
			case name == "vpc-id":
				matched = inst.VpcId == value
			case name == "subnet-id":
				matched = inst.SubnetId == value
			case name == "private-ip-address":
				matched = inst.PrivateIpAddress == value
			case name == "network-interface.network-interface-id":
				matched = inst.NetworkInterfaceId == value
			case name == "instance-type":
				matched = inst.InstanceType == value
			case name == "key-name":
				matched = inst.KeyName == value
			case name == "availability-zone":
				matched = awsAvailabilityZone() == value
			case name == "group-id":
				matched = stringInSlice(value, inst.SecurityGroupIds)
			case name == "tag-key":
				matched = ec2HasTagKey(inst.Tags, value)
			case strings.HasPrefix(name, "tag:"):
				matched = ec2HasTagValue(inst.Tags, strings.TrimPrefix(name, "tag:"), value)
			default:
				return false, fmt.Errorf("the filter %q is invalid", name)
			}
			if matched {
				break
			}
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func stringInSlice(needle string, values []string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func ec2HasTagKey(tags []EC2Tag, key string) bool {
	for _, tag := range tags {
		if tag.Key == key {
			return true
		}
	}
	return false
}

func ec2HasTagValue(tags []EC2Tag, key, value string) bool {
	for _, tag := range tags {
		if tag.Key == key && tag.Value == value {
			return true
		}
	}
	return false
}

func handleTerminateInstances(w http.ResponseWriter, r *http.Request) {
	writeInstanceStateChange(w, r, "terminated", true)
}

func handleStopInstances(w http.ResponseWriter, r *http.Request) {
	writeInstanceStateChange(w, r, "stopped", false)
}

func handleStartInstances(w http.ResponseWriter, r *http.Request) {
	writeInstanceStateChange(w, r, "running", false)
}

func writeInstanceStateChange(w http.ResponseWriter, r *http.Request, next string, deleteENI bool) {
	instanceIDs := ec2ParamList(r, "InstanceId")
	var items strings.Builder
	for _, id := range instanceIDs {
		inst, ok := ec2Instances.Get(id)
		if !ok {
			sim.AWSErrorf(w, "InvalidInstanceID.NotFound", http.StatusBadRequest, "The instance ID %q does not exist", id)
			return
		}
		prev := inst.State
		// Real VM start/stop only on a real-execution host; on an API-only host
		// the state change is purely modeled.
		if ec2RealVMHostAvailable() {
			if next == "running" {
				if err := ec2StartRealVM(r.Context(), inst); err != nil {
					fmt.Fprintf(os.Stderr, "failed to start real EC2 instance %s: %v\n", id, err)
					ec2ErrorXML(w, "IncorrectInstanceState", fmt.Sprintf("failed to start real EC2 instance: %v", err), http.StatusServiceUnavailable)
					return
				}
			}
			if next == "stopped" || next == "terminated" {
				if err := ec2StopRealVM(r.Context(), id); err != nil {
					ec2ErrorXML(w, "IncorrectInstanceState", fmt.Sprintf("failed to stop real EC2 instance: %v", err), http.StatusServiceUnavailable)
					return
				}
			}
		}
		inst.State = next
		ec2Instances.Put(id, inst)
		if next == "stopped" {
			ec2UpdateVolumeAttachmentsForInstance(id, "attached", "in-use")
		}
		if next == "running" {
			ec2UpdateVolumeAttachmentsForInstance(id, "attached", "in-use")
		}
		if deleteENI && inst.NetworkInterfaceId != "" {
			ec2NetworkInterfaces.Delete(inst.NetworkInterfaceId)
			if ec2RealNetHostAvailable() {
				if err := ec2DeleteRealNIC(r.Context(), inst.NetworkInterfaceId); err != nil {
					ec2ErrorXML(w, "DependencyViolation", fmt.Sprintf("failed to delete real EC2 network interface: %v", err), http.StatusServiceUnavailable)
					return
				}
			}
			ec2DeleteOnTerminationVolumes(id)
		}
		fmt.Fprintf(&items, `<item><instanceId>%s</instanceId><currentState><code>%d</code><name>%s</name></currentState><previousState><code>%d</code><name>%s</name></previousState></item>`,
			id, instanceStateCode(next), next, instanceStateCode(prev), prev)
	}
	action := "StopInstances"
	if next == "running" {
		action = "StartInstances"
	} else if next == "terminated" {
		action = "TerminateInstances"
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<%sResponse %s>
  <requestId>%s</requestId>
  <instancesSet>%s</instancesSet>
</%sResponse>`, action, ec2Xmlns(), generateUUID(), items.String(), action)
}

func ec2UpdateVolumeAttachmentsForInstance(instanceID, attachmentState, volumeState string) {
	for _, vol := range ec2Volumes.List() {
		changed := false
		for i := range vol.Attachments {
			if vol.Attachments[i].InstanceId == instanceID {
				vol.Attachments[i].State = attachmentState
				changed = true
			}
		}
		if changed {
			vol.State = volumeState
			ec2Volumes.Put(vol.VolumeId, vol)
		}
	}
}

func ec2DeleteOnTerminationVolumes(instanceID string) {
	for _, vol := range ec2Volumes.List() {
		keep := vol.Attachments[:0]
		deleteVolume := false
		for _, att := range vol.Attachments {
			if att.InstanceId == instanceID && att.DeleteOnTermination {
				deleteVolume = true
				continue
			}
			keep = append(keep, att)
		}
		if deleteVolume {
			if vol.DockerVolumeName != "" {
				ebsRemoveDockerVolume(vol.DockerVolumeName)
			} else {
				_ = os.RemoveAll(vol.HostPath)
			}
			ec2Volumes.Delete(vol.VolumeId)
			continue
		}
		if len(keep) != len(vol.Attachments) {
			vol.Attachments = keep
			if len(vol.Attachments) == 0 {
				vol.State = "available"
			}
			ec2Volumes.Put(vol.VolumeId, vol)
		}
	}
}

func handleDescribeInstanceStatus(w http.ResponseWriter, r *http.Request) {
	instanceIDs := ec2ParamList(r, "InstanceId")
	var instances []EC2Instance
	if len(instanceIDs) > 0 {
		for _, id := range instanceIDs {
			if inst, ok := ec2Instances.Get(id); ok && inst.State != "terminated" {
				instances = append(instances, inst)
			}
		}
	} else {
		instances = ec2Instances.Filter(func(inst EC2Instance) bool { return inst.State == "running" })
	}
	var items strings.Builder
	for _, inst := range instances {
		fmt.Fprintf(&items, `<item><instanceId>%s</instanceId><availabilityZone>%s</availabilityZone><instanceState><code>%d</code><name>%s</name></instanceState><systemStatus><status>ok</status><details><item><name>reachability</name><status>passed</status></item></details></systemStatus><instanceStatus><status>ok</status><details><item><name>reachability</name><status>passed</status></item></details></instanceStatus></item>`,
			inst.InstanceId, awsAvailabilityZone(), instanceStateCode(inst.State), inst.State)
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeInstanceStatusResponse %s>
  <requestId>%s</requestId>
  <instanceStatusSet>%s</instanceStatusSet>
</DescribeInstanceStatusResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDescribeInstanceAttribute(w http.ResponseWriter, r *http.Request) {
	instanceID := r.FormValue("InstanceId")
	inst, ok := ec2Instances.Get(instanceID)
	if !ok {
		sim.AWSErrorf(w, "InvalidInstanceID.NotFound", http.StatusBadRequest, "The instance ID %q does not exist", instanceID)
		return
	}
	attribute := r.FormValue("Attribute")
	if attribute == "" {
		attribute = "instanceType"
	}
	var body string
	switch attribute {
	case "instanceType":
		body = fmt.Sprintf("<instanceType><value>%s</value></instanceType>", inst.InstanceType)
	case "kernel":
		body = "<kernel><value/></kernel>"
	case "ramdisk":
		body = "<ramdisk><value/></ramdisk>"
	case "userData":
		body = "<userData><value/></userData>"
	case "disableApiTermination":
		body = "<disableApiTermination><value>false</value></disableApiTermination>"
	case "disableApiStop":
		body = "<disableApiStop><value>false</value></disableApiStop>"
	case "instanceInitiatedShutdownBehavior":
		body = "<instanceInitiatedShutdownBehavior><value>stop</value></instanceInitiatedShutdownBehavior>"
	case "rootDeviceName":
		body = fmt.Sprintf("<rootDeviceName><value>%s</value></rootDeviceName>", inst.RootDeviceName)
	case "sourceDestCheck":
		body = "<sourceDestCheck><value>true</value></sourceDestCheck>"
	default:
		body = fmt.Sprintf("<%s><value/></%s>", attribute, attribute)
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeInstanceAttributeResponse %s>
  <requestId>%s</requestId>
  <instanceId>%s</instanceId>
  %s
</DescribeInstanceAttributeResponse>`, ec2Xmlns(), generateUUID(), instanceID, body)
}

func handleModifyInstanceAttribute(w http.ResponseWriter, r *http.Request) {
	instanceID := r.FormValue("InstanceId")
	if _, ok := ec2Instances.Get(instanceID); !ok {
		sim.AWSErrorf(w, "InvalidInstanceID.NotFound", http.StatusBadRequest, "The instance ID %q does not exist", instanceID)
		return
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifyInstanceAttributeResponse %s><requestId>%s</requestId><return>true</return></ModifyInstanceAttributeResponse>`, ec2Xmlns(), generateUUID())
}

func handleCreateTags(w http.ResponseWriter, r *http.Request) {
	resources := ec2ParamList(r, "ResourceId")
	tags := parseIndexedTags(r, "Tag")
	for _, id := range resources {
		if strings.HasPrefix(id, "i-") {
			ec2Instances.Update(id, func(inst *EC2Instance) { inst.Tags = mergeEC2Tags(inst.Tags, tags) })
		}
		if strings.HasPrefix(id, "eni-") {
			ec2NetworkInterfaces.Update(id, func(eni *EC2NetworkInterface) { eni.Tags = mergeEC2Tags(eni.Tags, tags) })
		}
		if strings.HasPrefix(id, "vol-") {
			ec2Volumes.Update(id, func(vol *EC2Volume) { vol.Tags = mergeEC2Tags(vol.Tags, tags) })
		}
		if strings.HasPrefix(id, "snap-") {
			ec2Snapshots.Update(id, func(snap *EC2Snapshot) { snap.Tags = mergeEC2Tags(snap.Tags, tags) })
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateTagsResponse %s><requestId>%s</requestId><return>true</return></CreateTagsResponse>`, ec2Xmlns(), generateUUID())
}

func handleDeleteTags(w http.ResponseWriter, r *http.Request) {
	resources := ec2ParamList(r, "ResourceId")
	keys := map[string]bool{}
	for i := 1; ; i++ {
		key := r.FormValue(fmt.Sprintf("Tag.%d.Key", i))
		if key == "" {
			break
		}
		keys[key] = true
	}
	filter := func(tags []EC2Tag) []EC2Tag {
		var out []EC2Tag
		for _, tag := range tags {
			if !keys[tag.Key] {
				out = append(out, tag)
			}
		}
		return out
	}
	for _, id := range resources {
		if strings.HasPrefix(id, "i-") {
			ec2Instances.Update(id, func(inst *EC2Instance) { inst.Tags = filter(inst.Tags) })
		}
		if strings.HasPrefix(id, "eni-") {
			ec2NetworkInterfaces.Update(id, func(eni *EC2NetworkInterface) { eni.Tags = filter(eni.Tags) })
		}
		if strings.HasPrefix(id, "vol-") {
			ec2Volumes.Update(id, func(vol *EC2Volume) { vol.Tags = filter(vol.Tags) })
		}
		if strings.HasPrefix(id, "snap-") {
			ec2Snapshots.Update(id, func(snap *EC2Snapshot) { snap.Tags = filter(snap.Tags) })
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteTagsResponse %s><requestId>%s</requestId><return>true</return></DeleteTagsResponse>`, ec2Xmlns(), generateUUID())
}

func handleDescribeTags(w http.ResponseWriter, r *http.Request) {
	type tagEntry struct {
		resourceID   string
		resourceType string
		key          string
		value        string
	}
	filters := ec2Filters(r)
	matches := func(entry tagEntry) bool {
		for name, values := range filters {
			matched := false
			for _, value := range values {
				switch {
				case name == "resource-id" && entry.resourceID == value:
					matched = true
				case name == "resource-type" && entry.resourceType == value:
					matched = true
				case name == "key" && entry.key == value:
					matched = true
				case name == "value" && entry.value == value:
					matched = true
				case strings.HasPrefix(name, "tag:") && strings.TrimPrefix(name, "tag:") == entry.key && entry.value == value:
					matched = true
				}
			}
			if !matched {
				return false
			}
		}
		return true
	}
	var items strings.Builder
	writeEntry := func(entry tagEntry) {
		if !matches(entry) {
			return
		}
		fmt.Fprintf(&items, `<item><resourceId>%s</resourceId><resourceType>%s</resourceType><key>%s</key><value>%s</value></item>`,
			entry.resourceID, entry.resourceType, entry.key, entry.value)
	}
	for _, inst := range ec2Instances.List() {
		for _, tag := range inst.Tags {
			writeEntry(tagEntry{resourceID: inst.InstanceId, resourceType: "instance", key: tag.Key, value: tag.Value})
		}
	}
	for _, eni := range ec2NetworkInterfaces.List() {
		for _, tag := range eni.Tags {
			writeEntry(tagEntry{resourceID: eni.NetworkInterfaceId, resourceType: "network-interface", key: tag.Key, value: tag.Value})
		}
	}
	for _, vol := range ec2Volumes.List() {
		for _, tag := range vol.Tags {
			writeEntry(tagEntry{resourceID: vol.VolumeId, resourceType: "volume", key: tag.Key, value: tag.Value})
		}
	}
	for _, snap := range ec2Snapshots.List() {
		for _, tag := range snap.Tags {
			writeEntry(tagEntry{resourceID: snap.SnapshotId, resourceType: "snapshot", key: tag.Key, value: tag.Value})
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeTagsResponse %s>
  <requestId>%s</requestId>
  <tagSet>%s</tagSet>
</DescribeTagsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDescribeVolumes(w http.ResponseWriter, r *http.Request) {
	volumeIDs := ec2ParamList(r, "VolumeId")
	volumes := make([]EC2Volume, 0)
	if len(volumeIDs) > 0 {
		for _, id := range volumeIDs {
			vol, ok := ec2Volumes.Get(id)
			if !ok {
				ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", id), http.StatusBadRequest)
				return
			}
			volumes = append(volumes, vol)
		}
	} else {
		volumes = ec2Volumes.List()
	}
	var items strings.Builder
	for _, vol := range volumes {
		items.WriteString(ec2VolumeXML(vol))
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeVolumesResponse %s>
  <requestId>%s</requestId>
  <volumeSet>%s</volumeSet>
</DescribeVolumesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

// ebsECSDockerVolumeName returns the Docker named volume name for an ECS-managed EBS volume.
// ECS tasks use Docker named volumes so the data lives in the Docker daemon rather than on
// the sim process's own filesystem, making volumes accessible to sibling task containers
// regardless of whether the sim itself runs on the host or inside a container.
func ebsECSDockerVolumeName(volumeID string) string {
	return "sockerless-ebs-" + volumeID
}

// ebsSnapshotDockerVolumeName returns the Docker named volume name for a snapshot taken
// from an ECS-managed EBS volume.
func ebsSnapshotDockerVolumeName(snapshotID string) string {
	return "sockerless-snap-" + snapshotID
}

// ebsRemoveDockerVolume removes a Docker named volume created for an ECS EBS volume or
// snapshot. Errors are silently ignored (volume may already be absent).
func ebsRemoveDockerVolume(name string) {
	if name == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = sim.DockerClient().VolumeRemove(ctx, name, false)
}

// ebsCopyDockerVolumes copies all content from srcVolume into dstVolume using a
// short-lived Alpine container. The destination volume is auto-created by Docker if
// it does not yet exist.
func ebsCopyDockerVolumes(ctx context.Context, srcVolume, dstVolume string) error {
	handle, err := sim.StartContainerSync(sim.ContainerConfig{
		Image:        "alpine:latest",
		Architecture: "linux/" + runtime.GOARCH,
		Command:      []string{"sh", "-c", "cp -a /src/. /dst/"},
		Binds: []string{
			srcVolume + ":/src:ro",
			dstVolume + ":/dst",
		},
		Timeout: 60 * time.Second,
	}, discardLogSink{})
	if err != nil {
		return fmt.Errorf("start volume copy container: %w", err)
	}
	res := handle.Wait()
	if res.Error != nil {
		return fmt.Errorf("volume copy: %w", res.Error)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("volume copy exited with code %d", res.ExitCode)
	}
	return nil
}

func ebsHostRoot() string {
	if dir := os.Getenv("SIM_EBS_DATA_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "sockerless-sim-ebs")
}

func ebsVolumeHostDirPath(volumeID string) string {
	return filepath.Join(ebsHostRoot(), "volumes", volumeID)
}

func ebsVolumeBlockImagePath(vol EC2Volume) string {
	hostPath := vol.HostPath
	if hostPath == "" {
		hostPath = ebsVolumeHostDirPath(vol.VolumeId)
	}
	return filepath.Join(hostPath, "ebs.raw")
}

func ebsSnapshotHostDirPath(snapshotID string) string {
	return filepath.Join(ebsHostRoot(), "snapshots", snapshotID)
}

func EBSVolumeHostDir(volumeID string) string {
	dir := ebsVolumeHostDirPath(volumeID)
	_ = os.MkdirAll(dir, 0o777)
	return dir
}

func ebsPrepareVolumeHostPath(vol *EC2Volume) error {
	if vol.HostPath == "" {
		vol.HostPath = ebsVolumeHostDirPath(vol.VolumeId)
	}
	return os.MkdirAll(vol.HostPath, 0o777)
}

func ebsEnsureVolumeBlockImage(vol *EC2Volume) (string, error) {
	if err := ebsPrepareVolumeHostPath(vol); err != nil {
		return "", err
	}
	path := ebsVolumeBlockImagePath(*vol)
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
		if err != nil {
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
	}
	sizeBytes := int64(vol.Size) * 1024 * 1024 * 1024
	if sizeBytes < 1024*1024 {
		sizeBytes = 1024 * 1024
	}
	if err := os.Truncate(path, sizeBytes); err != nil {
		return "", err
	}
	return path, nil
}

func ebsCopyDir(dst, src string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o777); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			_ = out.Close()
			return err
		}
		if err := in.Close(); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

func ec2VolumeXML(vol EC2Volume) string {
	return "<item>" + ec2VolumeFieldsXML(vol) + "</item>"
}

func ec2VolumeFieldsXML(vol EC2Volume) string {
	var attachments strings.Builder
	if len(vol.Attachments) == 0 {
		attachments.WriteString("<attachmentSet/>")
	} else {
		attachments.WriteString("<attachmentSet>")
		for _, att := range vol.Attachments {
			fmt.Fprintf(&attachments, `<item><volumeId>%s</volumeId><instanceId>%s</instanceId><device>%s</device><status>%s</status><attachTime>%s</attachTime><deleteOnTermination>%t</deleteOnTermination></item>`,
				att.VolumeId, att.InstanceId, att.Device, att.State, att.AttachTime, att.DeleteOnTermination)
		}
		attachments.WriteString("</attachmentSet>")
	}
	snapshot := vol.SnapshotId
	if snapshot == "" {
		snapshot = ""
	}
	return fmt.Sprintf(`
    <volumeId>%s</volumeId>
    <size>%d</size>
    <snapshotId>%s</snapshotId>
    <availabilityZone>%s</availabilityZone>
    <status>%s</status>
    <createTime>%s</createTime>
    %s
    <volumeType>%s</volumeType>
    <encrypted>%t</encrypted>
    <multiAttachEnabled>false</multiAttachEnabled>
    %s
  `, vol.VolumeId, vol.Size, snapshot, vol.AvailabilityZone, vol.State, vol.CreateTime,
		attachments.String(), vol.VolumeType, vol.Encrypted, writeTagSetXML(vol.Tags))
}

func handleCreateVolume(w http.ResponseWriter, r *http.Request) {
	az := r.FormValue("AvailabilityZone")
	if az == "" {
		az = awsAvailabilityZone()
	}
	size := 8
	if v := r.FormValue("Size"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &size); err != nil || size < 1 {
			ec2ErrorXML(w, "InvalidParameterValue", "Size must be a positive integer", http.StatusBadRequest)
			return
		}
	}
	snapshotID := r.FormValue("SnapshotId")
	var data []byte
	var snapshotHostPath string
	if snapshotID != "" {
		ec2SettleSnapshot(snapshotID)
		snap, ok := ec2Snapshots.Get(snapshotID)
		if !ok {
			ec2ErrorXML(w, "InvalidSnapshot.NotFound", fmt.Sprintf("The snapshot %q does not exist", snapshotID), http.StatusBadRequest)
			return
		}
		if snap.State != "completed" {
			ec2ErrorXML(w, "IncorrectState", fmt.Sprintf("The snapshot %q is not completed", snapshotID), http.StatusBadRequest)
			return
		}
		if size < snap.VolumeSize {
			size = snap.VolumeSize
		}
		data = append([]byte(nil), snap.VolumeData...)
		snapshotHostPath = snap.HostPath
	}
	volType := r.FormValue("VolumeType")
	if volType == "" {
		volType = "gp3"
	}
	vol := EC2Volume{
		VolumeId:         ec2ID("vol"),
		Size:             size,
		SnapshotId:       snapshotID,
		AvailabilityZone: az,
		State:            "available",
		CreateTime:       time.Now().UTC().Format(time.RFC3339),
		VolumeType:       volType,
		Encrypted:        r.FormValue("Encrypted") == "true",
		Tags:             parseTags(r),
		Data:             data,
	}
	if err := ebsPrepareVolumeHostPath(&vol); err != nil {
		ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not create volume data path: %v", err), http.StatusInternalServerError)
		return
	}
	if snapshotHostPath != "" {
		if err := ebsCopyDir(vol.HostPath, snapshotHostPath); err != nil {
			ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not restore snapshot data: %v", err), http.StatusInternalServerError)
			return
		}
	}
	ec2Volumes.Put(vol.VolumeId, vol)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateVolumeResponse %s><requestId>%s</requestId>%s</CreateVolumeResponse>`,
		ec2Xmlns(), generateUUID(), ec2VolumeFieldsXML(vol))
}

func handleAttachVolume(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	instanceID := r.FormValue("InstanceId")
	device := r.FormValue("Device")
	if device == "" {
		device = "/dev/sdf"
	}
	vol, ok := ec2Volumes.Get(volID)
	if !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	if _, ok := ec2Instances.Get(instanceID); !ok {
		ec2ErrorXML(w, "InvalidInstanceID.NotFound", fmt.Sprintf("The instance ID %q does not exist", instanceID), http.StatusBadRequest)
		return
	}
	if len(vol.Attachments) > 0 {
		ec2ErrorXML(w, "IncorrectState", "Volume is already attached", http.StatusBadRequest)
		return
	}
	if _, err := ebsEnsureVolumeBlockImage(&vol); err != nil {
		ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not prepare volume block image: %v", err), http.StatusInternalServerError)
		return
	}
	// Real block-device attach only on a real-execution host (the volume binds
	// to the instance's Firecracker VM); modeled at the control plane otherwise
	if ec2RealVMHostAvailable() {
		if err := ec2AttachRealVolume(r.Context(), instanceID, &vol); err != nil {
			ec2ErrorXML(w, "IncorrectInstanceState", fmt.Sprintf("failed to attach real EBS volume: %v", err), http.StatusServiceUnavailable)
			return
		}
	}
	att := EC2VolumeAttachment{
		VolumeId:   volID,
		InstanceId: instanceID,
		Device:     device,
		State:      "attached",
		AttachTime: time.Now().UTC().Format(time.RFC3339),
	}
	vol.State = "in-use"
	vol.Attachments = []EC2VolumeAttachment{att}
	ec2Volumes.Put(volID, vol)
	ec2AttachmentResponse(w, "AttachVolume", att)
}

func handleDetachVolume(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	vol, ok := ec2Volumes.Get(volID)
	if !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	if len(vol.Attachments) == 0 {
		ec2ErrorXML(w, "IncorrectState", "Volume is not attached", http.StatusBadRequest)
		return
	}
	att := vol.Attachments[0]
	if ec2RealVMHostAvailable() {
		if err := ec2DetachRealVolume(r.Context(), att.InstanceId, volID); err != nil {
			ec2ErrorXML(w, "IncorrectInstanceState", fmt.Sprintf("failed to detach real EBS volume: %v", err), http.StatusServiceUnavailable)
			return
		}
	}
	att.State = "detached"
	vol.Attachments = nil
	vol.State = "available"
	ec2Volumes.Put(volID, vol)
	ec2AttachmentResponse(w, "DetachVolume", att)
}

func ec2AttachmentResponse(w http.ResponseWriter, action string, att EC2VolumeAttachment) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<%sResponse %s><requestId>%s</requestId><volumeId>%s</volumeId><instanceId>%s</instanceId><device>%s</device><status>%s</status><attachTime>%s</attachTime></%sResponse>`,
		action, ec2Xmlns(), generateUUID(), att.VolumeId, att.InstanceId, att.Device, att.State, att.AttachTime, action)
}

func handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	vol, ok := ec2Volumes.Get(volID)
	if !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	if len(vol.Attachments) > 0 {
		ec2ErrorXML(w, "VolumeInUse", "Volume is in-use", http.StatusBadRequest)
		return
	}
	if vol.DockerVolumeName != "" {
		ebsRemoveDockerVolume(vol.DockerVolumeName)
	} else {
		_ = os.RemoveAll(vol.HostPath)
	}
	ec2Volumes.Delete(volID)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteVolumeResponse %s><requestId>%s</requestId><return>true</return></DeleteVolumeResponse>`, ec2Xmlns(), generateUUID())
}

func handleModifyVolume(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	vol, ok := ec2Volumes.Get(volID)
	if !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	if v := r.FormValue("Size"); v != "" {
		var size int
		if _, err := fmt.Sscanf(v, "%d", &size); err != nil || size < vol.Size {
			ec2ErrorXML(w, "InvalidParameterValue", "Size must be an integer greater than or equal to the current volume size", http.StatusBadRequest)
			return
		}
		vol.Size = size
	}
	if v := r.FormValue("VolumeType"); v != "" {
		vol.VolumeType = v
	}
	if len(vol.Attachments) > 0 && ec2RealVMHostAvailable() {
		if _, err := ebsEnsureVolumeBlockImage(&vol); err != nil {
			ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not resize volume block image: %v", err), http.StatusInternalServerError)
			return
		}
		if err := ec2RefreshRealVolume(r.Context(), vol); err != nil {
			ec2ErrorXML(w, "IncorrectInstanceState", fmt.Sprintf("failed to refresh real EBS volume: %v", err), http.StatusServiceUnavailable)
			return
		}
	}
	ec2Volumes.Put(volID, vol)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<ModifyVolumeResponse %s><requestId>%s</requestId><volumeModification><volumeId>%s</volumeId><modificationState>completed</modificationState><targetSize>%d</targetSize><targetVolumeType>%s</targetVolumeType><originalSize>%d</originalSize><originalVolumeType>%s</originalVolumeType><progress>100</progress><startTime>%s</startTime></volumeModification></ModifyVolumeResponse>`,
		ec2Xmlns(), generateUUID(), vol.VolumeId, vol.Size, vol.VolumeType, vol.Size, vol.VolumeType, time.Now().UTC().Format(time.RFC3339))
}

func handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	volID := r.FormValue("VolumeId")
	vol, ok := ec2Volumes.Get(volID)
	if !ok {
		ec2ErrorXML(w, "InvalidVolume.NotFound", fmt.Sprintf("The volume %q does not exist", volID), http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	snap := EC2Snapshot{
		SnapshotId:    ec2ID("snap"),
		VolumeId:      volID,
		VolumeSize:    vol.Size,
		State:         "pending",
		StartTime:     now.Format(time.RFC3339),
		CompletionDue: now.Add(100 * time.Millisecond).Format(time.RFC3339Nano),
		Progress:      "0%",
		Description:   r.FormValue("Description"),
		OwnerId:       ec2Owner(),
		Tags:          parseTags(r),
		VolumeData:    append([]byte(nil), vol.Data...),
	}
	if vol.DockerVolumeName != "" {
		snap.DockerVolumeName = ebsSnapshotDockerVolumeName(snap.SnapshotId)
		if err := ebsCopyDockerVolumes(r.Context(), vol.DockerVolumeName, snap.DockerVolumeName); err != nil {
			ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not snapshot volume data: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		snap.HostPath = ebsSnapshotHostDirPath(snap.SnapshotId)
		if err := ebsPrepareVolumeHostPath(&vol); err != nil {
			ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not access volume data path: %v", err), http.StatusInternalServerError)
			return
		}
		if err := ebsCopyDir(snap.HostPath, vol.HostPath); err != nil {
			ec2ErrorXML(w, "InternalError", fmt.Sprintf("could not snapshot volume data: %v", err), http.StatusInternalServerError)
			return
		}
	}
	ec2Volumes.Put(vol.VolumeId, vol)
	ec2Snapshots.Put(snap.SnapshotId, snap)
	go ec2TransitionSnapshotToCompleted(snap.SnapshotId)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateSnapshotResponse %s><requestId>%s</requestId>%s</CreateSnapshotResponse>`,
		ec2Xmlns(), generateUUID(), ec2SnapshotFieldsXML(snap))
}

func ec2TransitionSnapshotToCompleted(snapshotID string) {
	time.Sleep(100 * time.Millisecond)
	ec2SettleSnapshot(snapshotID)
}

func ec2SettleSnapshot(snapshotID string) {
	ec2Snapshots.Update(snapshotID, func(snap *EC2Snapshot) {
		if snap.State == "pending" && !ec2SnapshotCompletionDue(*snap).After(time.Now().UTC()) {
			snap.State = "completed"
			snap.Progress = "100%"
		}
	})
}

func ec2SnapshotCompletionDue(snap EC2Snapshot) time.Time {
	if snap.CompletionDue != "" {
		if t, err := time.Parse(time.RFC3339Nano, snap.CompletionDue); err == nil {
			return t
		}
	}
	if t, err := time.Parse(time.RFC3339, snap.StartTime); err == nil {
		return t.Add(100 * time.Millisecond)
	}
	return time.Time{}
}

func handleDescribeSnapshots(w http.ResponseWriter, r *http.Request) {
	ids := ec2ParamList(r, "SnapshotId")
	snapshots := make([]EC2Snapshot, 0)
	if len(ids) > 0 {
		for _, id := range ids {
			ec2SettleSnapshot(id)
			snap, ok := ec2Snapshots.Get(id)
			if !ok {
				ec2ErrorXML(w, "InvalidSnapshot.NotFound", fmt.Sprintf("The snapshot %q does not exist", id), http.StatusBadRequest)
				return
			}
			snapshots = append(snapshots, snap)
		}
	} else {
		for _, snap := range ec2Snapshots.List() {
			ec2SettleSnapshot(snap.SnapshotId)
		}
		snapshots = ec2Snapshots.List()
	}
	var items strings.Builder
	for _, snap := range snapshots {
		items.WriteString(ec2SnapshotXML(snap))
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeSnapshotsResponse %s><requestId>%s</requestId><snapshotSet>%s</snapshotSet></DescribeSnapshotsResponse>`,
		ec2Xmlns(), generateUUID(), items.String())
}

func ec2SnapshotXML(snap EC2Snapshot) string {
	return "<item>" + ec2SnapshotFieldsXML(snap) + "</item>"
}

func ec2SnapshotFieldsXML(snap EC2Snapshot) string {
	return fmt.Sprintf(`<snapshotId>%s</snapshotId><volumeId>%s</volumeId><status>%s</status><startTime>%s</startTime><progress>%s</progress><ownerId>%s</ownerId><volumeSize>%d</volumeSize><description>%s</description>%s`,
		snap.SnapshotId, snap.VolumeId, snap.State, snap.StartTime, snap.Progress, snap.OwnerId, snap.VolumeSize, xmlEscape(snap.Description), writeTagSetXML(snap.Tags))
}

func handleDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	snapID := r.FormValue("SnapshotId")
	snap, ok := ec2Snapshots.Get(snapID)
	if !ok {
		ec2ErrorXML(w, "InvalidSnapshot.NotFound", fmt.Sprintf("The snapshot %q does not exist", snapID), http.StatusBadRequest)
		return
	}
	ec2Snapshots.Delete(snapID)
	if snap.DockerVolumeName != "" {
		ebsRemoveDockerVolume(snap.DockerVolumeName)
	} else {
		_ = os.RemoveAll(snap.HostPath)
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DeleteSnapshotResponse %s><requestId>%s</requestId><return>true</return></DeleteSnapshotResponse>`, ec2Xmlns(), generateUUID())
}

func parseIndexedTags(r *http.Request, prefix string) []EC2Tag {
	var tags []EC2Tag
	for i := 1; ; i++ {
		key := r.FormValue(fmt.Sprintf("%s.%d.Key", prefix, i))
		if key == "" {
			break
		}
		tags = append(tags, EC2Tag{Key: key, Value: r.FormValue(fmt.Sprintf("%s.%d.Value", prefix, i))})
	}
	return tags
}

func mergeEC2Tags(existing, updates []EC2Tag) []EC2Tag {
	byKey := map[string]string{}
	for _, t := range existing {
		byKey[t.Key] = t.Value
	}
	for _, t := range updates {
		byKey[t.Key] = t.Value
	}
	var out []EC2Tag
	for key, value := range byKey {
		out = append(out, EC2Tag{Key: key, Value: value})
	}
	return out
}

func handleDescribeImages(w http.ResponseWriter, r *http.Request) {
	imageIDs := ec2ParamList(r, "ImageId")
	if len(imageIDs) == 0 {
		imageIDs = []string{"ami-simulated"}
	}
	var items strings.Builder
	for _, id := range imageIDs {
		fmt.Fprintf(&items, `<item><imageId>%s</imageId><imageLocation>%s</imageLocation><imageState>available</imageState><imageOwnerId>%s</imageOwnerId><isPublic>true</isPublic><architecture>x86_64</architecture><imageType>machine</imageType><rootDeviceType>ebs</rootDeviceType><rootDeviceName>/dev/sda1</rootDeviceName><blockDeviceMapping><item><deviceName>/dev/sda1</deviceName><ebs><snapshotId>snap-%s</snapshotId><volumeSize>8</volumeSize><deleteOnTermination>true</deleteOnTermination><volumeType>gp3</volumeType></ebs></item></blockDeviceMapping><virtualizationType>hvm</virtualizationType><name>%s</name><hypervisor>xen</hypervisor></item>`,
			id, id, ec2Owner(), strings.TrimPrefix(id, "ami-"), id)
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeImagesResponse %s><requestId>%s</requestId><imagesSet>%s</imagesSet></DescribeImagesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDescribeInstanceTypes(w http.ResponseWriter, r *http.Request) {
	types := ec2ParamList(r, "InstanceType")
	if len(types) == 0 {
		types = []string{"t3.micro", "t3.small", "m6i.large"}
	}
	var items strings.Builder
	for _, name := range types {
		fmt.Fprintf(&items, `<item><instanceType>%s</instanceType><currentGeneration>true</currentGeneration><freeTierEligible>%t</freeTierEligible><supportedUsageClasses><item>on-demand</item><item>spot</item></supportedUsageClasses><supportedRootDeviceTypes><item>ebs</item></supportedRootDeviceTypes><supportedVirtualizationTypes><item>hvm</item></supportedVirtualizationTypes><vcpuInfo><defaultVCpus>2</defaultVCpus><defaultCores>1</defaultCores><defaultThreadsPerCore>2</defaultThreadsPerCore></vcpuInfo><memoryInfo><sizeInMiB>1024</sizeInMiB></memoryInfo><processorInfo><supportedArchitectures><item>x86_64</item></supportedArchitectures></processorInfo><networkInfo><networkPerformance>Up to 5 Gigabit</networkPerformance><maximumNetworkInterfaces>2</maximumNetworkInterfaces><ipv4AddressesPerInterface>2</ipv4AddressesPerInterface></networkInfo><ebsInfo><ebsOptimizedSupport>default</ebsOptimizedSupport><encryptionSupport>supported</encryptionSupport></ebsInfo></item>`,
			name, name == "t3.micro")
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeInstanceTypesResponse %s>
  <requestId>%s</requestId>
  <instanceTypeSet>%s</instanceTypeSet>
</DescribeInstanceTypesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

func handleDescribeKeyPairs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeKeyPairsResponse %s><requestId>%s</requestId><keySet/></DescribeKeyPairsResponse>`, ec2Xmlns(), generateUUID())
}

// handleDescribeInstanceTypeOfferings answers "is this instance type offered in
// these locations?" — the fck-nat module's pre-flight AZ validation. Like
// handleDescribeInstanceTypes, the API-only sim does not model real per-AZ
// capacity: it reports each requested instance type as offered in each
// requested (or default) location. Filters honoured: `instance-type` and
// `location`; LocationType selects region / availability-zone / -id scope.
func handleDescribeInstanceTypeOfferings(w http.ResponseWriter, r *http.Request) {
	locationType := r.FormValue("LocationType")
	if locationType == "" {
		locationType = "region"
	}
	filters := ec2Filters(r)
	types := filters["instance-type"]
	if len(types) == 0 {
		types = []string{"t3.micro", "t3.small", "t4g.nano", "m6i.large"}
	}
	locations := filters["location"]
	if len(locations) == 0 {
		switch locationType {
		case "availability-zone":
			locations = []string{awsAvailabilityZone()}
		case "availability-zone-id":
			locations = []string{awsRegion() + "-az1"}
		default: // region
			locations = []string{awsRegion()}
		}
	}
	var items strings.Builder
	for _, t := range types {
		for _, loc := range locations {
			fmt.Fprintf(&items, `<item><instanceType>%s</instanceType><location>%s</location><locationType>%s</locationType></item>`,
				xmlEscape(t), xmlEscape(loc), xmlEscape(locationType))
		}
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeInstanceTypeOfferingsResponse %s>
  <requestId>%s</requestId>
  <instanceTypeOfferingSet>%s</instanceTypeOfferingSet>
</DescribeInstanceTypeOfferingsResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

// ---- Network Interfaces ----

func handleDescribeNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	ids := ec2ParamList(r, "NetworkInterfaceId")
	var enis []EC2NetworkInterface
	if len(ids) > 0 {
		for _, id := range ids {
			eni, ok := ec2NetworkInterfaces.Get(id)
			if !ok {
				sim.AWSErrorf(w, "InvalidNetworkInterfaceID.NotFound", http.StatusBadRequest, "The networkInterface ID %q does not exist", id)
				return
			}
			enis = append(enis, eni)
		}
	} else {
		enis = ec2NetworkInterfaces.List()
	}
	var items strings.Builder
	for _, eni := range enis {
		items.WriteString("<item>" + eniFieldsXML(eni) + "</item>")
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<DescribeNetworkInterfacesResponse %s>
  <requestId>%s</requestId>
  <networkInterfaceSet>%s</networkInterfaceSet>
</DescribeNetworkInterfacesResponse>`, ec2Xmlns(), generateUUID(), items.String())
}

// ec2WriteSimpleResponse writes the `<NameResponse><requestId/><return>true</return></NameResponse>`
// shape used by EC2 mutation actions that have no payload.
func ec2WriteSimpleResponse(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<%s %s>
  <requestId>%s</requestId><return>true</return>
</%s>`, name, ec2Xmlns(), generateUUID(), name)
}

// eniFieldsXML renders the inner ENI fields (no wrapper element), shared by
// DescribeNetworkInterfaces (wrapped in <item>) and CreateNetworkInterface
// (wrapped in <networkInterface>). The attachment block only appears when the
// ENI is attached, and sourceDestCheck reflects the (modifiable)
// SourceDestDisabled flag.
func eniFieldsXML(eni EC2NetworkInterface) string {
	var groups strings.Builder
	for _, groupID := range eni.SecurityGroupIds {
		name := groupID
		if sg, ok := ec2SecurityGroups.Get(groupID); ok {
			name = sg.GroupName
		}
		fmt.Fprintf(&groups, "<item><groupId>%s</groupId><groupName>%s</groupName></item>", groupID, name)
	}
	status := eni.Status
	if status == "" {
		status = "available"
	}
	sourceDest := "true"
	if eni.SourceDestDisabled {
		sourceDest = "false"
	}
	var privateIPs strings.Builder
	fmt.Fprintf(&privateIPs, "<item><privateIpAddress>%s</privateIpAddress><primary>true</primary></item>", eni.PrivateIpAddress)
	for _, ip := range eni.SecondaryPrivateIps {
		fmt.Fprintf(&privateIPs, "<item><privateIpAddress>%s</privateIpAddress><primary>false</primary></item>", ip)
	}
	attachment := ""
	if eni.AttachmentId != "" {
		attachment = fmt.Sprintf(`<attachment><attachmentId>%s</attachmentId><instanceId>%s</instanceId><deviceIndex>%d</deviceIndex><status>attached</status><deleteOnTermination>%t</deleteOnTermination></attachment>`,
			eni.AttachmentId, eni.InstanceId, eni.DeviceIndex, eni.DeleteOnTermination)
	}
	return fmt.Sprintf(`<networkInterfaceId>%s</networkInterfaceId>
    <subnetId>%s</subnetId>
    <vpcId>%s</vpcId>
    <availabilityZone>%s</availabilityZone>
    <description>%s</description>
    <ownerId>%s</ownerId>
    <requesterManaged>false</requesterManaged>
    <status>%s</status>
    <macAddress>02:00:00:00:00:01</macAddress>
    <privateIpAddress>%s</privateIpAddress>
    <privateDnsName>ip-%s.%s.compute.internal</privateDnsName>
    <sourceDestCheck>%s</sourceDestCheck>
    <groupSet>%s</groupSet>
    <privateIpAddressesSet>%s</privateIpAddressesSet>
    %s
    %s`,
		eni.NetworkInterfaceId, eni.SubnetId, eni.VpcId, awsAvailabilityZone(), eni.Description, eni.OwnerId, status,
		eni.PrivateIpAddress, strings.ReplaceAll(eni.PrivateIpAddress, ".", "-"), awsRegion(), sourceDest, groups.String(),
		privateIPs.String(), attachment, writeTagSetXML(eni.Tags))
}

// handleCreateNetworkInterface materializes a standalone ENI in a subnet
// (status "available", source/dest check on). Control-plane modeling like
// handleCreateNatGateway — no real fabric.
func handleCreateNetworkInterface(w http.ResponseWriter, r *http.Request) {
	subnetID := r.FormValue("SubnetId")
	subnet, ok := ec2Subnets.Get(subnetID)
	if !ok {
		ec2ErrorXML(w, "InvalidSubnetID.NotFound", fmt.Sprintf("The subnet ID %q does not exist", subnetID), http.StatusBadRequest)
		return
	}
	privateIP := r.FormValue("PrivateIpAddress")
	if privateIP == "" {
		ip, err := AllocateSubnetIP(subnetID)
		if err != nil {
			ec2ErrorXML(w, "InsufficientFreeAddressesInSubnet", fmt.Sprintf("failed to allocate ENI private IP: %v", err), http.StatusBadRequest)
			return
		}
		privateIP = ip
	}
	eni := EC2NetworkInterface{
		NetworkInterfaceId: ec2ID("eni"),
		SubnetId:           subnetID,
		VpcId:              subnet.VpcId,
		PrivateIpAddress:   privateIP,
		Status:             "available",
		Description:        r.FormValue("Description"),
		SecurityGroupIds:   ec2ParamList(r, "SecurityGroupId"),
		Tags:               parseTags(r),
		OwnerId:            ec2Owner(),
	}
	ec2NetworkInterfaces.Put(eni.NetworkInterfaceId, eni)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<CreateNetworkInterfaceResponse %s>
  <requestId>%s</requestId>
  <networkInterface>%s</networkInterface>
</CreateNetworkInterfaceResponse>`, ec2Xmlns(), generateUUID(), eniFieldsXML(eni))
}

func handleAttachNetworkInterface(w http.ResponseWriter, r *http.Request) {
	eniID := r.FormValue("NetworkInterfaceId")
	eni, ok := ec2NetworkInterfaces.Get(eniID)
	if !ok {
		ec2ErrorXML(w, "InvalidNetworkInterfaceID.NotFound", fmt.Sprintf("The networkInterface ID %q does not exist", eniID), http.StatusBadRequest)
		return
	}
	if eni.AttachmentId != "" {
		ec2ErrorXML(w, "InvalidNetworkInterface.InUse", fmt.Sprintf("Interface %q is already attached", eniID), http.StatusBadRequest)
		return
	}
	attachID := ec2ID("eni-attach")
	eni.AttachmentId = attachID
	eni.InstanceId = r.FormValue("InstanceId")
	if di := r.FormValue("DeviceIndex"); di != "" {
		fmt.Sscanf(di, "%d", &eni.DeviceIndex)
	}
	eni.Status = "in-use"
	ec2NetworkInterfaces.Put(eniID, eni)
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AttachNetworkInterfaceResponse %s>
  <requestId>%s</requestId>
  <attachmentId>%s</attachmentId>
</AttachNetworkInterfaceResponse>`, ec2Xmlns(), generateUUID(), attachID)
}

func handleDetachNetworkInterface(w http.ResponseWriter, r *http.Request) {
	attachID := r.FormValue("AttachmentId")
	for _, eni := range ec2NetworkInterfaces.List() {
		if eni.AttachmentId == attachID {
			eni.AttachmentId = ""
			eni.InstanceId = ""
			eni.DeviceIndex = 0
			eni.Status = "available"
			ec2NetworkInterfaces.Put(eni.NetworkInterfaceId, eni)
			ec2WriteSimpleResponse(w, "DetachNetworkInterfaceResponse")
			return
		}
	}
	ec2ErrorXML(w, "InvalidAttachmentID.NotFound", fmt.Sprintf("The attachment ID %q does not exist", attachID), http.StatusBadRequest)
}

func handleDeleteNetworkInterface(w http.ResponseWriter, r *http.Request) {
	eniID := r.FormValue("NetworkInterfaceId")
	eni, ok := ec2NetworkInterfaces.Get(eniID)
	if !ok {
		ec2ErrorXML(w, "InvalidNetworkInterfaceID.NotFound", fmt.Sprintf("The networkInterface ID %q does not exist", eniID), http.StatusBadRequest)
		return
	}
	if eni.AttachmentId != "" {
		ec2ErrorXML(w, "InvalidParameterValue", fmt.Sprintf("Network interface %q is currently in use", eniID), http.StatusBadRequest)
		return
	}
	ec2NetworkInterfaces.Delete(eniID)
	ec2WriteSimpleResponse(w, "DeleteNetworkInterfaceResponse")
}

func handleModifyNetworkInterfaceAttribute(w http.ResponseWriter, r *http.Request) {
	eniID := r.FormValue("NetworkInterfaceId")
	eni, ok := ec2NetworkInterfaces.Get(eniID)
	if !ok {
		ec2ErrorXML(w, "InvalidNetworkInterfaceID.NotFound", fmt.Sprintf("The networkInterface ID %q does not exist", eniID), http.StatusBadRequest)
		return
	}
	if v := r.FormValue("SourceDestCheck.Value"); v != "" {
		eni.SourceDestDisabled = v == "false"
	}
	if v := r.FormValue("Description.Value"); v != "" {
		eni.Description = v
	}
	if groups := ec2ParamList(r, "SecurityGroupId"); len(groups) > 0 {
		eni.SecurityGroupIds = groups
	}
	ec2NetworkInterfaces.Put(eniID, eni)
	ec2WriteSimpleResponse(w, "ModifyNetworkInterfaceAttributeResponse")
}

func handleAssignPrivateIpAddresses(w http.ResponseWriter, r *http.Request) {
	eniID := r.FormValue("NetworkInterfaceId")
	eni, ok := ec2NetworkInterfaces.Get(eniID)
	if !ok {
		ec2ErrorXML(w, "InvalidNetworkInterfaceID.NotFound", fmt.Sprintf("The networkInterface ID %q does not exist", eniID), http.StatusBadRequest)
		return
	}
	assigned := ec2ParamList(r, "PrivateIpAddress")
	if n := r.FormValue("SecondaryPrivateIpAddressCount"); n != "" {
		var count int
		fmt.Sscanf(n, "%d", &count)
		for i := 0; i < count; i++ {
			ip, err := AllocateSubnetIP(eni.SubnetId)
			if err != nil {
				break
			}
			assigned = append(assigned, ip)
		}
	}
	eni.SecondaryPrivateIps = append(eni.SecondaryPrivateIps, assigned...)
	ec2NetworkInterfaces.Put(eniID, eni)
	var ipItems strings.Builder
	for _, ip := range assigned {
		fmt.Fprintf(&ipItems, "<item><privateIpAddress>%s</privateIpAddress></item>", ip)
	}
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<AssignPrivateIpAddressesResponse %s>
  <requestId>%s</requestId>
  <networkInterfaceId>%s</networkInterfaceId>
  <assignedPrivateIpAddressesSet>%s</assignedPrivateIpAddressesSet>
</AssignPrivateIpAddressesResponse>`, ec2Xmlns(), generateUUID(), eniID, ipItems.String())
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
