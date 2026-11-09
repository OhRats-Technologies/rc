package server

import (
	"errors"
	"time"
)

const maxAuthLifetime = 366 * 24 * time.Hour

func authLifetime(value, fallback string, allowNever bool) (time.Time, error) {
	selected := value
	if selected == "" {
		selected = fallback
	}
	if selected == "never" {
		if !allowNever {
			return time.Time{}, errors.New("authorization lifetime cannot be never")
		}
		return time.Time{}, nil
	}
	var duration time.Duration
	switch selected {
	case "1h":
		duration = time.Hour
	case "1d":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	case "90d":
		duration = 90 * 24 * time.Hour
	case "180d":
		duration = 180 * 24 * time.Hour
	case "1y":
		duration = 365 * 24 * time.Hour
	default:
		return time.Time{}, errors.New("invalid authorization lifetime")
	}
	if duration > maxAuthLifetime {
		return time.Time{}, errors.New("authorization lifetime is too long")
	}
	return time.Now().UTC().Add(duration), nil
}
