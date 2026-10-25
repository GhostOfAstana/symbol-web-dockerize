package monitor

import "math"

// Score применяет интервалы ТЗ. Проценты CPU выше 100 допустимы на нескольких ядрах.
func Score(container string, metrics Metrics) (Report, error) {
	if !finiteNonnegative(metrics.UptimeSeconds) || !finiteNonnegative(metrics.MemoryPercent) ||
		metrics.MemoryPercent > 100 || !finiteNonnegative(metrics.CPUPercent) || metrics.RestartCount < 0 {
		return Report{}, ErrInvalidMetrics
	}
	scores := Scores{Memory: percentScore(metrics.MemoryPercent), CPU: percentScore(metrics.CPUPercent)}
	switch {
	case metrics.RestartCount == 0:
		scores.Restarts = 25
	case metrics.RestartCount <= 2:
		scores.Restarts = 15
	case metrics.RestartCount <= 5:
		scores.Restarts = 5
	}
	switch {
	case metrics.UptimeSeconds >= 3600:
		scores.Uptime = 25
	case metrics.UptimeSeconds >= 600:
		scores.Uptime = 15
	case metrics.UptimeSeconds >= 60:
		scores.Uptime = 5
	}
	total := scores.Memory + scores.CPU + scores.Restarts + scores.Uptime
	return Report{Container: container, Status: "running", Metrics: metrics,
		Scores: scores, HealthScore: total, Rating: Rating(total)}, nil
}

func finiteNonnegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func percentScore(value float64) int {
	switch {
	case value < 50:
		return 25
	case value < 75:
		return 15
	case value < 90:
		return 5
	default:
		return 0
	}
}

// Rating зависит только от суммы: звёзды не могут противоречить баллам.
func Rating(total int) string {
	switch {
	case total >= 90:
		return "5 stars (Excellent)"
	case total >= 80:
		return "4 stars (Good)"
	case total >= 70:
		return "3 stars (Fair)"
	case total >= 60:
		return "2 stars (Poor)"
	default:
		return "1 star (Critical)"
	}
}
