package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

type ASLaunchConfiguration struct {
	Name         string
	ImageId      string
	InstanceType string
	KeyName      string
}

type AutoScalingGroup struct {
	Name                    string
	LaunchConfigurationName string
	MinSize                 int
	MaxSize                 int
	DesiredCapacity         int
	VPCZoneIdentifier       string
	InstanceIds             []string
	CreatedTime             string
	Tags                    []EC2Tag
}

type ScalingActivity struct {
	ActivityId           string
	AutoScalingGroupName string
	Description          string
	Cause                string
	StartTime            string
	EndTime              string
	StatusCode           string
}

var (
	asLaunchConfigurations sim.Store[ASLaunchConfiguration]
	autoScalingGroups      sim.Store[AutoScalingGroup]
	scalingActivities      sim.Store[ScalingActivity]
)

func registerAutoScaling(r *sim.AWSQueryRouter, srv *sim.Server) {
	asLaunchConfigurations = sim.MakeStore[ASLaunchConfiguration](srv.DB(), "autoscaling_launch_configurations")
	autoScalingGroups = sim.MakeStore[AutoScalingGroup](srv.DB(), "autoscaling_groups")
	scalingActivities = sim.MakeStore[ScalingActivity](srv.DB(), "autoscaling_activities")

	r.RegisterVersioned("2011-01-01", "CreateLaunchConfiguration", handleASCreateLaunchConfiguration)
	r.RegisterVersioned("2011-01-01", "DescribeLaunchConfigurations", handleASDescribeLaunchConfigurations)
	r.RegisterVersioned("2011-01-01", "DeleteLaunchConfiguration", handleASDeleteLaunchConfiguration)
	r.RegisterVersioned("2011-01-01", "CreateAutoScalingGroup", handleASCreateAutoScalingGroup)
	r.RegisterVersioned("2011-01-01", "DescribeAutoScalingGroups", handleASDescribeAutoScalingGroups)
	r.RegisterVersioned("2011-01-01", "UpdateAutoScalingGroup", handleASUpdateAutoScalingGroup)
	r.RegisterVersioned("2011-01-01", "SetDesiredCapacity", handleASSetDesiredCapacity)
	r.RegisterVersioned("2011-01-01", "DescribeScalingActivities", handleASDescribeScalingActivities)
	r.RegisterVersioned("2011-01-01", "CreateOrUpdateTags", handleASCreateOrUpdateTags)
	r.RegisterVersioned("2011-01-01", "DeleteTags", handleASDeleteTags)
	r.RegisterVersioned("2011-01-01", "DescribeTags", handleASDescribeTags)
	r.RegisterVersioned("2011-01-01", "DeleteAutoScalingGroup", handleASDeleteAutoScalingGroup)
}

func handleASCreateLaunchConfiguration(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("LaunchConfigurationName")
	if name == "" {
		asError(w, "ValidationError", "LaunchConfigurationName is required", http.StatusBadRequest)
		return
	}
	if _, exists := asLaunchConfigurations.Get(name); exists {
		asError(w, "AlreadyExists", "LaunchConfiguration already exists", http.StatusBadRequest)
		return
	}
	lc := ASLaunchConfiguration{
		Name:         name,
		ImageId:      firstNonEmpty(r.FormValue("ImageId"), "ami-simulated"),
		InstanceType: firstNonEmpty(r.FormValue("InstanceType"), "t3.micro"),
		KeyName:      r.FormValue("KeyName"),
	}
	asLaunchConfigurations.Put(name, lc)
	asEmptyResponse(w, "CreateLaunchConfiguration")
}

func handleASDescribeLaunchConfigurations(w http.ResponseWriter, r *http.Request) {
	names := autoscalingParamList(r, "LaunchConfigurationNames.member")
	configs := make([]ASLaunchConfiguration, 0)
	if len(names) > 0 {
		for _, name := range names {
			if lc, ok := asLaunchConfigurations.Get(name); ok {
				configs = append(configs, lc)
			}
		}
	} else {
		configs = asLaunchConfigurations.List()
	}
	var items strings.Builder
	for _, lc := range configs {
		fmt.Fprintf(&items, `<member><LaunchConfigurationName>%s</LaunchConfigurationName><ImageId>%s</ImageId><InstanceType>%s</InstanceType><KeyName>%s</KeyName></member>`,
			xmlEscape(lc.Name), xmlEscape(lc.ImageId), xmlEscape(lc.InstanceType), xmlEscape(lc.KeyName))
	}
	asResponse(w, "DescribeLaunchConfigurations", fmt.Sprintf("<LaunchConfigurations>%s</LaunchConfigurations>", items.String()))
}

