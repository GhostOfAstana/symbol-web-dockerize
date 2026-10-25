package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	assets "github.com/GhostOfAstana/symbol-web-dockerize"
	"github.com/GhostOfAstana/symbol-web-dockerize/internal/handlers"
	"github.com/GhostOfAstana/symbol-web-dockerize/internal/monitor"
)

func server(t *testing.T, collector handlers.Collector, container string) http.Handler {
	t.Helper()
	mux, err := handlers.New(assets.FS, collector, container)
	if err != nil {
		t.Fatal(err)
	}
	return handlers.SecureHeaders(mux)
}

func TestPagesAndMethods(t *testing.T) {
	application := server(t, nil, "")
	for _, test := range []struct {
		method, path string
		status       int
	}{{"GET", "/", 200}, {"GET", "/healthz", 200}, {"GET", "/missing", 404}, {"GET", "/symbol-art", 405}, {"POST", "/", 405}, {"GET", "/static/css/style.css", 200}, {"GET", "/static/js/script.js", 200}, {"GET", "/static/", 404}, {"GET", "/static/secrets", 404}, {"GET", "/health-monitor", 503}} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		application.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s: %d", test.method, test.path, response.Code)
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("нет защитных заголовков")
		}
	}
}

func TestGenerateAllBanners(t *testing.T) {
	application := server(t, nil, "")
	for _, banner := range []string{"standard", "shadow", "thinkertoy"} {
		request := httptest.NewRequest("POST", "/symbol-art", strings.NewReader(url.Values{"text": {"Hello\nDocker!"}, "banner": {banner}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		application.ServeHTTP(response, request)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `id="art-output"`) {
			t.Fatalf("%s: %d %s", banner, response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Length") != fmt.Sprint(response.Body.Len()) {
			t.Fatal("неверная длина ответа")
		}
	}
}

func TestBadInputAndXSS(t *testing.T) {
	application := server(t, nil, "")
	for _, test := range []struct {
		text, banner string
		status       int
	}{{"", "standard", 400}, {"Привет", "standard", 400}, {"a", "../../etc/passwd", 400}, {strings.Repeat("a", 4097), "standard", 400}, {"<script>alert(1)</script>", "standard", 200}} {
		request := httptest.NewRequest("POST", "/symbol-art", strings.NewReader(url.Values{"text": {test.text}, "banner": {test.banner}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		application.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("%q %q: %d", test.text, test.banner, response.Code)
		}
		if strings.Contains(response.Body.String(), "<script>alert(1)</script>") {
			t.Fatal("HTML пользователя выполнится")
		}
	}
	request := httptest.NewRequest("POST", "/symbol-art", strings.NewReader(strings.Repeat("x", 40<<10)))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	application.ServeHTTP(response, request)
	if response.Code != 413 {
		t.Fatal(response.Code)
	}
	request = httptest.NewRequest("POST", "/symbol-art", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	application.ServeHTTP(response, request)
	if response.Code != 400 {
		t.Fatal(response.Code)
	}
}

type fixedCollector struct{ err error }

func (collector fixedCollector) Collect(context.Context, string) (monitor.Report, error) {
	if collector.err != nil {
		return monitor.Report{}, collector.err
	}
	return monitor.Score("app", monitor.Metrics{UptimeSeconds: 7200, MemoryPercent: 8.8, CPUPercent: 2.3})
}

func TestHealthResponses(t *testing.T) {
	for _, test := range []struct {
		collector fixedCollector
		status    int
	}{{fixedCollector{}, 200}, {fixedCollector{errors.New("secret daemon path")}, 503}} {
		application := server(t, test.collector, "app")
		response := httptest.NewRecorder()
		application.ServeHTTP(response, httptest.NewRequest("GET", "/health-monitor", nil))
		if response.Code != test.status {
			t.Fatal(response.Code)
		}
		if strings.Contains(response.Body.String(), "secret daemon path") {
			t.Fatal("утечка внутренней ошибки")
		}
		if response.Code == 200 {
			var report monitor.Report
			if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.HealthScore != 100 {
				t.Fatalf("%+v %v", report, err)
			}
		}
	}
}

type blockingCollector struct {
	start   chan struct{}
	release chan struct{}
}

func (collector *blockingCollector) Collect(ctx context.Context, _ string) (monitor.Report, error) {
	close(collector.start)
	select {
	case <-collector.release:
		return monitor.Score("app", monitor.Metrics{})
	case <-ctx.Done():
		return monitor.Report{}, ctx.Err()
	}
}

func TestHealthConcurrencyBound(t *testing.T) {
	collector := &blockingCollector{make(chan struct{}), make(chan struct{})}
	application := server(t, collector, "app")
	done := make(chan struct{})
	go func() {
		defer close(done)
		application.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health-monitor", nil))
	}()
	<-collector.start
	response := httptest.NewRecorder()
	application.ServeHTTP(response, httptest.NewRequest("GET", "/health-monitor", nil))
	if response.Code != 503 || response.Header().Get("Retry-After") == "" {
		t.Fatal("не ограничены процессы Docker")
	}
	close(collector.release)
	<-done
}

func TestConcurrentGeneration(t *testing.T) {
	application := server(t, nil, "")
	var group sync.WaitGroup
	for range 64 {
		group.Go(func() {
			request := httptest.NewRequest("POST", "/symbol-art", strings.NewReader("text=Hello&banner=standard"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			application.ServeHTTP(response, request)
			if response.Code != 200 && response.Code != 503 {
				t.Errorf("%d", response.Code)
			}
		})
	}
	group.Wait()
}

type waitingBody struct {
	started chan struct{}
	release chan struct{}
}

func (body waitingBody) Read([]byte) (int, error) {
	body.started <- struct{}{}
	<-body.release
	return 0, io.EOF
}

func TestGenerationOverloadReturnsRetry(t *testing.T) {
	application := server(t, nil, "")
	started, release := make(chan struct{}, 8), make(chan struct{})
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			request := httptest.NewRequest("POST", "/symbol-art", waitingBody{started, release})
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			application.ServeHTTP(httptest.NewRecorder(), request)
		})
	}
	for range 8 {
		<-started
	}
	request := httptest.NewRequest("POST", "/symbol-art", strings.NewReader("text=Hello&banner=standard"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	application.ServeHTTP(response, request)
	close(release)
	group.Wait()
	if response.Code != 503 || response.Header().Get("Retry-After") != "1" {
		t.Fatalf("%d: %v", response.Code, response.Header())
	}
}
