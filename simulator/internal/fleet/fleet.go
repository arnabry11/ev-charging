// Package fleet derives the identities and power levels of the virtual chargers.
package fleet

import (
	"fmt"
	"strconv"
	"strings"
)

// powerPercent spreads chargers around the base power so their live lines differ.
var powerPercent = [...]int64{100, 50, 150, 300, 75}

// ChargerIDs returns count ids that count up from base by its trailing number,
// keeping the width of that number (CHG-MUM-0009 is followed by CHG-MUM-0010).
func ChargerIDs(base string, count int) ([]string, error) {
	if count < 1 {
		return nil, fmt.Errorf("charger count must be at least 1, got %d", count)
	}
	if count == 1 {
		return []string{base}, nil
	}
	digits := len(base) - len(strings.TrimRight(base, "0123456789"))
	if digits == 0 {
		return nil, fmt.Errorf("charger id %q needs a trailing number to run %d chargers", base, count)
	}
	prefix, number := base[:len(base)-digits], base[len(base)-digits:]
	start, err := strconv.Atoi(number)
	if err != nil {
		return nil, fmt.Errorf("charger id %q: %w", base, err)
	}
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%0*d", prefix, digits, start+i)
	}
	return ids, nil
}

// PowerW returns the charging power for the charger at index. Index 0 keeps baseW.
func PowerW(baseW int64, index int) int64 {
	return baseW * powerPercent[index%len(powerPercent)] / 100
}