func handleASDeleteLaunchConfiguration(w http.ResponseWriter, r *http.Request) {
	asLaunchConfigurations.Delete(r.FormValue("LaunchConfigurationName"))
	asEmptyResponse(w, "DeleteLaunchConfiguration")
}

func handleASCreateAutoScalingGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("AutoScalingGroupName")
	if name == "" {
		asError(w, "ValidationError", "AutoScalingGroupName is required", http.StatusBadRequest)
		return
	}
	if _, exists := autoScalingGroups.Get(name); exists {
		asError(w, "AlreadyExists", "AutoScalingGroup already exists", http.StatusBadRequest)
		return
	}
	minSize := asAtoiDefault(r.FormValue("MinSize"), 0)
	maxSize := asAtoiDefault(r.FormValue("MaxSize"), minSize)
	desired := asAtoiDefault(r.FormValue("DesiredCapacity"), minSize)
	if desired > maxSize {
		maxSize = desired
	}
	asg := AutoScalingGroup{
		Name:                    name,
		LaunchConfigurationName: r.FormValue("LaunchConfigurationName"),
		MinSize:                 minSize,
		MaxSize:                 maxSize,
		DesiredCapacity:         desired,
		VPCZoneIdentifier:       r.FormValue("VPCZoneIdentifier"),
		CreatedTime:             time.Now().UTC().Format(time.RFC3339),
		Tags:                    autoscalingTags(r),
	}
	if err := reconcileAutoScalingGroup(&asg, "Created Auto Scaling group"); err != nil {
		asError(w, "ValidationError", err.Error(), http.StatusBadRequest)
		return
	}
	autoScalingGroups.Put(name, asg)
	asEmptyResponse(w, "CreateAutoScalingGroup")
}

func handleASDescribeAutoScalingGroups(w http.ResponseWriter, r *http.Request) {
	names := autoscalingParamList(r, "AutoScalingGroupNames.member")
	groups := make([]AutoScalingGroup, 0)
	if len(names) > 0 {
		for _, name := range names {
			if asg, ok := autoScalingGroups.Get(name); ok {
				groups = append(groups, asg)
			}
		}
	} else {
		groups = autoScalingGroups.List()
	}
	var items strings.Builder
	for _, asg := range groups {
		items.WriteString(autoScalingGroupXML(asg))
	}
	asResponse(w, "DescribeAutoScalingGroups", fmt.Sprintf("<AutoScalingGroups>%s</AutoScalingGroups>", items.String()))
}

func handleASUpdateAutoScalingGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("AutoScalingGroupName")
	asg, ok := autoScalingGroups.Get(name)
	if !ok {
		asError(w, "ValidationError", "AutoScalingGroup not found", http.StatusBadRequest)
		return
	}
	if v := r.FormValue("LaunchConfigurationName"); v != "" {
		asg.LaunchConfigurationName = v
	}
	if v := r.FormValue("MinSize"); v != "" {
		asg.MinSize = asAtoiDefault(v, asg.MinSize)
	}
	if v := r.FormValue("MaxSize"); v != "" {
		asg.MaxSize = asAtoiDefault(v, asg.MaxSize)
	}
	if v := r.FormValue("DesiredCapacity"); v != "" {
		asg.DesiredCapacity = asAtoiDefault(v, asg.DesiredCapacity)
	}
	if v := r.FormValue("VPCZoneIdentifier"); v != "" {
		asg.VPCZoneIdentifier = v
	}
	if err := reconcileAutoScalingGroup(&asg, "Updated Auto Scaling group"); err != nil {
		asError(w, "ValidationError", err.Error(), http.StatusBadRequest)
		return
	}
	autoScalingGroups.Put(name, asg)
	asEmptyResponse(w, "UpdateAutoScalingGroup")
}

func handleASSetDesiredCapacity(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("AutoScalingGroupName")
	asg, ok := autoScalingGroups.Get(name)
	if !ok {
		asError(w, "ValidationError", "AutoScalingGroup not found", http.StatusBadRequest)
		return
	}
	asg.DesiredCapacity = asAtoiDefault(r.FormValue("DesiredCapacity"), asg.DesiredCapacity)
	if asg.DesiredCapacity > asg.MaxSize {
		asg.MaxSize = asg.DesiredCapacity
	}
	if err := reconcileAutoScalingGroup(&asg, "Set desired capacity"); err != nil {
		asError(w, "ValidationError", err.Error(), http.StatusBadRequest)
		return
	}
	autoScalingGroups.Put(name, asg)
	asEmptyResponse(w, "SetDesiredCapacity")
}

