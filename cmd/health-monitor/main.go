package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/GhostOfAstana/symbol-web-dockerize/internal/monitor"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errorsOut io.Writer) int {
	flags := flag.NewFlagSet("health-monitor", flag.ContinueOnError)
	flags.SetOutput(errorsOut)
	lintPath := flags.String("lint", "", "проверить Dockerfile и соседний .dockerignore")
	jsonOutput := flags.Bool("json", false, "вывести показатели в JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *lintPath != "" {
		if flags.NArg() != 0 || *jsonOutput {
			fmt.Fprintln(errorsOut, "Для --lint укажите только путь Dockerfile.")
			return 2
		}
		findings, err := monitor.Lint(*lintPath)
		if err != nil {
			fmt.Fprintln(errorsOut, err)
			return 1
		}
		fmt.Fprintln(out, "Проверка Dockerfile")
		passed := 0
		for _, finding := range findings {
			state := "FAIL"
			if finding.Pass {
				state = "PASS"
				passed++
			}
			fmt.Fprintf(out, "[%s] %s — польза: %s\n", state, finding.Name, finding.Benefit)
		}
		fmt.Fprintf(out, "Пройдено: %d/%d\n", passed, len(findings))
		if passed != len(findings) {
			return 1
		}
		return 0
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(errorsOut, "Использование: health-monitor [--json] <контейнер> | --lint Dockerfile")
		return 2
	}
	report, err := monitor.NewClient().Collect(context.Background(), flags.Arg(0))
	if err != nil {
		fmt.Fprintln(errorsOut, err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(out).Encode(report); err != nil {
			fmt.Fprintln(errorsOut, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(out, "Монитор состояния контейнера\nКонтейнер: %s (%s)\n\n", report.Container, report.Status)
	fmt.Fprintf(out, "Время работы: %s (%d/25)\n", (time.Duration(report.Metrics.UptimeSeconds) * time.Second).Truncate(time.Second), report.Scores.Uptime)
	fmt.Fprintf(out, "Память: %.2f%% (%d/25)\nCPU: %.2f%% (%d/25)\n", report.Metrics.MemoryPercent, report.Scores.Memory, report.Metrics.CPUPercent, report.Scores.CPU)
	fmt.Fprintf(out, "Перезапуски: %d (%d/25)\n\nОценка: %d/100\nРейтинг: %s\n", report.Metrics.RestartCount, report.Scores.Restarts, report.HealthScore, report.Rating)
	return 0
}
