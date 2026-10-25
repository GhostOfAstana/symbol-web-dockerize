// Package handlers связывает HTTP-запросы с генератором и монитором.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"strconv"

	"github.com/GhostOfAstana/symbol-web-dockerize/internal/ascii"
	"github.com/GhostOfAstana/symbol-web-dockerize/internal/monitor"
)

const maxForm = 32 << 10

type Collector interface {
	Collect(context.Context, string) (monitor.Report, error)
}

type app struct {
	generator       *ascii.Generator
	templates       *template.Template
	collector       Collector
	container       string
	monitorSlots    chan struct{}
	generationSlots chan struct{}
}

type page struct {
	Text   string
	Banner string
	Art    string
	Error  string
}

func New(files fs.FS, collector Collector, container string) (*http.ServeMux, error) {
	generator, err := ascii.New(files)
	if err != nil {
		return nil, err
	}
	templates, err := template.ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	application := &app{generator: generator, templates: templates, collector: collector,
		container: container, monitorSlots: make(chan struct{}, 1), generationSlots: make(chan struct{}, 8)}
	mux := http.NewServeMux()
	mux.HandleFunc("/", application.index)
	mux.HandleFunc("/symbol-art", application.generate)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health-monitor", application.health)
	static, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}
	server := http.StripPrefix("/static/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		if !method(w, r, http.MethodGet) {
			return
		}
		switch r.URL.Path {
		case "/static/css/style.css", "/static/js/script.js", "/static/js/text-assist.js":
			server.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	return mux, nil
}

// SecureHeaders задаёт защитные заголовки и единый предел тела до разбора формы.
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (application *app) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		application.render(w, http.StatusNotFound, page{Banner: "standard", Error: "Страница не найдена."})
		return
	}
	if !method(w, r, http.MethodGet) {
		return
	}
	application.render(w, http.StatusOK, page{Banner: "standard"})
}

func (application *app) generate(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	// Предел одновременно создаваемых баннеров ограничивает пик расхода памяти.
	select {
	case application.generationSlots <- struct{}{}:
		defer func() { <-application.generationSlots }()
	default:
		w.Header().Set("Retry-After", "1")
		application.render(w, http.StatusServiceUnavailable, page{Banner: "standard", Error: "Сервер обрабатывает несколько надписей. Повторите попытку через секунду."})
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		application.render(w, http.StatusBadRequest, page{Banner: "standard", Error: "Отправьте обычную HTML-форму с текстом и шрифтом."})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		status := http.StatusBadRequest
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			status = http.StatusRequestEntityTooLarge
		}
		application.render(w, status, page{Banner: "standard", Error: "Форма повреждена или превышает предел 32 КиБ."})
		return
	}
	data := page{Text: r.PostForm.Get("text"), Banner: r.PostForm.Get("banner")}
	if data.Text == "" {
		data.Error = "Введите хотя бы один печатный ASCII-символ."
		application.render(w, http.StatusBadRequest, data)
		return
	}
	data.Art, err = application.generator.Generate(data.Text, data.Banner)
	if err != nil {
		data.Error = err.Error()
		application.render(w, http.StatusBadRequest, data)
		return
	}
	application.render(w, http.StatusOK, data)
}

func (application *app) health(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if application.collector == nil || application.container == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "HTTP-монитор отключён. Запустите health-monitor на Docker-хосте или задайте MONITOR_CONTAINER."})
		return
	}
	select {
	case application.monitorSlots <- struct{}{}:
		defer func() { <-application.monitorSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Монитор уже читает показатели. Повторите запрос через две секунды."})
		return
	}
	report, err := application.collector.Collect(r.Context(), application.container)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Не удалось прочитать показатели. Проверьте Docker, имя и состояние контейнера."})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (application *app) render(w http.ResponseWriter, status int, data page) {
	var output bytes.Buffer
	if err := application.templates.ExecuteTemplate(&output, "index.html", data); err != nil {
		http.Error(w, "Не удалось подготовить страницу.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(output.Len()))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(output.Bytes())
}

func method(w http.ResponseWriter, r *http.Request, expected string) bool {
	if r.Method == expected {
		return true
	}
	w.Header().Set("Allow", expected)
	http.Error(w, "Метод запроса не поддерживается.", http.StatusMethodNotAllowed)
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "Не удалось подготовить ответ.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
