package sandbox

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

// builtinUname prints system information.
func builtinUname(args []string, hc interp.HandlerContext) error {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	hostname := "sandbox"
	hc.Env.Each(func(n string, vr expand.Variable) bool {
		if n == "HOSTNAME" && vr.Kind == expand.String {
			hostname = vr.Str
			return false
		}
		return true
	})
	if len(args) == 1 {
		fmt.Fprintln(hc.Stdout, "Linux")
		return nil
	}
	for _, flag := range args[1:] {
		switch flag {
		case "-a", "--all":
			fmt.Fprintf(hc.Stdout, "Linux %s 5.15.0 #1 SMP %s GNU/Linux\n", hostname, arch)
		case "-s":
			fmt.Fprintln(hc.Stdout, "Linux")
		case "-n":
			fmt.Fprintln(hc.Stdout, hostname)
		case "-r":
			fmt.Fprintln(hc.Stdout, "5.15.0")
		case "-m":
			fmt.Fprintln(hc.Stdout, arch)
		default:
			fmt.Fprintf(hc.Stderr, "uname: unknown option: %s\n", flag)
			return interp.ExitStatus(1)
		}
	}
	return nil
}

// builtinHostname prints the hostname from the HOSTNAME env var.
func builtinHostname(args []string, hc interp.HandlerContext) error {
	hostname := "localhost"
	hc.Env.Each(func(name string, vr expand.Variable) bool {
		if name == "HOSTNAME" && vr.Kind == expand.String {
			hostname = vr.Str
			return false
		}
		return true
	})
	fmt.Fprintln(hc.Stdout, hostname)
	return nil
}

// builtinId prints user/group identity information.
func builtinId(args []string, hc interp.HandlerContext) error {
	uid, gid := "0", "0"
	hc.Env.Each(func(name string, vr expand.Variable) bool {
		if vr.Kind != expand.String {
			return true
		}
		switch name {
		case "SOCKERLESS_UID":
			uid = vr.Str
		case "SOCKERLESS_GID":
			gid = vr.Str
		}
		return true
	})
	uname := "root"
	gname := "root"
	if uid != "0" {
		uname = "user"
	}
	if gid != "0" {
		gname = "group"
	}
	if len(args) == 1 {
		fmt.Fprintf(hc.Stdout, "uid=%s(%s) gid=%s(%s)\n", uid, uname, gid, gname)
		return nil
	}
	for _, flag := range args[1:] {
		switch flag {
		case "-u":
			fmt.Fprintln(hc.Stdout, uid)
		case "-g":
			fmt.Fprintln(hc.Stdout, gid)
		case "-un":
			fmt.Fprintln(hc.Stdout, uname)
		case "-gn":
			fmt.Fprintln(hc.Stdout, gname)
		default:
			fmt.Fprintf(hc.Stderr, "id: unknown option: %s\n", flag)
			return interp.ExitStatus(1)
		}
	}
	return nil
}

// builtinDate prints the current date in UTC.
func builtinDate(args []string, hc interp.HandlerContext) error {
	fmt.Fprintln(hc.Stdout, time.Now().UTC().Format("Mon Jan  2 15:04:05 UTC 2006"))
	return nil
}

// builtinPwd handles the pwd command. NOTE: mvdan.cc/sh handles pwd as a
// shell builtin (reads the PWD variable), so this is rarely reached.
// The PWD override in runShellInDir is the primary fix. Fallback only.
func builtinPwd(args []string, hc interp.HandlerContext, rootDir string) error {
	dir := hc.Dir
	if strings.HasPrefix(dir, rootDir) {
		dir = strings.TrimPrefix(dir, rootDir)
		if dir == "" {
			dir = "/"
		}
	}
	fmt.Fprintln(hc.Stdout, dir)
	return nil
}

// builtinWhich locates a command in builtins, applets, or PATH.
func builtinWhich(args []string, hc interp.HandlerContext, rootDir string) error {
	allFound := true
	for _, cmdName := range args[1:] {
		if cmdName == "" || cmdName[0] == '-' {
			continue
		}
		found := false
		// Check builtins
		if builtinCommands[cmdName] || isShellBinary(cmdName) {
			fmt.Fprintln(hc.Stdout, "/usr/bin/"+cmdName)
			found = true
		} else if knownApplets[cmdName] {
			fmt.Fprintln(hc.Stdout, "/bin/"+cmdName)
			found = true
		} else if p := findInPATH(cmdName, hc, rootDir); p != "" {
			// Strip rootDir to get container path
			cp := strings.TrimPrefix(p, rootDir)
			if cp == "" {
				cp = "/"
			}
			fmt.Fprintln(hc.Stdout, cp)
			found = true
		}
		if !found {
			fmt.Fprintf(hc.Stderr, "which: no %s in PATH\n", cmdName)
			allFound = false
		}
	}
	if !allFound {
		return interp.ExitStatus(1)
	}
	return nil
}
