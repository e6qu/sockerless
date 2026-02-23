package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "context":
		if len(os.Args) < 3 {
			contextUsage()
			os.Exit(1)
		}
		switch os.Args[2] {
		case "create":
			contextCreate(os.Args[3:])
		case "list", "ls":
			contextList()
		case "show":
			contextShow(os.Args[3:])
		case "use":
			contextUse(os.Args[3:])
		case "delete", "rm":
			contextDelete(os.Args[3:])
		case "current":
			contextCurrent()
		case "reload":
			contextReload()
		default:
			contextUsage()
			os.Exit(1)
		}
	case "status":
		cmdStatus()
	case "ps":
		cmdPs()
	case "server":
		cmdServer(os.Args[2:])
	case "metrics":
		cmdMetrics()
	case "resources":
		cmdResources(os.Args[2:])
	case "check":
		cmdCheck()
	case "version":
		fmt.Println("sockerless v0.1.0")
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless <command>

Commands:
  context   Manage backend contexts
  server    Start/stop/restart servers
  status    Show server status
  ps        List containers
  metrics   Show server metrics
  resources Manage cloud resources
  check     Run backend health checks
  version   Print version`)
}

func contextUsage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless context <subcommand>

Subcommands:
  create   Create a new context
  list     List all contexts
  show     Show context details
  use      Set the active context
  delete   Delete a context
  current  Show the active context name
  reload   Reload context config on running servers`)
}
