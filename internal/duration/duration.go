// Package duration provides human-friendly duration parsing shared across
// exec-utils commands. Wraps github.com/xhit/go-str2duration and adds
// normalization for long-form spellings (1day, 5 seconds, 1h30min, ...).
package duration

import (
	"strings"
	"time"

	str2duration "github.com/xhit/go-str2duration/v2"
)

// Normalize common human unit spellings to the compact forms xhit/go-str2duration understands (ns, us, ms, s, m, h,
// d, w). Longer aliases must precede shorter ones -- strings.NewReplacer picks the first matching pattern at each
// input position, so "seconds" wins over "second" wins over "sec" for the same position.
var unitAliases = strings.NewReplacer(
	" ", "",
	"\t", "",
	"seconds", "s",
	"second", "s",
	"secs", "s",
	"sec", "s",
	"minutes", "m",
	"minute", "m",
	"mins", "m",
	"min", "m",
	"hours", "h",
	"hour", "h",
	"hrs", "h",
	"hr", "h",
	"days", "d",
	"day", "d",
	"weeks", "w",
	"week", "w",
)

// Parse a human duration string. Heavy lifting is done by github.com/xhit/go-str2duration for units up to weeks;
// long-form spellings ("1day", "5 seconds", "1h30min") get normalized down to compact units first. Negative values
// fold to their absolute magnitude.
func Parse(s string) (time.Duration, error) {
	d, err := str2duration.ParseDuration(unitAliases.Replace(s))
	if err != nil {
		return 0, err
	}
	if d < 0 {
		d = -d
	}
	return d, nil
}
