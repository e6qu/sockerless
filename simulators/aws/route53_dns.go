package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// Route 53 is the authoritative DNS server for its hosted zones. The HTTP
// API lets callers mutate the store; this UDP/TCP DNS listener answers real
// DNS queries against that same store, so `dig @<sim> <name>` returns what
// ChangeResourceRecordSets wrote. There is no second source of truth and no
// snapshot: each query walks r53Zones directly.

var (
	r53DNSAddr string
	r53DNSOnce sync.Once
)

func startRoute53DNSServer() {
	r53DNSOnce.Do(func() {
		addr := ":" + envOr("SIM_DNS_PORT", "5353")
		udpConn, err := net.ListenPacket("udp", addr)
		if err != nil {
			// Fall back to a kernel-assigned free port so the
			// simulator still boots when the configured port is busy.
			udpConn, err = net.ListenPacket("udp", ":0")
			if err != nil {
				log.Printf("route53 dns: failed to listen udp: %v", err)
				return
			}
		}
		r53DNSAddr = udpConn.LocalAddr().String()
		log.Printf("route53 dns: serving on udp %s", r53DNSAddr)

		go serveRoute53DNSUDP(udpConn)

		if tcpLn, err := net.Listen("tcp", udpConn.LocalAddr().String()); err == nil {
			go serveRoute53DNSTCP(tcpLn)
		}
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func serveRoute53DNSUDP(c net.PacketConn) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := c.ReadFrom(buf)
		if err != nil {
			if isClosedErr(err) {
				return
			}
			continue
		}
		query := append([]byte(nil), buf[:n]...)
		go func(q []byte, from net.Addr) {
			resp, _ := answerRoute53DNS(q)
			if resp != nil {
				_, _ = c.WriteTo(resp, from)
			}
		}(query, addr)
	}
}

func serveRoute53DNSTCP(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if isClosedErr(err) {
				return
			}
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
			lenBuf := make([]byte, 2)
			if _, err := readFull(c, lenBuf); err != nil {
				return
			}
			msgLen := int(lenBuf[0])<<8 | int(lenBuf[1])
			if msgLen < 12 || msgLen > 65535 {
				return
			}
			msgBuf := make([]byte, msgLen)
			if _, err := readFull(c, msgBuf); err != nil {
				return
			}
			resp, _ := answerRoute53DNS(msgBuf)
			if resp == nil {
				return
			}
			out := make([]byte, 2+len(resp))
			out[0] = byte(len(resp) >> 8)
			out[1] = byte(len(resp))
			copy(out[2:], resp)
			_, _ = conn.Write(out)
		}(conn)
	}
}

func readFull(c net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func isClosedErr(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "use of closed") ||
		strings.Contains(err.Error(), "closed network"))
}

// answerRoute53DNS parses a DNS query packet, resolves it against the Route 53
// store, and returns a packed DNS response. A nil response means "drop the
// packet" — used only for malformed queries that have no question to echo back.
func answerRoute53DNS(query []byte) ([]byte, error) {
	var p dnsmessage.Parser
	hdr, err := p.Start(query)
	if err != nil {
		return nil, err
	}
	q, err := p.Question()
	if err != nil {
		// No question: mirror real DNS — reply with the header and FORMERR.
		return buildRoute53DNSResponse(hdr, dnsmessage.RCodeFormatError, q, nil), nil
	}

	answers, rcode := resolveRoute53(q)
	resp := buildRoute53DNSResponse(hdr, rcode, q, answers)
	return resp, nil
}

func buildRoute53DNSResponse(qHdr dnsmessage.Header, rcode dnsmessage.RCode, q dnsmessage.Question, answers []dnsmessage.Resource) []byte {
	out := dnsmessage.Header{
		ID:               qHdr.ID,
		Response:         true,
		OpCode:           qHdr.OpCode,
		Authoritative:    true,
		RecursionDesired: qHdr.RecursionDesired,
		RCode:            rcode,
	}
	b := dnsmessage.NewBuilder(nil, out)
	b.EnableCompression()
	if q.Name.Length != 0 {
		if err := b.StartQuestions(); err == nil {
			_ = b.Question(q)
		}
	}
	if len(answers) > 0 {
		_ = b.StartAnswers()
		for _, a := range answers {
			_ = packAnyResource(&b, a)
		}
	}
	msg, err := b.Finish()
	if err != nil {
		return nil
	}
	return msg
}

func packAnyResource(b *dnsmessage.Builder, r dnsmessage.Resource) error {
	h := r.Header
	h.Class = dnsmessage.ClassINET
	switch body := r.Body.(type) {
	case *dnsmessage.AResource:
		return b.AResource(h, *body)
	case *dnsmessage.AAAAResource:
		return b.AAAAResource(h, *body)
	case *dnsmessage.CNAMEResource:
		return b.CNAMEResource(h, *body)
	case *dnsmessage.TXTResource:
		return b.TXTResource(h, *body)
	case *dnsmessage.NSResource:
		return b.NSResource(h, *body)
	case *dnsmessage.MXResource:
		return b.MXResource(h, *body)
	case *dnsmessage.PTRResource:
		return b.PTRResource(h, *body)
	case *dnsmessage.SRVResource:
		return b.SRVResource(h, *body)
	case *dnsmessage.SOAResource:
		return b.SOAResource(h, *body)
	default:
		return nil
	}
}