func handleASDescribeScalingActivities(w http.ResponseWriter, r *http.Request) {
	groupName := r.FormValue("AutoScalingGroupName")
	var items strings.Builder
	for _, activity := range scalingActivities.List() {
		if groupName != "" && activity.AutoScalingGroupName != groupName {
			continue
		}
		fmt.Fprintf(&items, `<member><ActivityId>%s</ActivityId><AutoScalingGroupName>%s</AutoScalingGroupName><Description>%s</Description><Cause>%s</Cause><StartTime>%s</StartTime><EndTime>%s</EndTime><StatusCode>%s</StatusCode></member>`,
			activity.ActivityId, xmlEscape(activity.AutoScalingGroupName), xmlEscape(activity.Description), xmlEscape(activity.Cause), activity.StartTime, activity.EndTime, activity.StatusCode)
	}
	asResponse(w, "DescribeScalingActivities", fmt.Sprintf("<Activities>%s</Activities>", items.String()))
}

func handleASDeleteAutoScalingGroup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("AutoScalingGroupName")
	asg, ok := autoScalingGroups.Get(name)
	if !ok {
		asEmptyResponse(w, "DeleteAutoScalingGroup")
		return
	}
	asg.DesiredCapacity = 0
	_ = reconcileAutoScalingGroup(&asg, "Deleted Auto Scaling group")
	autoScalingGroups.Delete(name)
	asEmptyResponse(w, "DeleteAutoScalingGroup")
}

func handleASCreateOrUpdateTags(w http.ResponseWriter, r *http.Request) {
	for i := 1; ; i++ {
		resourceID := r.FormValue(fmt.Sprintf("Tags.member.%d.ResourceId", i))
		if resourceID == "" {
			break
		}
		asg, ok := autoScalingGroups.Get(resourceID)
		if !ok {
			continue
		}
		key := r.FormValue(fmt.Sprintf("Tags.member.%d.Key", i))
		if key == "" {
			continue
		}
		value := r.FormValue(fmt.Sprintf("Tags.member.%d.Value", i))
		found := false
		for j := range asg.Tags {
			if asg.Tags[j].Key == key {
				asg.Tags[j].Value = value
				found = true
				break
			}
		}
		if !found {
			asg.Tags = append(asg.Tags, EC2Tag{Key: key, Value: value})
		}
		autoScalingGroups.Put(asg.Name, asg)
	}
	asEmptyResponse(w, "CreateOrUpdateTags")
}

func handleASDeleteTags(w http.ResponseWriter, r *http.Request) {
	for i := 1; ; i++ {
		resourceID := r.FormValue(fmt.Sprintf("Tags.member.%d.ResourceId", i))
		if resourceID == "" {
			break
		}
		asg, ok := autoScalingGroups.Get(resourceID)
		if !ok {
			continue
		}
		key := r.FormValue(fmt.Sprintf("Tags.member.%d.Key", i))
		keep := asg.Tags[:0]
		for _, tag := range asg.Tags {
			if tag.Key != key {
				keep = append(keep, tag)
			}
		}
		asg.Tags = keep
		autoScalingGroups.Put(asg.Name, asg)
	}
	asEmptyResponse(w, "DeleteTags")
}

func handleASDescribeTags(w http.ResponseWriter, r *http.Request) {
	var items strings.Builder
	for _, asg := range autoScalingGroups.List() {
		for _, tag := range asg.Tags {
			fmt.Fprintf(&items, `<member><ResourceId>%s</ResourceId><ResourceType>auto-scaling-group</ResourceType><Key>%s</Key><Value>%s</Value><PropagateAtLaunch>true</PropagateAtLaunch></member>`,
				xmlEscape(asg.Name), xmlEscape(tag.Key), xmlEscape(tag.Value))
		}
	}
	asResponse(w, "DescribeTags", fmt.Sprintf("<Tags>%s</Tags>", items.String()))
}

