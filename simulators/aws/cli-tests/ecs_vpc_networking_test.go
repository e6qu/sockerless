package aws_cli_test

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

const vpcNetBusybox = "public.ecr.aws/docker/library/busybox:latest"

// vpcServerScript runs a long-lived HTTP server serving "ok" on :80, so probes
// can test real TCP reachability/isolation between tasks.
const vpcServerScript = "mkdir -p /www && echo ok > /www/index.html && httpd -f -p 80 -h /www"

// TestECSVPCNetworking proves issue #516 end-to-end and is tier-agnostic (works
// on both the netns and Docker-network fabrics, since it probes via the task
// containers themselves): an ECS task's ENI privateIPv4Address is the
// container's REAL eth0 address, reachable from another task in the same VPC and
// isolated from a task in a different VPC.
func TestECSVPCNetworking(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	q := func(args ...string) string { return strings.TrimSpace(runCLI(t, awsCLI(args...))) }

	vpcA, subnetA := mkVPCSubnet(t, q, "10.91.0.0/16", "10.91.0.0/24")
	_, subnetB := mkVPCSubnet(t, q, "10.92.0.0/16", "10.92.0.0/24")
	q("ecs", "create-cluster", "--cluster-name", "default", "--query", "cluster.clusterName", "--output", "text")
	registerTaskDef(q, "vpc-server", vpcServerScript)
	registerTaskDef(q, "vpc-client", "sleep 120")

	server := runTask(q, "vpc-server", subnetA)
	clientSame := runTask(q, "vpc-client", subnetA)
	clientOther := runTask(q, "vpc-client", subnetB)
	t.Cleanup(func() {
		for _, task := range []string{server, clientSame, clientOther} {
			runCLI(t, awsCLI("ecs", "stop-task", "--cluster", "default", "--task", task))
		}
		rmDockerNetworks(ecsVPCNet(vpcA), ecsVPCNet(vpcA)+"-egress")
	})
	waitRunning(t, q, server)
	waitRunning(t, q, clientSame)
	waitRunning(t, q, clientOther)

	// #516: the reported ENI IP is the container's real eth0 address.
	eniIP := taskENIIP(q, server)
	if !strings.HasPrefix(eniIP, "10.91.0.") {
		t.Fatalf("server ENI IP not in subnet-A CIDR: %q", eniIP)
	}
	if real := taskEth0IP(t, server); real != eniIP {
		t.Fatalf("reported ENI IP %q != container's real eth0 IP %q (#516)", eniIP, real)
	}

	// Intra-VPC reachable; cross-VPC isolated.
	if code, out := taskWget(t, clientSame, eniIP); code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("same-VPC task should reach %s: exit=%d out=%q", eniIP, code, out)
	}
	if code, _ := taskWget(t, clientOther, eniIP); code == 0 {
		t.Fatalf("different-VPC task should be isolated from %s", eniIP)
	}
}

// ---- shared helpers (tier-agnostic) ----

func mkVPCSubnet(t *testing.T, q func(...string) string, vpcCidr, snCidr string) (vpcID, subnetID string) {
	t.Helper()
	vpcID = q("ec2", "create-vpc", "--cidr-block", vpcCidr, "--query", "Vpc.VpcId", "--output", "text")
	subnetID = q("ec2", "create-subnet", "--vpc-id", vpcID, "--cidr-block", snCidr,
		"--availability-zone", "us-east-1a", "--query", "Subnet.SubnetId", "--output", "text")
	return vpcID, subnetID
}

func registerTaskDef(q func(...string) string, family, script string) {
	q("ecs", "register-task-definition", "--family", family,
		"--network-mode", "awsvpc", "--requires-compatibilities", "FARGATE", "--cpu", "256", "--memory", "512",
		"--container-definitions", `[{"name":"app","image":"`+vpcNetBusybox+`","entryPoint":["sh","-c"],"command":["`+script+`"]}]`,
		"--query", "taskDefinition.taskDefinitionArn", "--output", "text")
}

func runTask(q func(...string) string, family, subnet string) string {
	return q("ecs", "run-task", "--cluster", "default", "--task-definition", family,
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnet+`]}`,
		"--query", "tasks[0].taskArn", "--output", "text")
}

func taskENIIP(q func(...string) string, taskArn string) string {
	return q("ecs", "describe-tasks", "--cluster", "default", "--tasks", taskArn,
		"--query", "tasks[0].containers[0].networkInterfaces[0].privateIpv4Address", "--output", "text")
}

func ecsVPCNet(vpcID string) string { return "sockerless-sim-vpc-" + vpcID }

func taskID(taskArn string) string {
	parts := strings.Split(taskArn, "/")
	return parts[len(parts)-1]
}

func taskContainerID(t *testing.T, taskArn string) string {
	t.Helper()
	cid := strings.TrimSpace(dockerOut(t, "ps", "-q", "-f", "label=sockerless-sim-task="+taskID(taskArn)))
	if cid == "" {
		t.Fatalf("no running container for task %s", taskArn)
	}
	return cid
}

