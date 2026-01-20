package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CronSchedule struct {
	Minutes []int
	Hours   []int
}

func ParseCronExpr(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 2 {
		return nil, fmt.Errorf("invalid cron expression, expected 2 fields (minute hour), got %d", len(fields))
	}

	schedule := &CronSchedule{}
	var err error

	schedule.Minutes, err = parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	schedule.Hours, err = parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	return schedule, nil
}

func parseField(field string, min, max int) ([]int, error) {
	var result []int

	// asterisk (*)
	if field == "*" {
		for i := min; i <= max; i++ {
			result = append(result, i)
		}
		return result, nil
	}

	// step values (*/5) every 5 minutes
	if strings.HasPrefix(field, "*/") {
		stepStr := strings.TrimPrefix(field, "*/")
		step, err := strconv.Atoi(stepStr)
		if err != nil {
			return nil, fmt.Errorf("invalid step value: %s", stepStr)
		}
		if step <= 0 {
			return nil, fmt.Errorf("step must be positive")
		}
		for i := min; i <= max; i += step {
			result = append(result, i)
		}
		return result, nil
	}

	// Handle single value
	val, err := strconv.Atoi(field)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", field)
	}
	if val < min || val > max {
		return nil, fmt.Errorf("value %d out of range [%d-%d]", val, min, max)
	}
	return []int{val}, nil
}

func (cs *CronSchedule) Next(after time.Time) time.Time {
	// Start from the next minute
	t := after.Add(time.Minute).Truncate(time.Minute)

	// Limit iterations to prevent infinite loops (check next 48 hours)
	for i := 0; i < 2880; i++ { // 48 hours * 60 minutes
		if cs.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}

	// Fallback: return far future if no match found
	return after.Add(48 * time.Hour)
}

// matches checks if the given time matches the cron schedule
func (cs *CronSchedule) matches(t time.Time) bool {
	if !contains(cs.Minutes, t.Minute()) {
		return false
	}

	if !contains(cs.Hours, t.Hour()) {
		return false
	}

	return true
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