// resolveRoute53 walks the Route 53 store, finds the longest-suffix matching
// hosted zone, and returns answer records for the question.
func resolveRoute53(q dnsmessage.Question) ([]dnsmessage.Resource, dnsmessage.RCode) {
	qName := normalizeDNSName(q.Name.String())
	qType := q.Type

	zoneID, zoneName := longestMatchingZone(qName)
	if zoneID == "" {
		return nil, dnsmessage.RCodeNameError
	}
	stored, ok := r53Zones.Get(zoneID)
	if !ok {
		return nil, dnsmessage.RCodeServerFailure
	}

	hdrTTL := uint32(300)

	var answers []dnsmessage.Resource
	matched := false
	for _, rr := range stored.Records {
		if !strings.EqualFold(strings.TrimSuffix(rr.Name, "."), qName) {
			continue
		}
		rrType := strings.ToUpper(rr.Type)
		if rrType != typeNameForQType(qType) {
			continue
		}
		matched = true
		if rr.AliasTarget != nil && rr.AliasTarget.DNSName != "" {
			answers = append(answers, resolveAlias(rr.AliasTarget.DNSName, qType)...)
			continue
		}
		ttl := hdrTTL
		if rr.TTL != nil && *rr.TTL > 0 {
			ttl = uint32(*rr.TTL)
		}
		answers = append(answers, recordsFromRRSet(rr, qName, ttl)...)
	}

	// SOA at the apex is the negative-caching signal real Route 53 returns
	// for NXDOMAIN inside an existing zone. Other types with no records
	// return NOERROR with no answers (NODATA).
	if !matched && qType != dnsmessage.TypeSOA {
		soa := findSOA(stored.Records, zoneName)
		if soa != nil {
			return nil, dnsmessage.RCodeNameError
		}
	}

	return answers, dnsmessage.RCodeSuccess
}

func resolveAlias(target string, qType dnsmessage.Type) []dnsmessage.Resource {
	targetName := normalizeDNSName(strings.TrimSuffix(target, "."))
	zoneID, _ := longestMatchingZone(targetName)
	if zoneID == "" {
		return nil
	}
	stored, ok := r53Zones.Get(zoneID)
	if !ok {
		return nil
	}
	var out []dnsmessage.Resource
	for _, rr := range stored.Records {
		if !strings.EqualFold(strings.TrimSuffix(rr.Name, "."), targetName) {
			continue
		}
		rrType := strings.ToUpper(rr.Type)
		if rrType == typeNameForQType(qType) || (rrType == "A" && qType == dnsmessage.TypeA) {
			if rr.AliasTarget != nil && rr.AliasTarget.DNSName != "" {
				out = append(out, resolveAlias(rr.AliasTarget.DNSName, qType)...)
				continue
			}
			out = append(out, recordsFromRRSet(rr, targetName, 60)...)
		}
	}
	return out
}

func longestMatchingZone(qName string) (zoneID, zoneName string) {
	var best string
	var bestID string
	for _, sz := range r53Zones.List() {
		zone := strings.TrimSuffix(strings.ToLower(sz.Zone.Name), ".")
		if zone == "" {
			continue
		}
		if qName == zone || strings.HasSuffix(qName, "."+zone) {
			if len(zone) > len(best) {
				best = zone
				bestID = r53ZoneIDFromPath(sz.Zone.Id)
			}
		}
	}
	return bestID, best
}

func findSOA(records []R53ResourceRecordSet, zoneName string) *R53ResourceRecordSet {
	apex := dnsFullName(zoneName)
	for i := range records {
		if strings.EqualFold(records[i].Name, apex) && strings.EqualFold(records[i].Type, "SOA") {
			return &records[i]
		}
	}
	return nil
}

