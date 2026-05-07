#!/usr/bin/env bash
# manual-test-real-workloads.sh — exercise a sockerless backend with real
# container workloads. Bundles all probes into a small number of `docker
# run` invocations to keep cloud-cold-start latency manageable (each
# `docker run` against a FaaS backend is one cold start; against
# Cloud Run Jobs is one Job creation + execution = ~60-120s).
#
# Usage:
#   DOCKER_HOST=tcp://127.0.0.1:3375 ./scripts/manual-test-real-workloads.sh [name]
#
#   `name` is a free-text label that prefixes the log directory.
#
# Honors:
#   DOCKER_HOST    where sockerless is listening (or any docker daemon)
#   PROBE_TIMEOUT  per-bundle timeout in seconds (default 360)
#   SKIP_BUILD     "1" to skip the Go-build bundle (faster, fewer cloud builds)
#
# Strategy: three bundles of probes, each one `docker run`:
#   bundle-O: alpine sh -c '...os/kernel/cap/net probes in series...'
#   bundle-N: alpine sh -c '...network DNS + HTTPS probes...'
#   bundle-W: golang:1.22-alpine sh -c '...go build + run arithmetic...'
#
# Per-bundle log files in /tmp/sockerless-real-workloads/<label>/.

set -u

LABEL="${1:-default}"
TIMEOUT="${PROBE_TIMEOUT:-360}"
LOGDIR="/tmp/sockerless-real-workloads/${LABEL}"
rm -rf "$LOGDIR"
mkdir -p "$LOGDIR"

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    GREEN='\033[32m'; RED='\033[31m'; YELLOW='\033[33m'; DIM='\033[2m'; RESET='\033[0m'
else
    GREEN=''; RED=''; YELLOW=''; DIM=''; RESET=''
fi

PASS=0
FAIL=0

# check ROW DESCRIPTION EXPECTED-SUBSTRING LOGFILE
check() {
    local row="$1"; local desc="$2"; local expected="$3"; local log="$4"
    if grep -qF "$expected" "$log"; then
        printf "${GREEN}PASS${RESET} [%s] %-40s ${DIM}(matched %q)${RESET}\n" "$row" "$desc" "$expected"
        PASS=$((PASS+1))
    else
        printf "${RED}FAIL${RESET} [%s] %-40s ${DIM}(expected %q not found)${RESET}\n" "$row" "$desc" "$expected"
        FAIL=$((FAIL+1))
    fi
}

echo "=== sockerless real-workload probes — label='$LABEL' DOCKER_HOST=$DOCKER_HOST ==="
echo "    timeout=${TIMEOUT}s log dir=$LOGDIR"
echo ""

if ! docker info > "$LOGDIR/_info.log" 2>&1; then
    echo -e "${RED}ABORT${RESET}: docker info failed against $DOCKER_HOST"
    sed 's/^/  /' "$LOGDIR/_info.log"
    exit 2
fi

driver=$(grep -E '^\s+(Storage Driver|Server Version|OSType|Architecture):' "$LOGDIR/_info.log" | tr -d '\n' | head -c 200)
echo -e "${YELLOW}backend identity${RESET}: $driver"
echo ""