func reconcileAutoScalingGroup(asg *AutoScalingGroup, cause string) error {
	lc, ok := asLaunchConfigurations.Get(asg.LaunchConfigurationName)
	if !ok {
		return fmt.Errorf("LaunchConfiguration %q not found", asg.LaunchConfigurationName)
	}
	subnetID := strings.TrimSpace(strings.Split(asg.VPCZoneIdentifier, ",")[0])
	if subnetID == "" {
		subnetID = "subnet-0123456789abcdef0"
	}
	subnet, ok := ec2Subnets.Get(subnetID)
	if !ok {
		return fmt.Errorf("subnet %q not found", subnetID)
	}
	for len(asg.InstanceIds) < asg.DesiredCapacity {
		ip, err := AllocateSubnetIP(subnetID)
		if err != nil {
			return err
		}
		inst := ec2CreateInstance(EC2InstanceCreateSpec{
			ReservationId:    ec2ID("r"),
			ImageId:          lc.ImageId,
			InstanceType:     lc.InstanceType,
			Subnet:           subnet,
			SubnetId:         subnetID,
			PrivateIP:        ip,
			SecurityGroupIds: nil,
			Tags:             asg.Tags,
			LaunchTime:       time.Now().UTC().Format(time.RFC3339),
			KeyName:          lc.KeyName,
			State:            "running",
		})
		asg.InstanceIds = append(asg.InstanceIds, inst.InstanceId)
	}
	for len(asg.InstanceIds) > asg.DesiredCapacity {
		id := asg.InstanceIds[len(asg.InstanceIds)-1]
		asg.InstanceIds = asg.InstanceIds[:len(asg.InstanceIds)-1]
		if inst, ok := ec2Instances.Get(id); ok {
			inst.State = "terminated"
			ec2Instances.Put(id, inst)
			if inst.NetworkInterfaceId != "" {
				ec2NetworkInterfaces.Delete(inst.NetworkInterfaceId)
			}
			ec2DeleteOnTerminationVolumes(id)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	activity := ScalingActivity{
		ActivityId:           generateUUID(),
		AutoScalingGroupName: asg.Name,
		Description:          fmt.Sprintf("%s to %d instances", cause, asg.DesiredCapacity),
		Cause:                cause,
		StartTime:            now,
		EndTime:              now,
		StatusCode:           "Successful",
	}
	scalingActivities.Put(activity.ActivityId, activity)
	return nil
}

func autoScalingGroupXML(asg AutoScalingGroup) string {
	var instances strings.Builder
	for _, id := range asg.InstanceIds {
		instances.WriteString("<member><InstanceId>")
		instances.WriteString(id)
		instances.WriteString("</InstanceId><LifecycleState>InService</LifecycleState><HealthStatus>Healthy</HealthStatus></member>")
	}
	return fmt.Sprintf(`<member><AutoScalingGroupName>%s</AutoScalingGroupName><LaunchConfigurationName>%s</LaunchConfigurationName><MinSize>%d</MinSize><MaxSize>%d</MaxSize><DesiredCapacity>%d</DesiredCapacity><DefaultCooldown>300</DefaultCooldown><AvailabilityZones><member>%s</member></AvailabilityZones><VPCZoneIdentifier>%s</VPCZoneIdentifier><CreatedTime>%s</CreatedTime><Instances>%s</Instances><Tags>%s</Tags></member>`,
		xmlEscape(asg.Name), xmlEscape(asg.LaunchConfigurationName), asg.MinSize, asg.MaxSize, asg.DesiredCapacity, awsAvailabilityZone(), xmlEscape(asg.VPCZoneIdentifier), asg.CreatedTime, instances.String(), autoscalingTagXML(asg.Tags))
}

func autoscalingTagXML(tags []EC2Tag) string {
	var out strings.Builder
	for _, tag := range tags {
		fmt.Fprintf(&out, `<member><Key>%s</Key><Value>%s</Value><PropagateAtLaunch>true</PropagateAtLaunch></member>`, xmlEscape(tag.Key), xmlEscape(tag.Value))
	}
	return out.String()
}

func autoscalingTags(r *http.Request) []EC2Tag {
	var tags []EC2Tag
	for i := 1; ; i++ {
		key := r.FormValue(fmt.Sprintf("Tags.member.%d.Key", i))
		if key == "" {
			break
		}
		tags = append(tags, EC2Tag{Key: key, Value: r.FormValue(fmt.Sprintf("Tags.member.%d.Value", i))})
	}
	return tags
}

func autoscalingParamList(r *http.Request, prefix string) []string {
	var out []string
	for i := 1; ; i++ {
		v := r.FormValue(fmt.Sprintf("%s.%d", prefix, i))
		if v == "" {
			break
		}
		out = append(out, v)
	}
	return out
}

func asAtoiDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func asEmptyResponse(w http.ResponseWriter, action string) {
	asResponse(w, action, "")
}

func asResponse(w http.ResponseWriter, action, body string) {
	w.Header().Set("Content-Type", "text/xml")
	fmt.Fprintf(w, `<%sResponse xmlns="https://autoscaling.amazonaws.com/doc/2011-01-01/"><ResponseMetadata><RequestId>%s</RequestId></ResponseMetadata><%sResult>%s</%sResult></%sResponse>`,
		action, generateUUID(), action, body, action, action)
}

func asError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<ErrorResponse xmlns="https://autoscaling.amazonaws.com/doc/2011-01-01/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>%s</RequestId></ErrorResponse>`,
		code, xmlEscape(message), generateUUID())
}
