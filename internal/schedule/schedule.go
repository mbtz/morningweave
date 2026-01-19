package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	minutesPerYear = 525600
)

var monthNames = map[string]int{
	"jan": 1,
	"feb": 2,
	"mar": 3,
	"apr": 4,
	"may": 5,
	"jun": 6,
	"jul": 7,
	"aug": 8,
	"sep": 9,
	"oct": 10,
	"nov": 11,
	"dec": 12,
}

var dowNames = map[string]int{
	"sun": 0,
	"mon": 1,
	"tue": 2,
	"wed": 3,
	"thu": 4,
	"fri": 5,
	"sat": 6,
}

// Spec wraps a parsed cron schedule.
type Spec struct {
	raw    string
	minute []bool
	hour   []bool
	dom    []bool
	month  []bool
	dow    []bool
	domAny bool
	dowAny bool
}

// Parse converts a 5-field cron string into a Spec.
func Parse(spec string) (Spec, error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return Spec{}, fmt.Errorf("schedule is empty")
	}

	fields := strings.Fields(trimmed)
	if len(fields) != 5 {
		return Spec{}, fmt.Errorf("schedule must have 5 fields, got %d", len(fields))
	}

	minutes, _, err := parseField(fields[0], 0, 59, nil)
	if err != nil {
		return Spec{}, fmt.Errorf("minute: %w", err)
	}
	hours, _, err := parseField(fields[1], 0, 23, nil)
	if err != nil {
		return Spec{}, fmt.Errorf("hour: %w", err)
	}
	dom, domAny, err := parseField(fields[2], 1, 31, nil)
	if err != nil {
		return Spec{}, fmt.Errorf("day-of-month: %w", err)
	}
	months, _, err := parseField(fields[3], 1, 12, monthNames)
	if err != nil {
		return Spec{}, fmt.Errorf("month: %w", err)
	}
	dow, dowAny, err := parseField(fields[4], 0, 6, dowNames)
	if err != nil {
		return Spec{}, fmt.Errorf("day-of-week: %w", err)
	}

	return Spec{
		raw:    trimmed,
		minute: minutes,
		hour:   hours,
		dom:    dom,
		month:  months,
		dow:    dow,
		domAny: domAny,
		dowAny: dowAny,
	}, nil
}

// Next returns the next execution time after the provided instant.
func (s Spec) Next(from time.Time) (time.Time, error) {
	start := from.Truncate(time.Minute).Add(time.Minute)
	current := start
	for i := 0; i < minutesPerYear*5; i++ {
		if s.matches(current) {
			return current, nil
		}
		current = current.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no run time found within %d minutes", minutesPerYear*5)
}

func (s Spec) String() string {
	return s.raw
}

// NextRun parses the spec and returns the next run after the provided instant.
func NextRun(spec string, from time.Time) (time.Time, error) {
	parsed, err := Parse(spec)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.Next(from)
}

func (s Spec) matches(t time.Time) bool {
	if !s.minute[t.Minute()] || !s.hour[t.Hour()] {
		return false
	}
	if !s.month[int(t.Month())] {
		return false
	}

	day := t.Day()
	dow := int(t.Weekday())
	domMatch := s.dom[day]
	dowMatch := s.dow[dow]

	if s.domAny && s.dowAny {
		return true
	}
	if s.domAny {
		return dowMatch
	}
	if s.dowAny {
		return domMatch
	}
	return domMatch || dowMatch
}

func parseField(value string, min int, max int, names map[string]int) ([]bool, bool, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return nil, false, fmt.Errorf("field is empty")
	}
	if trimmed == "?" || trimmed == "*" {
		return allowAll(min, max), true, nil
	}

	allowed := make([]bool, max+1)
	parts := strings.Split(trimmed, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		base, step := part, 0
		if strings.Contains(part, "/") {
			segments := strings.Split(part, "/")
			if len(segments) != 2 {
				return nil, false, fmt.Errorf("invalid step syntax: %s", part)
			}
			base = strings.TrimSpace(segments[0])
			stepValue := strings.TrimSpace(segments[1])
			if stepValue == "" {
				return nil, false, fmt.Errorf("missing step value in %s", part)
			}
			parsed, err := strconv.Atoi(stepValue)
			if err != nil || parsed <= 0 {
				return nil, false, fmt.Errorf("invalid step value %s", stepValue)
			}
			step = parsed
		}

		start := min
		end := max
		if base != "" && base != "*" && base != "?" {
			if strings.Contains(base, "-") {
				rangeParts := strings.Split(base, "-")
				if len(rangeParts) != 2 {
					return nil, false, fmt.Errorf("invalid range %s", base)
				}
				rangeStart, err := parseValue(rangeParts[0], min, max, names)
				if err != nil {
					return nil, false, err
				}
				rangeEnd, err := parseValue(rangeParts[1], min, max, names)
				if err != nil {
					return nil, false, err
				}
				if rangeStart > rangeEnd {
					return nil, false, fmt.Errorf("range start greater than end in %s", base)
				}
				start = rangeStart
				end = rangeEnd
			} else {
				val, err := parseValue(base, min, max, names)
				if err != nil {
					return nil, false, err
				}
				start = val
				end = val
			}
		}

		if step == 0 {
			step = 1
		}

		for i := start; i <= end; i += step {
			if i < min || i > max {
				continue
			}
			allowed[i] = true
		}
	}

	if !hasAny(allowed, min, max) {
		return nil, false, fmt.Errorf("no values selected")
	}
	return allowed, isAll(allowed, min, max), nil
}

func parseValue(raw string, min int, max int, names map[string]int) (int, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}
	if names != nil {
		if named, ok := names[value]; ok {
			return named, nil
		}
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid value %s", value)
	}
	if parsed == 7 && max == 6 {
		parsed = 0
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("value %d outside %d-%d", parsed, min, max)
	}
	return parsed, nil
}

func allowAll(min int, max int) []bool {
	allowed := make([]bool, max+1)
	for i := min; i <= max; i++ {
		allowed[i] = true
	}
	return allowed
}

func hasAny(values []bool, min int, max int) bool {
	for i := min; i <= max && i < len(values); i++ {
		if values[i] {
			return true
		}
	}
	return false
}

func isAll(values []bool, min int, max int) bool {
	for i := min; i <= max && i < len(values); i++ {
		if !values[i] {
			return false
		}
	}
	return true
}