# ─────────────────────────── bundle-O ───────────────────────────────
echo -e "${DIM}--- bundle-O: os/kernel/cap probes (1 docker run, alpine) ---${RESET}"
timeout "$TIMEOUT" docker run --rm alpine sh -c '
echo "###O01-uname-a###"
uname -a
echo "###O02-proc-version###"
cat /proc/version
echo "###O03-os-release###"
cat /etc/os-release
echo "###O04-cpuinfo###"
head -3 /proc/cpuinfo
echo "###O05-meminfo###"
head -1 /proc/meminfo
echo "###O06-mount###"
mount | head -3
echo "###C01-id###"
id
echo "###C02-hostname###"
hostname
echo "###C03-ulimit###"
ulimit -a 2>&1
echo "###C04-ps###"
ps -ef
echo "###END-BUNDLE-O###"
' > "$LOGDIR/bundle-O.log" 2>&1
echo "  done (exit=$?)"
check O01 "uname -a"          "Linux"      "$LOGDIR/bundle-O.log"
check O02 "kernel version"    "Linux"      "$LOGDIR/bundle-O.log"
check O03 "/etc/os-release"   "Alpine"     "$LOGDIR/bundle-O.log"
check O04 "cpuinfo"           "###O04"     "$LOGDIR/bundle-O.log"
check O05 "meminfo"           "MemTotal"   "$LOGDIR/bundle-O.log"
check O06 "mount table"       "###O06"     "$LOGDIR/bundle-O.log"
check C01 "id (uid+gid)"      "uid="       "$LOGDIR/bundle-O.log"
check C02 "hostname"          "###C02"     "$LOGDIR/bundle-O.log"
check C03 "ulimit -a"         "open files" "$LOGDIR/bundle-O.log"
check C04 "ps -ef"            "PID"        "$LOGDIR/bundle-O.log"
check ZZ  "bundle-O end-marker" "###END-BUNDLE-O###" "$LOGDIR/bundle-O.log"

echo ""
echo -e "${DIM}--- bundle-E: env passthrough (alpine -e ... env, with sleep so the container outlives Cloud Logging ingestion) ---${RESET}"
# `env` exits in <100ms; on FaaS backends the Cloud Logging ingestion
# pipeline can lose the last lines if the container exits faster than
# the per-backend final-fetch settle window. Sleep 4s before exit so
# the env output is always queryable by the time attach grabs the tail.
timeout "$TIMEOUT" docker run --rm -e FOOBAR=baz -e ANOTHER=qux alpine sh -c 'env; sleep 4' > "$LOGDIR/bundle-E.log" 2>&1
echo "  done (exit=$?)"
check C05 "env FOOBAR=baz"    "FOOBAR=baz"  "$LOGDIR/bundle-E.log"
check C06 "env ANOTHER=qux"   "ANOTHER=qux" "$LOGDIR/bundle-E.log"

echo ""
echo -e "${DIM}--- bundle-N: network/DNS/HTTPS (1 docker run, alpine) ---${RESET}"
timeout "$TIMEOUT" docker run --rm alpine sh -c '
echo "###N01-getent###"
getent hosts google.com 2>&1 | head
echo "###N02-https-head###"
wget -S --spider --timeout=20 https://www.google.com/ 2>&1 | head -5 || echo "wget-failed"
echo "###END-BUNDLE-N###"
' > "$LOGDIR/bundle-N.log" 2>&1
echo "  done (exit=$?)"
check N01 "DNS resolve google.com" "google.com" "$LOGDIR/bundle-N.log"
check N02 "outbound HTTPS"         "200"        "$LOGDIR/bundle-N.log"

