package main

import (
	"strconv"
	"strings"
	"time"
)

// AWS EventBridge Scheduler cron evaluation. The expression is
// cron(minutes hours day-of-month month day-of-week year) — six fields. Each
// field supports `*`, `?` (treated as `*`), single values, lists (`,`), ranges
// (`-`), steps (`/`), and named months (JAN–DEC) / days (SUN–SAT). Day-of-week
// is 1–7 with 1 = Sunday, matching AWS. The `L`, `W`, and `#` qualifiers are not
// supported; an expression using them returns no next time (it won't fire),
// which is preferable to firing at the wrong moment.

var cronMonths = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

var cronDows = map[string]int{
	"SUN": 1, "MON": 2, "TUE": 3, "WED": 4, "THU": 5, "FRI": 6, "SAT": 7,
}

// schedulerCronNext returns the next fire time strictly after `after` for an AWS
// cron(...) expression, or false if the expression is unparseable / uses
// unsupported qualifiers / has no occurrence within four years.
func schedulerCronNext(expr string, after time.Time) (time.Time, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "cron(") || !strings.HasSuffix(expr, ")") {
		return time.Time{}, false
	}
	f := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(expr, "cron("), ")"))
	if len(f) != 6 {
		return time.Time{}, false
	}
	mins, ok1 := cronField(f[0], 0, 59, nil)
	hrs, ok2 := cronField(f[1], 0, 23, nil)
	doms, ok3 := cronField(f[2], 1, 31, nil)
	mons, ok4 := cronField(f[3], 1, 12, cronMonths)
	dows, ok5 := cronField(f[4], 1, 7, cronDows)
	yrs, ok6 := cronField(f[5], 1970, 2199, nil)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
		return time.Time{}, false
	}
	domWild := f[2] == "*" || f[2] == "?"
	dowWild := f[4] == "*" || f[4] == "?"

	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.AddDate(4, 0, 0)
	for t.Before(limit) {
		if yrs[t.Year()] && mons[int(t.Month())] && hrs[t.Hour()] && mins[t.Minute()] {
			domMatch := domWild || doms[t.Day()]
			dowMatch := dowWild || dows[int(t.Weekday())+1] // Go Sunday=0 → AWS 1
			if domMatch && dowMatch {
				return t, true
			}
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, false
}

// cronField parses one cron field into the set of matching values in [min,max].
func cronField(spec string, min, max int, names map[string]int) (map[int]bool, bool) {
	out := make(map[int]bool)
	for _, part := range strings.Split(spec, ",") {
		step := 1
		hasStep := false
		rng := part
		if i := strings.Index(part, "/"); i >= 0 {
			rng = part[:i]
			s, err := strconv.Atoi(part[i+1:])
			if err != nil || s <= 0 {
				return nil, false
			}
			step = s
			hasStep = true
		}
		lo, hi := min, max
		switch {
		case rng == "*" || rng == "?":
			// full range (with optional step)
		case strings.Contains(rng, "-"):
			i := strings.Index(rng, "-")
			a, okA := cronAtoi(rng[:i], names)
			b, okB := cronAtoi(rng[i+1:], names)
			if !okA || !okB {
				return nil, false
			}
			lo, hi = a, b
		default:
			v, ok := cronAtoi(rng, names)
			if !ok {
				return nil, false
			}
			lo = v
			// AWS: a bare value with a step ("N/step") means "from N to the
			// field max, every step" (e.g. minutes 0/5 → 0,5,…,55). Without a
			// step it is the single value N.
			if hasStep {
				hi = max
			} else {
				hi = v
			}
		}
		if lo < min || hi > max || lo > hi {
			return nil, false
		}
		for v := lo; v <= hi; v += step {
			out[v] = true
		}
	}
	return out, len(out) > 0
}

func cronAtoi(s string, names map[string]int) (int, bool) {
	if names != nil {
		if v, ok := names[strings.ToUpper(s)]; ok {
			return v, true
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}
