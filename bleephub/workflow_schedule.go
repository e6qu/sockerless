package bleephub

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// scheduleFiredKeys dedupes cron firings: a (repo, file, cron) tuple
// fires at most once per minute even if the ticker drifts.
type scheduleFiredKeys struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// startScheduleDispatcher launches the minute-aligned loop that fires
// `on: schedule:` workflows, the server-side clock real GitHub runs for
// cron triggers.
func (s *Server) startScheduleDispatcher() {
	go func() {
		for {
			now := time.Now().UTC()
			next := now.Truncate(time.Minute).Add(time.Minute)
			time.Sleep(time.Until(next))
			s.fireDueSchedules(time.Now().UTC())
		}
	}()
}

// fireDueSchedules triggers every schedule-bearing workflow (at HEAD of
// each repo's default branch) whose cron matches the given minute.
// Separated from the ticker so tests drive it with a fixed clock.
func (s *Server) fireDueSchedules(now time.Time) {
	minute := now.Truncate(time.Minute)

	s.store.mu.RLock()
	repoKeys := make([]string, 0, len(s.store.ReposByName))
	for key := range s.store.ReposByName {
		repoKeys = append(repoKeys, key)
	}
	s.store.mu.RUnlock()

	for _, repoKey := range repoKeys {
		parts := splitRepoKeyParts(repoKey)
		stor := s.store.GetGitStorage(parts[0], parts[1])
		if stor == nil {
			continue
		}
		for name, content := range listWorkflowFiles(stor) {
			on, err := ParseWorkflowOn(content)
			if err != nil {
				continue
			}
			sched := on["schedule"]
			if sched == nil || len(sched.Crons) == 0 {
				continue
			}
			if s.workflowFileDisabled(repoKey, name) {
				continue
			}
			for _, cron := range sched.Crons {
				cs, err := parseCron(cron)
				if err != nil {
					s.logger.Warn().Err(err).Str("file", name).Str("cron", cron).Msg("invalid cron in on: schedule")
					continue
				}
				if !cs.matches(minute) {
					continue
				}
				if !s.markScheduleFired(repoKey+"\x00"+name+"\x00"+cron, minute) {
					continue
				}
				s.fireScheduledWorkflow(repoKey, name, content, cron)
			}
		}
	}
}

// markScheduleFired records a firing; false means this (key, minute)
// already fired.
func (s *Server) markScheduleFired(key string, minute time.Time) bool {
	s.scheduleFired.mu.Lock()
	defer s.scheduleFired.mu.Unlock()
	if s.scheduleFired.seen == nil {
		s.scheduleFired.seen = map[string]time.Time{}
	}
	if last, ok := s.scheduleFired.seen[key]; ok && last.Equal(minute) {
		return false
	}
	s.scheduleFired.seen[key] = minute
	return true
}

// fireScheduledWorkflow submits one schedule-triggered run. The schedule
// event has no webhook delivery on real GitHub — it only starts the run;
// its payload carries the matching cron line.
func (s *Server) fireScheduledWorkflow(repoKey, fileName string, content []byte, cron string) {
	s.store.mu.RLock()
	repo := s.store.ReposByName[repoKey]
	s.store.mu.RUnlock()
	if repo == nil {
		return
	}
	defaultBranch := repo.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	ref := "refs/heads/" + defaultBranch

	parts := splitRepoKeyParts(repoKey)
	stor := s.store.GetGitStorage(parts[0], parts[1])

	payload := map[string]interface{}{
		"schedule":   cron,
		"repository": repoPayload(repo),
	}
	sha := resolveRefSha(stor, ref)
	if sha == "0000000000000000000000000000000000000000" {
		s.logger.Error().
			Str("repo", repoKey).
			Str("ref", ref).
			Str("cron", cron).
			Msg("scheduled workflow rejected because the default-branch git ref did not resolve to a commit")
		return
	}
	meta := &WorkflowEventMeta{
		EventName: "schedule",
		Ref:       ref,
		Sha:       sha,
		Repo:      repoKey,
		Payload:   payload,
	}
	workflow, err := s.submitTriggeredWorkflow(fileName, content, meta)
	if err != nil {
		s.logger.Error().Err(err).Str("file", fileName).Str("cron", cron).Msg("failed to fire scheduled workflow")
		return
	}
	s.logger.Info().
		Str("workflow_id", workflow.ID).
		Str("file", fileName).
		Str("cron", cron).
		Msg("workflow fired by schedule")
}