if [ "${SKIP_BUILD:-0}" != "1" ]; then
    echo ""
    echo -e "${DIM}--- bundle-W: real workload — go build + arithmetic eval (1 docker run, golang:alpine) ---${RESET}"
    # Inline Go source delivered as a single docker run. Source is
    # base64-encoded to ride safely through shell quoting.
    GO_SRC_B64=$(cat <<'GOSRC' | base64 | tr -d '\n'
package main
import (
  "fmt"
  "os"
)
type Tk struct{ k int; v float64 }
const (P=0;M=1;X=2;D=3;N=4;LP=5;RP=6;EOF=7)
func tok(s string)([]Tk,error){
  var t []Tk; i:=0
  for i<len(s){
    c:=s[i]
    switch{
    case c==32||c==9: i++
    case c==43: t=append(t,Tk{P,0}); i++
    case c==45: t=append(t,Tk{M,0}); i++
    case c==42: t=append(t,Tk{X,0}); i++
    case c==47: t=append(t,Tk{D,0}); i++
    case c==40: t=append(t,Tk{LP,0}); i++
    case c==41: t=append(t,Tk{RP,0}); i++
    case (c>=48&&c<=57)||c==46:
      j:=i
      for j<len(s)&&((s[j]>=48&&s[j]<=57)||s[j]==46){ j++ }
      var v float64; fmt.Sscanf(s[i:j],"%f",&v)
      t=append(t,Tk{N,v}); i=j
    default: return nil,fmt.Errorf("bad %q at %d",c,i)
    }
  }
  t=append(t,Tk{EOF,0}); return t,nil
}
type P_ struct{ t []Tk; i int }
func (p *P_)pe()Tk{return p.t[p.i]}
func (p *P_)ad()Tk{r:=p.t[p.i];p.i++;return r}
func (p *P_)expr()(float64,error){
  l,e:=p.term(); if e!=nil{return 0,e}
  for p.pe().k==P||p.pe().k==M {o:=p.ad();r,e:=p.term();if e!=nil{return 0,e};if o.k==P{l+=r}else{l-=r}}
  return l,nil
}
func (p *P_)term()(float64,error){
  l,e:=p.factor(); if e!=nil{return 0,e}
  for p.pe().k==X||p.pe().k==D {o:=p.ad();r,e:=p.factor();if e!=nil{return 0,e};if o.k==X{l*=r}else{l/=r}}
  return l,nil
}
func (p *P_)factor()(float64,error){
  t:=p.pe()
  switch t.k{
  case N: p.ad(); return t.v,nil
  case LP: p.ad(); v,e:=p.expr(); if e!=nil{return 0,e}; p.ad(); return v,nil
  case M: p.ad(); v,e:=p.factor(); return -v,e
  }
  return 0,fmt.Errorf("bad token")
}
func evalE(s string)(float64,error){tk,e:=tok(s);if e!=nil{return 0,e};p:=&P_{t:tk};return p.expr()}
func main(){
  cases:=[]string{"1+2*3","(4+5)*6","10/2-1","2.5*4"}
  expect:=[]float64{7,54,4,10}
  for i,c:=range cases{
    g,e:=evalE(c)
    if e!=nil{fmt.Println("FAIL",c,e);os.Exit(1)}
    if g!=expect[i]{fmt.Printf("FAIL %s = %v, want %v\n",c,g,expect[i]);os.Exit(1)}
    fmt.Printf("OK  %s = %v\n",c,g)
  }
  fmt.Println("ARITHMETIC-OK")
}
GOSRC
)
    # shellcheck disable=SC2016 # `$GO_SRC_B64` expands inside the
    # container via the `-e` env passthrough, not in this outer shell.
    timeout "$TIMEOUT" docker run --rm -e GO_SRC_B64="$GO_SRC_B64" golang:1.22-alpine sh -c '
mkdir -p /tmp/m && cd /tmp/m
echo "$GO_SRC_B64" | base64 -d > main.go
echo "module m" > go.mod
echo "go 1.22" >> go.mod
go run main.go
' > "$LOGDIR/bundle-W.log" 2>&1
    echo "  done (exit=$?)"
    check W01 "go run arithmetic" "ARITHMETIC-OK" "$LOGDIR/bundle-W.log"
else
    echo ""
    echo -e "${YELLOW}SKIP${RESET} bundle-W (SKIP_BUILD=1)"
fi

echo ""
echo "=== summary ==="
TOTAL=$((PASS+FAIL))
if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}ALL ${TOTAL} ROWS PASS${RESET}  (label=$LABEL  log dir=$LOGDIR)"
    exit 0
else
    echo -e "${RED}${FAIL}/${TOTAL} ROWS FAILED${RESET}  (label=$LABEL  log dir=$LOGDIR)"
    echo -e "${DIM}    inspect logs above to triage${RESET}"
    exit 1
fi
