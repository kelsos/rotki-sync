package utils

import "time"

func HasEnoughTimeElapsed(epochTime int64) bool {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	timeFromEpoch := time.Unix(epochTime, 0)

	isFromPreviousDay := timeFromEpoch.Year() == yesterday.Year() &&
		timeFromEpoch.Month() == yesterday.Month() &&
		timeFromEpoch.Day() == yesterday.Day()

	isAtLeast20HoursAgo := now.Sub(timeFromEpoch) > 20*time.Hour

	return isFromPreviousDay && isAtLeast20HoursAgo
}