// taskEth0IP reads the container's real eth0 IPv4 (works in both tiers).
func taskEth0IP(t *testing.T, taskArn string) string {
	t.Helper()
	cid := taskContainerID(t, taskArn)
	// Retry: the netns veth is plumbed just as the task reaches RUNNING.
	for attempt := 0; attempt < 10; attempt++ {
		out, _ := exec.Command("docker", "exec", cid, "ip", "-4", "-o", "addr", "show", "eth0").CombinedOutput()
		for _, f := range strings.Fields(string(out)) {
			if strings.HasPrefix(f, "inet") {
				continue
			}
			if i := strings.Index(f, "/"); i > 0 && strings.Count(f[:i], ".") == 3 {
				return f[:i]
			}
		}
		time.Sleep(time.Second)
	}
	return ""
}

// taskWget fetches http://ip/index.html from inside a task container.
func taskWget(t *testing.T, taskArn, ip string) (int, string) {
	t.Helper()
	cid := taskContainerID(t, taskArn)
	out, err := exec.Command("docker", "exec", cid, "wget", "-T", "3", "-q", "-O", "-", "http://"+ip+"/index.html").CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), string(out)
		}
		return -1, string(out)
	}
	return 0, string(out)
}

func dockerOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// ecsNetnsTierActive approximates the sim's netns-tier gate (Linux + the network
// tools) so tier-specific tests can opt in.
func ecsNetnsTierActive() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	for _, bin := range []string{"ip", "nft", "nsenter", "sysctl"} {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	return true
}

func waitRunning(t *testing.T, q func(...string) string, taskArn string) {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if q("ecs", "describe-tasks", "--cluster", "default", "--tasks", taskArn,
			"--query", "tasks[0].lastStatus", "--output", "text") == "RUNNING" {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("task %s never reached RUNNING", taskArn)
}

// rmDockerNetworks removes simulator VPC networks (Docker tier), retrying while
// task containers detach. No-op for names that don't exist (netns tier).
func rmDockerNetworks(names ...string) {
	for attempt := 0; attempt < 12; attempt++ {
		pending := false
		for _, n := range names {
			if exec.Command("docker", "network", "inspect", n).Run() != nil {
				continue
			}
			if exec.Command("docker", "network", "rm", n).Run() != nil {
				pending = true
			}
		}
		if !pending {
			return
		}
		time.Sleep(time.Second)
	}
}

// TestECSVPCOverlappingCIDR proves the netns fabric does what Docker bridges
// can't: two VPCs with the SAME AWS CIDR (legal — VPCs are isolated) both run
// tasks that get the SAME real ENI IP, with no remapping and full isolation.
// Netns-tier only (the Docker-network tier can't host overlapping bridges).
func TestECSVPCOverlappingCIDR(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	if !ecsNetnsTierActive() {
		t.Skip("overlapping VPC CIDRs require the netns fabric (Linux + CAP_NET_ADMIN)")
	}
	q := func(args ...string) string { return strings.TrimSpace(runCLI(t, awsCLI(args...))) }

	_, subnetA := mkVPCSubnet(t, q, "10.50.0.0/16", "10.50.0.0/24")
	_, subnetB := mkVPCSubnet(t, q, "10.50.0.0/16", "10.50.0.0/24") // same CIDR
	q("ecs", "create-cluster", "--cluster-name", "default", "--query", "cluster.clusterName", "--output", "text")
	registerTaskDef(q, "ovl-server", vpcServerScript)
	registerTaskDef(q, "ovl-client", "sleep 120")

	serverA := runTask(q, "ovl-server", subnetA)
	clientA := runTask(q, "ovl-client", subnetA)
	clientB := runTask(q, "ovl-client", subnetB)
	t.Cleanup(func() {
		for _, task := range []string{serverA, clientA, clientB} {
			runCLI(t, awsCLI("ecs", "stop-task", "--cluster", "default", "--task", task))
		}
	})
	waitRunning(t, q, serverA)
	waitRunning(t, q, clientA)
	waitRunning(t, q, clientB)

	// Both VPCs keep the real CIDR — the server's ENI IP is its real eth0, and a
	// task in VPC-B legitimately gets the same address (separate routing tables).
	ip := taskENIIP(q, serverA)
	if !strings.HasPrefix(ip, "10.50.0.") {
		t.Fatalf("server should keep its real AWS CIDR (no remap): got %q", ip)
	}
	if real := taskEth0IP(t, serverA); real != ip {
		t.Fatalf("reported ENI IP %q != real eth0 IP %q", ip, real)
	}

	// Same-VPC reaches the server; the same-CIDR VPC-B task is fully isolated.
	if code, out := taskWget(t, clientA, ip); code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("same-VPC task should reach %s: exit=%d out=%q", ip, code, out)
	}
	if code, _ := taskWget(t, clientB, ip); code == 0 {
		t.Fatalf("overlapping-CIDR VPC-B task must be isolated from VPC-A's %s", ip)
	}
}
