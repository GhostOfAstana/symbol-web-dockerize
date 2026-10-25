package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestScoreWorkedExamples(t *testing.T) {
	cases := []struct {
		name    string
		metrics Metrics
		score   int
		rating  string
	}{
		{"healthy", Metrics{9258, 8.8, 2.3, 0}, 100, "5 stars (Excellent)"},
		{"degraded", Metrics{300, 82, 60, 3}, 30, "1 star (Critical)"},
		{"new", Metrics{0, 0, 0, 0}, 75, "3 stars (Fair)"},
		{"overloaded", Metrics{0, 95, 200, 6}, 0, "1 star (Critical)"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			report, err := Score("app", test.metrics)
			if err != nil {
				t.Fatal(err)
			}
			if report.HealthScore != test.score || report.Rating != test.rating {
				t.Fatalf("получено %+v", report)
			}
			if report.HealthScore != report.Scores.CPU+report.Scores.Memory+report.Scores.Restarts+report.Scores.Uptime {
				t.Fatal("сумма расходится")
			}
		})
	}
}

func TestScoreBoundaries(t *testing.T) {
	for _, test := range []struct {
		percent  float64
		expected int
	}{
		{0, 25}, {49.999, 25}, {50, 15}, {74.999, 15}, {75, 5}, {89.999, 5}, {90, 0}, {100, 0}, {450, 0},
	} {
		if actual := percentScore(test.percent); actual != test.expected {
			t.Errorf("%f: %d", test.percent, actual)
		}
	}
	for _, test := range []struct {
		seconds  float64
		expected int
	}{
		{0, 0}, {59.999, 0}, {60, 5}, {599.999, 5}, {600, 15}, {3599.999, 15}, {3600, 25},
	} {
		report, err := Score("app", Metrics{UptimeSeconds: test.seconds})
		if err != nil || report.Scores.Uptime != test.expected {
			t.Errorf("%f: %+v %v", test.seconds, report, err)
		}
	}
	for _, test := range []struct{ count, expected int }{{0, 25}, {1, 15}, {2, 15}, {3, 5}, {5, 5}, {6, 0}} {
		report, err := Score("app", Metrics{RestartCount: test.count})
		if err != nil || report.Scores.Restarts != test.expected {
			t.Errorf("%d: %+v %v", test.count, report, err)
		}
	}
	for _, test := range []struct {
		score    int
		expected string
	}{{59, "1 star (Critical)"}, {60, "2 stars (Poor)"}, {69, "2 stars (Poor)"}, {70, "3 stars (Fair)"}, {79, "3 stars (Fair)"}, {80, "4 stars (Good)"}, {89, "4 stars (Good)"}, {90, "5 stars (Excellent)"}, {100, "5 stars (Excellent)"}} {
		if actual := Rating(test.score); actual != test.expected {
			t.Errorf("%d: %s", test.score, actual)
		}
	}
}

func TestScoreRejectsInvalidMetrics(t *testing.T) {
	for _, metrics := range []Metrics{
		{UptimeSeconds: -1}, {MemoryPercent: 101}, {MemoryPercent: math.NaN()},
		{CPUPercent: math.Inf(1)}, {RestartCount: -1}, {UptimeSeconds: math.Inf(1)},
	} {
		if _, err := Score("app", metrics); !errors.Is(err, ErrInvalidMetrics) {
			t.Fatalf("%+v: %v", metrics, err)
		}
	}
}

type fakeRunner struct {
	outputs [][]byte
	err     error
	calls   [][]string
}

func (runner *fakeRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("нет таймаута")
	}
	runner.calls = append(runner.calls, args)
	if runner.err != nil {
		return nil, runner.err
	}
	if len(runner.outputs) == 0 {
		return nil, errors.New("лишний вызов")
	}
	result := runner.outputs[0]
	runner.outputs = runner.outputs[1:]
	return result, nil
}

func TestCollectRealCommandContract(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	runner := &fakeRunner{outputs: [][]byte{
		[]byte(`{"State":{"Running":true,"Status":"running","StartedAt":"2026-10-25T10:00:00Z"},"RestartCount":0}`),
		[]byte(`{"MemPerc":"8.8%","CPUPerc":"2.3%"}`),
	}}
	client := &Client{Runner: runner, Now: func() time.Time { return now }}
	report, err := client.Collect(context.Background(), "audit-app")
	if err != nil {
		t.Fatal(err)
	}
	if report.HealthScore != 100 || report.Metrics.UptimeSeconds != 7200 {
		t.Fatalf("%+v", report)
	}
	expected := [][]string{
		{"container", "inspect", "--format", "{{json .}}", "--", "audit-app"},
		{"stats", "--no-stream", "--format", "{{json .}}", "--", "audit-app"},
	}
	if !reflect.DeepEqual(runner.calls, expected) {
		t.Fatalf("%v", runner.calls)
	}
}

