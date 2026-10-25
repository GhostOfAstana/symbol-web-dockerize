package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Timeout = 8 * time.Second
const maxDockerOutput = 64 << 10

var containerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

type Runner interface {
	Run(context.Context, ...string) ([]byte, error)
}

// CommandRunner передаёт аргументы напрямую Docker: shell и интерполяция не используются.
type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", args...)
	command.WaitDelay = time.Second
	output := &limitedBuffer{remaining: maxDockerOutput}
	errorsOutput := &limitedBuffer{remaining: 8 << 10}
	command.Stdout, command.Stderr = output, errorsOutput
	if err := command.Run(); err != nil {
		return nil, ErrUnavailable
	}
	return output.Bytes(), nil
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	remaining int
}

func (buffer *limitedBuffer) Bytes() []byte { return buffer.buffer.Bytes() }

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	if len(data) > buffer.remaining {
		return 0, errors.New("превышен предел вывода Docker")
	}
	buffer.remaining -= len(data)
	return buffer.buffer.Write(data)
}

type Client struct {
	Runner Runner
	Now    func() time.Time
}

func NewClient() *Client { return &Client{Runner: CommandRunner{}, Now: time.Now} }

func (client *Client) Collect(ctx context.Context, name string) (Report, error) {
	if !containerName.MatchString(name) {
		return Report{}, errors.New("некорректное имя контейнера")
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	inspected, err := client.Runner.Run(ctx, "container", "inspect", "--format", "{{json .}}", "--", name)
	if err != nil {
		return Report{}, fmt.Errorf("inspect: %w", err)
	}
	var inspection struct {
		State struct {
			Status    string
			Running   bool
			StartedAt string
		}
		RestartCount int
	}
	if err := decode(inspected, &inspection); err != nil {
		return Report{}, ErrInvalidMetrics
	}
	if !inspection.State.Running || inspection.State.Status != "running" {
		return Report{}, ErrNotRunning
	}
	started, err := time.Parse(time.RFC3339Nano, inspection.State.StartedAt)
	if err != nil || started.IsZero() {
		return Report{}, ErrInvalidMetrics
	}
	uptime := client.Now().Sub(started).Seconds()
	if uptime < 0 {
		return Report{}, ErrInvalidMetrics
	}
	statistics, err := client.Runner.Run(ctx, "stats", "--no-stream", "--format", "{{json .}}", "--", name)
	if err != nil {
		return Report{}, fmt.Errorf("stats: %w", err)
	}
	var stats struct {
		MemPerc string
		CPUPerc string
	}
	if err := decode(statistics, &stats); err != nil {
		return Report{}, ErrInvalidMetrics
	}
	memory, err := percentage(stats.MemPerc)
	if err != nil {
		return Report{}, err
	}
	cpu, err := percentage(stats.CPUPerc)
	if err != nil {
		return Report{}, err
	}
	return Score(name, Metrics{UptimeSeconds: uptime, MemoryPercent: memory,
		CPUPercent: cpu, RestartCount: inspection.RestartCount})
}

func decode(data []byte, destination any) error {
	if len(data) == 0 || len(data) > maxDockerOutput {
		return ErrInvalidMetrics
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ErrInvalidMetrics
	}
	return nil
}

func percentage(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, "%") {
		return 0, ErrInvalidMetrics
	}
	result, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	if err != nil || !finiteNonnegative(result) {
		return 0, ErrInvalidMetrics
	}
	return result, nil
}
