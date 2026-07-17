package main

import (
	"fmt"
	"strings"
)

func isValidTimeFormat(t string) bool {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return false
	}

	var hour, minute int
	if _, err := fmt.Sscanf(t, "%d:%d", &hour, &minute); err != nil {
		return false
	}

	return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59
}
