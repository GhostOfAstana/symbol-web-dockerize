package monitor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Finding struct {
	Name    string
	Pass    bool
	Benefit string
}

// Lint проверяет четыре учебных правила и никогда не меняет проверяемые файлы.
func Lint(path string) ([]Finding, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("открыть Dockerfile: %w", err)
	}
	defer file.Close()
	return lint(file, filepath.Dir(path))
}

func lint(reader io.Reader, directory string) ([]Finding, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, (1<<20)+1))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	stages, runs := 0, 0
	baseOK := true
	continuation := false
	size := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		size += len(scanner.Text()) + 1
		if size > 1<<20 {
			return nil, fmt.Errorf("Dockerfile превышает 1 МиБ")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if continuation {
			continuation = strings.HasSuffix(line, "\\")
			continue
		}
		fields := strings.Fields(line)
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			stages++
			index := 1
			if len(fields) > index && strings.HasPrefix(fields[index], "--platform=") {
				index++
			}
			if len(fields) <= index {
				baseOK = false
				continue
			}
			image := strings.ToLower(fields[index])
			if !strings.Contains(image, "alpine") && !strings.Contains(image, "slim") {
				baseOK = false
			}
		case "RUN":
			runs++
		}
		continuation = strings.HasSuffix(line, "\\")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("прочитать Dockerfile: %w", err)
	}
	info, err := os.Stat(filepath.Join(directory, ".dockerignore"))
	ignorePresent := err == nil && info.Mode().IsRegular()
	return []Finding{
		{"многоэтапная сборка", stages >= 2, "меньше файлов и размер итогового образа"},
		{"базовые образы alpine/slim", stages > 0 && baseOK, "меньше размер базового образа"},
		{fmt.Sprintf("RUN-слоёв не больше 5 (найдено %d)", runs), runs <= 5, "меньше слоёв образа"},
		{".dockerignore существует", ignorePresent, "меньше контекст сборки"},
	}, nil
}