func recordsFromRRSet(rr R53ResourceRecordSet, name string, ttl uint32) []dnsmessage.Resource {
	if rr.ResourceRecords == nil {
		return nil
	}
	rrType := strings.ToUpper(rr.Type)
	dnsName, err := dnsmessage.NewName(dnsFullName(name))
	if err != nil {
		return nil
	}
	out := make([]dnsmessage.Resource, 0, len(rr.ResourceRecords.Items))
	hdr := dnsmessage.ResourceHeader{Name: dnsName, Class: dnsmessage.ClassINET, TTL: ttl}
	for _, rec := range rr.ResourceRecords.Items {
		val := rec.Value
		var body dnsmessage.ResourceBody
		switch rrType {
		case "A":
			ip := net.ParseIP(strings.TrimSpace(val))
			if ip4 := ip.To4(); ip4 != nil {
				var a [4]byte
				copy(a[:], ip4)
				body = &dnsmessage.AResource{A: a}
			}
		case "AAAA":
			ip := net.ParseIP(strings.TrimSpace(val))
			if ip.To4() == nil && ip != nil {
				var aaaa [16]byte
				copy(aaaa[:], ip.To16())
				body = &dnsmessage.AAAAResource{AAAA: aaaa}
			}
		case "CNAME", "PTR":
			n, err := dnsmessage.NewName(dnsFullName(strings.TrimSpace(val)))
			if err == nil {
				if rrType == "CNAME" {
					body = &dnsmessage.CNAMEResource{CNAME: n}
				} else {
					body = &dnsmessage.PTRResource{PTR: n}
				}
			}
		case "NS":
			n, err := dnsmessage.NewName(dnsFullName(strings.TrimSpace(val)))
			if err == nil {
				body = &dnsmessage.NSResource{NS: n}
			}
		case "TXT":
			body = &dnsmessage.TXTResource{TXT: splitTXTChunks(val)}
		case "MX":
			pref, host := splitMX(val)
			n, err := dnsmessage.NewName(dnsFullName(host))
			if err == nil {
				body = &dnsmessage.MXResource{Pref: pref, MX: n}
			}
		case "SRV":
			prio, weight, port, target := splitSRV(val)
			n, err := dnsmessage.NewName(dnsFullName(target))
			if err == nil {
				body = &dnsmessage.SRVResource{Priority: prio, Weight: weight, Port: port, Target: n}
			}
		case "SOA":
			body = buildSOAFromValue(val)
		}
		if body != nil {
			out = append(out, dnsmessage.Resource{Header: hdr, Body: body})
		}
	}
	return out
}

func buildSOAFromValue(val string) dnsmessage.ResourceBody {
	// Route 53 SOA value: "ns hostmaster serial refresh retry expire minttl"
	fields := strings.Fields(strings.TrimSpace(val))
	if len(fields) < 7 {
		return nil
	}
	serial, _ := strconv.ParseUint(fields[2], 10, 32)
	refresh, _ := strconv.ParseUint(fields[3], 10, 32)
	retry, _ := strconv.ParseUint(fields[4], 10, 32)
	expire, _ := strconv.ParseUint(fields[5], 10, 32)
	minTTL, _ := strconv.ParseUint(fields[6], 10, 32)
	ns, err := dnsmessage.NewName(dnsFullName(fields[0]))
	if err != nil {
		return nil
	}
	mbox, err := dnsmessage.NewName(dnsFullName(strings.ReplaceAll(fields[1], "@", ".")))
	if err != nil {
		return nil
	}
	return &dnsmessage.SOAResource{
		NS: ns, MBox: mbox,
		Serial: uint32(serial), Refresh: uint32(refresh),
		Retry: uint32(retry), Expire: uint32(expire), MinTTL: uint32(minTTL),
	}
}

func splitTXTChunks(val string) []string {
	const chunk = 255
	if len(val) == 0 {
		return []string{""}
	}
	out := []string{}
	for i := 0; i < len(val); i += chunk {
		end := i + chunk
		if end > len(val) {
			end = len(val)
		}
		out = append(out, val[i:end])
	}
	return out
}

func splitMX(val string) (uint16, string) {
	fields := strings.Fields(strings.TrimSpace(val))
	if len(fields) < 2 {
		return 0, val
	}
	pref, _ := strconv.ParseUint(fields[0], 10, 16)
	return uint16(pref), fields[1]
}

func splitSRV(val string) (prio, weight, port uint16, target string) {
	fields := strings.Fields(strings.TrimSpace(val))
	if len(fields) < 4 {
		return 0, 0, 0, val
	}
	p, _ := strconv.ParseUint(fields[0], 10, 16)
	w, _ := strconv.ParseUint(fields[1], 10, 16)
	pt, _ := strconv.ParseUint(fields[2], 10, 16)
	return uint16(p), uint16(w), uint16(pt), fields[3]
}

func typeNameForQType(t dnsmessage.Type) string {
	switch t {
	case dnsmessage.TypeA:
		return "A"
	case dnsmessage.TypeAAAA:
		return "AAAA"
	case dnsmessage.TypeCNAME:
		return "CNAME"
	case dnsmessage.TypeTXT:
		return "TXT"
	case dnsmessage.TypeNS:
		return "NS"
	case dnsmessage.TypeMX:
		return "MX"
	case dnsmessage.TypePTR:
		return "PTR"
	case dnsmessage.TypeSRV:
		return "SRV"
	case dnsmessage.TypeSOA:
		return "SOA"
	default:
		return ""
	}
}

func normalizeDNSName(s string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
}

func dnsFullName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "."
	}
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