func TestCollectFailureCases(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	valid := []byte(`{"State":{"Running":true,"Status":"running","StartedAt":"2026-10-25T10:00:00Z"},"RestartCount":0}`)
	for _, test := range []struct {
		name    string
		outputs [][]byte
		err     error
	}{
		{"missing-daemon", nil, ErrUnavailable},
		{"empty", [][]byte{nil}, nil},
		{"malformed", [][]byte{[]byte("{")}, nil},
		{"stopped", [][]byte{[]byte(`{"State":{"Running":false,"Status":"exited"}}`)}, nil},
		{"future", [][]byte{[]byte(`{"State":{"Running":true,"Status":"running","StartedAt":"2027-10-25T10:00:00Z"}}`)}, nil},
		{"negative-restarts", [][]byte{[]byte(`{"State":{"Running":true,"Status":"running","StartedAt":"2026-10-25T10:00:00Z"},"RestartCount":-1}`), []byte(`{"MemPerc":"8.8%","CPUPerc":"2.3%"}`)}, nil},
		{"bad-percent", [][]byte{valid, []byte(`{"MemPerc":"NaN%","CPUPerc":"0%"}`)}, nil},
		{"missing-percent", [][]byte{valid, []byte(`{"MemPerc":"8.8","CPUPerc":"0%"}`)}, nil},
		{"trailing-json", [][]byte{append(append([]byte{}, valid...), []byte(" {}")...)}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &Client{Runner: &fakeRunner{outputs: test.outputs, err: test.err}, Now: func() time.Time { return now }}
			if _, err := client.Collect(context.Background(), "app"); err == nil {
				t.Fatal("ожидалась ошибка")
			}
		})
	}
}

func TestContainerNameValidation(t *testing.T) {
	for _, name := range []string{"", "--help", "a;touch /tmp/pwn", "$(id)", "a\nb", strings.Repeat("a", 129), "../app"} {
		runner := &fakeRunner{}
		client := &Client{Runner: runner, Now: time.Now}
		if _, err := client.Collect(context.Background(), name); err == nil || len(runner.calls) != 0 {
			t.Fatalf("передано опасное имя %q", name)
		}
	}
}

func TestLimitedBuffer(t *testing.T) {
	buffer := &limitedBuffer{remaining: 3}
	if _, err := buffer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Write([]byte("d")); err == nil {
		t.Fatal("предел не применён")
	}
	if string(buffer.Bytes()) != "abc" {
		t.Fatal("повреждён накопленный вывод")
	}
}

func TestLint(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Dockerfile")
	good := "FROM golang:1.26-alpine AS build\nRUN go build \\\n+  -o /app/server ./cmd/server\nFROM alpine:3.23\n"
	if err := os.WriteFile(path, []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".dockerignore"), []byte(".git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err := Lint(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if !finding.Pass || finding.Benefit == "" {
			t.Fatalf("%+v", finding)
		}
	}
	after, _ := os.ReadFile(path)
	if string(after) != good {
		t.Fatal("линтер изменил Dockerfile")
	}
	if err := os.Remove(filepath.Join(directory, ".dockerignore")); err != nil {
		t.Fatal(err)
	}
	bad := "FROM golang:latest\n" + strings.Repeat("RUN true\n", 6)
	findings, err = lint(strings.NewReader(bad), directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.Pass {
			t.Fatalf("ожидался FAIL: %+v", finding)
		}
	}
	if _, err := Lint(filepath.Join(directory, "missing")); err == nil {
		t.Fatal("нет ошибки отсутствия файла")
	}
	if _, err := lint(strings.NewReader(strings.Repeat("#x\n", 400000)), directory); err == nil {
		t.Fatal("нет предела Dockerfile")
	}
}

func FuzzScore(f *testing.F) {
	f.Add(9258.0, 8.8, 2.3, 0)
	f.Add(300.0, 82.0, 60.0, 3)
	f.Fuzz(func(t *testing.T, uptime, memory, cpu float64, restarts int) {
		report, err := Score("fuzz", Metrics{uptime, memory, cpu, restarts})
		if err != nil {
			return
		}
		if report.HealthScore < 0 || report.HealthScore > 100 || report.Rating != Rating(report.HealthScore) {
			t.Fatalf("%+v", report)
		}
		if _, err := json.Marshal(report); err != nil {
			t.Fatal(err)
		}
	})
}

func FuzzDockerJSON(f *testing.F) {
	f.Add(`{"MemPerc":"8.8%","CPUPerc":"2.3%"}`)
	f.Fuzz(func(t *testing.T, value string) {
		var result struct{ MemPerc, CPUPerc string }
		_ = decode([]byte(value), &result)
		_, _ = percentage(result.MemPerc)
	})
}
