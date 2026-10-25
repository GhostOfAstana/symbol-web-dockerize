// Package monitor читает показатели Docker и объясняет оценку состояния контейнера.
package monitor

import "errors"

var (
	ErrUnavailable    = errors.New("Docker недоступен: проверьте daemon, права и имя контейнера")
	ErrNotRunning     = errors.New("контейнер не запущен: оценка состояния недоступна")
	ErrInvalidMetrics = errors.New("Docker вернул некорректные показатели")
)

type Metrics struct {
	UptimeSeconds float64 `json:"uptime_seconds"`
	MemoryPercent float64 `json:"memory_percent"`
	CPUPercent    float64 `json:"cpu_percent"`
	RestartCount  int     `json:"restart_count"`
}

type Scores struct {
	Uptime   int `json:"uptime"`
	Memory   int `json:"memory"`
	CPU      int `json:"cpu"`
	Restarts int `json:"restarts"`
}

type Report struct {
	Container   string  `json:"container"`
	Status      string  `json:"status"`
	Metrics     Metrics `json:"metrics"`
	Scores      Scores  `json:"scores"`
	HealthScore int     `json:"health_score"`
	Rating      string  `json:"rating"`
}
