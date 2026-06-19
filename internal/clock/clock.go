package clock

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ConfidenceHostClock    = "host_clock"
	ConfidenceMonotonic    = "monotonic"
	ConfidenceWallFallback = "wall_clock_fallback"
	ConfidenceUnknown      = "unknown"
)

var durationPart = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h|d)`)

func Now() time.Time {
	return time.Now().UTC()
}

func FormatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func ParseRFC3339(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func ParseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("duration is required")
	}

	matches := durationPart.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration %q", value)
	}

	var total time.Duration
	pos := 0
	for _, match := range matches {
		if match[0] != pos {
			return 0, fmt.Errorf("invalid duration %q", value)
		}

		numberText := value[match[2]:match[3]]
		unit := strings.ToLower(value[match[4]:match[5]])
		number, err := strconv.ParseFloat(numberText, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", value, err)
		}

		var unitDuration time.Duration
		switch unit {
		case "ns":
			unitDuration = time.Nanosecond
		case "us", "µs":
			unitDuration = time.Microsecond
		case "ms":
			unitDuration = time.Millisecond
		case "s":
			unitDuration = time.Second
		case "m":
			unitDuration = time.Minute
		case "h":
			unitDuration = time.Hour
		case "d":
			unitDuration = 24 * time.Hour
		default:
			return 0, fmt.Errorf("invalid duration unit %q", unit)
		}

		part := number * float64(unitDuration)
		if part > float64(math.MaxInt64) {
			return 0, fmt.Errorf("duration %q is too large", value)
		}
		total += time.Duration(part)
		pos = match[1]
	}

	if pos != len(value) || total <= 0 {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return total, nil
}

func FormatDuration(d time.Duration) string {
	if d < 0 {
		return "-" + FormatDuration(-d)
	}
	if d < time.Second {
		return d.String()
	}

	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, " ")
}