// ── Cron parsing (POSIX 5-field, with JAN-DEC / SUN-SAT names) ──────

type cronSchedule struct {
	min, hour, dom, month, dow uint64
	domStar, dowStar           bool
}

var cronMonthNames = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

var cronDowNames = map[string]int{
	"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
}

// parseCron parses a 5-field cron expression (minute hour day-of-month
// month day-of-week) with lists, ranges, steps, and month/day names.
func parseCron(expr string) (*cronSchedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron %q: want 5 fields, got %d", expr, len(fields))
	}
	cs := &cronSchedule{}
	var err error
	if cs.min, _, err = parseCronField(fields[0], 0, 59, nil); err != nil {
		return nil, fmt.Errorf("cron %q minute: %w", expr, err)
	}
	if cs.hour, _, err = parseCronField(fields[1], 0, 23, nil); err != nil {
		return nil, fmt.Errorf("cron %q hour: %w", expr, err)
	}
	if cs.dom, cs.domStar, err = parseCronField(fields[2], 1, 31, nil); err != nil {
		return nil, fmt.Errorf("cron %q day-of-month: %w", expr, err)
	}
	if cs.month, _, err = parseCronField(fields[3], 1, 12, cronMonthNames); err != nil {
		return nil, fmt.Errorf("cron %q month: %w", expr, err)
	}
	if cs.dow, cs.dowStar, err = parseCronField(fields[4], 0, 7, cronDowNames); err != nil {
		return nil, fmt.Errorf("cron %q day-of-week: %w", expr, err)
	}
	// 7 means Sunday, same as 0.
	if cs.dow&(1<<7) != 0 {
		cs.dow |= 1
		cs.dow &^= 1 << 7
	}
	return cs, nil
}

// parseCronField parses one field into a bitset. star reports whether
// the field was unrestricted ("*"), which matters for the day-of-month /
// day-of-week OR rule.
func parseCronField(field string, lo, hi int, names map[string]int) (bits uint64, star bool, err error) {
	resolve := func(tok string) (int, error) {
		if names != nil {
			if v, ok := names[strings.ToUpper(tok)]; ok {
				return v, nil
			}
		}
		n, err := strconv.Atoi(tok)
		if err != nil {
			return 0, fmt.Errorf("invalid value %q", tok)
		}
		return n, nil
	}
	star = field == "*"
	for _, term := range strings.Split(field, ",") {
		step := 1
		if idx := strings.IndexByte(term, '/'); idx >= 0 {
			st, err := strconv.Atoi(term[idx+1:])
			if err != nil || st <= 0 {
				return 0, false, fmt.Errorf("invalid step in %q", term)
			}
			step = st
			term = term[:idx]
		}
		from, to := lo, hi
		switch {
		case term == "*":
			// full range
		case strings.Contains(term, "-"):
			parts := strings.SplitN(term, "-", 2)
			if from, err = resolve(parts[0]); err != nil {
				return 0, false, err
			}
			if to, err = resolve(parts[1]); err != nil {
				return 0, false, err
			}
		default:
			v, err := resolve(term)
			if err != nil {
				return 0, false, err
			}
			from, to = v, v
		}
		if from < lo || to > hi || from > to {
			return 0, false, fmt.Errorf("value out of range [%d-%d] in %q", lo, hi, term)
		}
		for v := from; v <= to; v += step {
			bits |= 1 << uint(v)
		}
	}
	if bits == 0 {
		return 0, false, fmt.Errorf("empty field %q", field)
	}
	return bits, star, nil
}

// matches reports whether the schedule fires at t (minute precision).
// Standard cron rule: when both day-of-month and day-of-week are
// restricted, either matching suffices.
func (c *cronSchedule) matches(t time.Time) bool {
	if c.min&(1<<uint(t.Minute())) == 0 {
		return false
	}
	if c.hour&(1<<uint(t.Hour())) == 0 {
		return false
	}
	if c.month&(1<<uint(int(t.Month()))) == 0 {
		return false
	}
	domOK := c.dom&(1<<uint(t.Day())) != 0
	dowOK := c.dow&(1<<uint(int(t.Weekday()))) != 0
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dowOK
	case c.dowStar:
		return domOK
	default:
		return domOK || dowOK
	}
}
