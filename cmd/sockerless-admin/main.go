package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

// componentFlag collects repeated --backend name=addr or --simulator name=addr flags.
type componentFlag struct {
	entries []struct{ name, addr string }
}

func (f *componentFlag) String() string { return "" }
func (f *componentFlag) Set(v string) error {
	name, addr, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("expected name=addr, got %q", v)
	}
	f.entries = append(f.entries, struct{ name, addr string }{name, addr})
	return nil
}

func main() {
	addr := flag.String("addr", ":9090", "admin server listen address")
	configPath := flag.String("config", "", "path to admin.json config file")

	var backends, simulators componentFlag
	flag.Var(&backends, "backend", "backend component as name=addr (repeatable)")
	flag.Var(&simulators, "simulator", "simulator component as name=addr (repeatable)")
	bleephubAddr := flag.String("bleephub", "", "bleephub coordinator address")
	frontendAddr := flag.String("frontend", "", "Docker frontend management address")
	showVersion := flag.Bool("version", false, "print version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("sockerless-admin %s\n", version)
		os.Exit(0)
	}

	reg := NewRegistry()
	procMgr := NewProcessManager(reg)
	projectMgr := NewProjectManager(procMgr, reg, defaultProjectStoreDir())

	// 1. Load from config file
	if *configPath != "" {
		cfg, err := loadConfigFile(reg, *configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		for _, p := range cfg.Processes {
			procMgr.AddProcess(p)
		}
	}

	// 2. Auto-discover from ~/.sockerless/contexts/
	discoverFromContexts(reg)

	// 3. Load persisted projects
	if projects, err := LoadProjects(defaultProjectStoreDir()); err == nil {
		for _, p := range projects {
			projectMgr.LoadProject(p)
		}
	}

	// 4. CLI flags (highest priority — override any discovered components)
	for _, b := range backends.entries {
		reg.Add(Component{Name: b.name, Type: "backend", Addr: normalizeAddr(b.addr)})
	}
	for _, s := range simulators.entries {
		reg.Add(Component{Name: "sim-" + s.name, Type: "simulator", Addr: normalizeAddr(s.addr)})
	}
	if *bleephubAddr != "" {
		reg.Add(Component{Name: "bleephub", Type: "coordinator", Addr: normalizeAddr(*bleephubAddr)})
	}
	if *frontendAddr != "" {
		reg.Add(Component{Name: "frontend", Type: "frontend", Addr: normalizeAddr(*frontendAddr)})
	}

	// Start background health polling
	done := make(chan struct{})
	go reg.PollLoop(5*time.Second, done)

	mux := http.NewServeMux()
	registerAPI(mux, reg, procMgr, projectMgr)
	registerUI(mux)

	// Redirect / to /ui/
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})

	srv := &http.Server{Addr: *addr, Handler: mux}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		close(done)
		projectMgr.StopAll()
		procMgr.StopAll()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("sockerless-admin %s listening on %s (%d components)", version, *addr, reg.Len())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// normalizeAddr ensures the address has an http:// scheme.
func normalizeAddr(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
