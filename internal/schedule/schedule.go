// Package schedule holds the pure time math for launch windows: whether a
// moment is inside a game's window, when its next window starts, and the
// labels shown for that. It has no state and no side effects.
package schedule

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"frictionless-launcher/internal/config"
)

// InWindow reports whether now falls inside one of game's schedule windows.
// Windows include their end minute (19:00-21:00 includes 21:00).
func InWindow(game config.Game, now time.Time) bool {
	currentTime := now.Format("15:04")
	currentDay := now.Weekday().String()[:3]

	for _, schedule := range game.Schedules {
		dayMatches := false
		for _, day := range schedule.Days {
			if strings.EqualFold(day, currentDay) {
				dayMatches = true
				break
			}
		}
		if !dayMatches {
			continue
		}
		if currentTime >= schedule.StartTime && currentTime <= schedule.EndTime {
			return true
		}
	}
	return false
}

// LaunchedInWindow reports whether lastLaunch falls inside the schedule
// window that now is in, i.e. whether game was already launched (or the
// launch cancelled) during the current window.
func LaunchedInWindow(game config.Game, lastLaunch, now time.Time) bool {
	for _, schedule := range game.Schedules {
		dayMatches := false
		for _, day := range schedule.Days {
			if strings.EqualFold(day, now.Weekday().String()[:3]) {
				dayMatches = true
				break
			}
		}

		if !dayMatches {
			continue
		}

		startTime, _ := time.Parse("15:04", schedule.StartTime)
		endTime, _ := time.Parse("15:04", schedule.EndTime)

		startTime = time.Date(now.Year(), now.Month(), now.Day(), startTime.Hour(), startTime.Minute(), 0, 0, now.Location())
		endTime = time.Date(now.Year(), now.Month(), now.Day(), endTime.Hour(), endTime.Minute(), 0, 0, now.Location())

		// A game with more than one window today only counts as "already
		// launched" against whichever window now actually falls in — a
		// launch recorded during an earlier window must not suppress a
		// later, distinct window for the same game later the same day.
		if now.Before(startTime) || now.After(endTime.Add(time.Minute)) {
			continue
		}

		if lastLaunch.After(startTime) && lastLaunch.Before(endTime.Add(time.Minute)) {
			return true
		}
	}

	return false
}

// NextStart returns the next time one of game's windows starts, strictly
// after from and within the next 7 days.
func NextStart(game config.Game, from time.Time) (time.Time, bool) {
	for daysAhead := 0; daysAhead <= 7; daysAhead++ {
		day := from.AddDate(0, 0, daysAhead)
		dayName := day.Weekday().String()[:3]
		var earliest time.Time
		found := false
		for _, s := range game.Schedules {
			for _, d := range s.Days {
				if !strings.EqualFold(d, dayName) {
					continue
				}
				startH, startM := 0, 0
				fmt.Sscanf(s.StartTime, "%d:%d", &startH, &startM)
				candidate := time.Date(day.Year(), day.Month(), day.Day(), startH, startM, 0, 0, day.Location())
				if !candidate.After(from) {
					continue
				}
				if !found || candidate.Before(earliest) {
					earliest = candidate
					found = true
				}
			}
		}
		if found {
			return earliest, true
		}
	}
	return time.Time{}, false
}

// NextLabel returns a short human-readable label for game's next window
// start: "Today 19:00" when it's within 24 hours, else e.g. "Thu 19:00", or
// "unscheduled".
func NextLabel(game config.Game, now time.Time) string {
	t, ok := NextStart(game, now)
	if !ok {
		return "unscheduled"
	}
	if t.Before(now.Add(24 * time.Hour)) {
		return "Today " + t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

// Overlaps reports whether [start1,end1] and [start2,end2] share any time.
// Times are "HH:MM" strings, which compare correctly as strings.
func Overlaps(start1, end1, start2, end2 string) bool {
	return start1 < end2 && start2 < end1
}

// ValidClock reports whether t is a 24-hour "HH:MM" time.
func ValidClock(t string) bool {
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

// Upcoming returns up to n enabled games, ordered by their next window start.
// Games with no window in the next 7 days are left out.
func Upcoming(games []config.Game, n int, now time.Time) []config.Game {
	type candidate struct {
		game config.Game
		next time.Time
	}
	var candidates []candidate
	for _, game := range games {
		if !game.Enabled {
			continue
		}
		if t, ok := NextStart(game, now); ok {
			candidates = append(candidates, candidate{game, t})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].next.Before(candidates[j].next)
	})
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	result := make([]config.Game, len(candidates))
	for i, c := range candidates {
		result[i] = c.game
	}
	return result
}

// Conflict returns the name of any game in games whose schedule overlaps the
// given days and times, ignoring the game named skipName (the one being
// edited). It returns "" if there is no conflict.
func Conflict(games []config.Game, skipName string, days []string, startTime, endTime string) string {
	for _, existing := range games {
		if existing.GameName == skipName {
			continue
		}
		for _, es := range existing.Schedules {
			for _, eDay := range es.Days {
				for _, newDay := range days {
					if strings.EqualFold(eDay, newDay) && Overlaps(startTime, endTime, es.StartTime, es.EndTime) {
						return existing.GameName
					}
				}
			}
		}
	}
	return ""
}

// WarnOverlaps logs a warning for every pair of games whose windows overlap.
// The editor refuses to save overlaps, but a hand-edited config.yaml can
// still contain them.
func WarnOverlaps(games []config.Game) {
	for i := 0; i < len(games); i++ {
		for _, si := range games[i].Schedules {
			for _, di := range si.Days {
				for j := i + 1; j < len(games); j++ {
					for _, sj := range games[j].Schedules {
						for _, dj := range sj.Days {
							if strings.EqualFold(di, dj) && Overlaps(si.StartTime, si.EndTime, sj.StartTime, sj.EndTime) {
								log.Printf("WARNING: schedule overlap between %q and %q on %s (%s-%s vs %s-%s)",
									games[i].GameName, games[j].GameName,
									di, si.StartTime, si.EndTime, sj.StartTime, sj.EndTime)
							}
						}
					}
				}
			}
		}
	}
}
