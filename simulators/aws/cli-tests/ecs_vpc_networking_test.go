package aws_cli_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

const vpcNetBusybox = "public.ecr.aws/docker/library/busybox:latest"

// TestECSVPCNetworking proves issue #516 end-to-end: an ECS task's ENI
// privateIPv4Address is the container's REAL, routable IP (not a phantom VPC
// address), reachable from another container in the same VPC and isolated from a
// different VPC — i.e. VPCs are genuinely isolated networks with the implicit
// local route enforced by a real Linux bridge.
func TestECSVPCNetworking(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	q := func(args ...string) string { return strings.TrimSpace(runCLI(t, awsCLI(args...))) }

	mkVPC := func(name, vpcCidr, snCidr string) (vpcID, subnetID string) {
		vpcID = q("ec2", "create-vpc", "--cidr-block", vpcCidr, "--query", "Vpc.VpcId", "--output", "text")
		subnetID = q("ec2", "create-subnet", "--vpc-id", vpcID, "--cidr-block", snCidr,
			"--availability-zone", "us-east-1a", "--query", "Subnet.SubnetId", "--output", "text")
		return vpcID, subnetID
	}
	vpcA, subnetA := mkVPC("a", "10.91.0.0/16", "10.91.0.0/24")
	vpcB, subnetB := mkVPC("b", "10.92.0.0/16", "10.92.0.0/24")
	q("ecs", "create-cluster", "--cluster-name", "default", "--query", "cluster.clusterName", "--output", "text")

	// A long-lived HTTP server task in VPC-A.
	serverScript := "mkdir -p /www && echo ok > /www/index.html && httpd -f -p 80 -h /www"
	q("ecs", "register-task-definition", "--family", "vpc-server",
		"--network-mode", "awsvpc", "--requires-compatibilities", "FARGATE", "--cpu", "256", "--memory", "512",
		"--container-definitions", `[{"name":"app","image":"`+vpcNetBusybox+`","entryPoint":["sh","-c"],"command":["`+serverScript+`"]}]`,
		"--query", "taskDefinition.taskDefinitionArn", "--output", "text")
	// A second task in VPC-B materialises VPC-B's network for the isolation probe.
	q("ecs", "register-task-definition", "--family", "vpc-idle",
		"--network-mode", "awsvpc", "--requires-compatibilities", "FARGATE", "--cpu", "256", "--memory", "512",
		"--container-definitions", `[{"name":"app","image":"`+vpcNetBusybox+`","entryPoint":["sh","-c"],"command":["sleep 120"]}]`,
		"--query", "taskDefinition.taskDefinitionArn", "--output", "text")

	taskA := q("ecs", "run-task", "--cluster", "default", "--task-definition", "vpc-server",
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnetA+`]}`,
		"--query", "tasks[0].taskArn", "--output", "text")
	taskB := q("ecs", "run-task", "--cluster", "default", "--task-definition", "vpc-idle",
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnetB+`]}`,
		"--query", "tasks[0].taskArn", "--output", "text")
	t.Cleanup(func() {
		runCLI(t, awsCLI("ecs", "stop-task", "--cluster", "default", "--task", taskA))
		runCLI(t, awsCLI("ecs", "stop-task", "--cluster", "default", "--task", taskB))
		_ = exec.Command("docker", "network", "rm", ecsVPCNet(vpcA), ecsVPCNet(vpcB)).Run()
	})

	// Wait for the server task to be RUNNING and read its ENI IP.
	var eniIP string
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		st := q("ecs", "describe-tasks", "--cluster", "default", "--tasks", taskA,
			"--query", "tasks[0].lastStatus", "--output", "text")
		if st == "RUNNING" {
			eniIP = q("ecs", "describe-tasks", "--cluster", "default", "--tasks", taskA,
				"--query", "tasks[0].containers[0].networkInterfaces[0].privateIpv4Address", "--output", "text")
			break
		}
		time.Sleep(2 * time.Second)
	}
	if eniIP == "" || !strings.HasPrefix(eniIP, "10.91.0.") {
		t.Fatalf("server task ENI IP not in subnet-A CIDR: got %q", eniIP)
	}

	// #516 core: the container's REAL Docker IP equals the ENI IP.
	realIP := dockerTaskIP(t, taskID(taskA), ecsVPCNet(vpcA))
	if realIP != eniIP {
		t.Fatalf("ENI privateIPv4Address %q != container's real IP %q (issue #516)", eniIP, realIP)
	}

	// Intra-VPC: a probe in the same VPC reaches the task over TCP.
	if code, out := dockerProbe(ecsVPCNet(vpcA), eniIP); code != 0 || !strings.Contains(out, "ok") {
		t.Fatalf("same-VPC probe should reach %s: exit=%d out=%q", eniIP, code, out)
	}
	// Cross-VPC: a probe in a different VPC cannot reach it (isolation).
	if code, _ := dockerProbe(ecsVPCNet(vpcB), eniIP); code == 0 {
		t.Fatalf("cross-VPC probe to %s should be isolated but succeeded", eniIP)
	}
}

func ecsVPCNet(vpcID string) string { return "sockerless-sim-vpc-" + vpcID }

func taskID(taskArn string) string {
	parts := strings.Split(taskArn, "/")
	return parts[len(parts)-1]
}

// dockerTaskIP returns the task container's IPv4 on the given network.
func dockerTaskIP(t *testing.T, tid, netName string) string {
	t.Helper()
	cid := strings.TrimSpace(dockerOut(t, "ps", "-q", "-f", "label=sockerless-sim-task="+tid))
	if cid == "" {
		t.Fatalf("no running container for task %s", tid)
	}
	tmpl := "{{with index .NetworkSettings.Networks \"" + netName + "\"}}{{.IPAddress}}{{end}}"
	return strings.TrimSpace(dockerOut(t, "inspect", "-f", tmpl, cid))
}

func dockerOut(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// dockerProbe runs a one-shot busybox on netName and TCP-fetches ip:80, returning
// the exit code + stdout.
func dockerProbe(netName, ip string) (int, string) {
	out, err := exec.Command("docker", "run", "--rm", "--network", netName,
		vpcNetBusybox, "wget", "-T", "3", "-q", "-O", "-", "http://"+ip+"/index.html").CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), string(out)
		}
		return -1, string(out)
	}
	return 0, string(out)
}
