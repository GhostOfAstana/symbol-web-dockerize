# Карта требований и проверок

Эта таблица связывает пункты приложенного ТЗ с файлами и воспроизводимыми проверками. Она не подменяет результат конкретного запуска тестов.

| Требование | Где реализовано | Как проверить |
| --- | --- | --- |
| Go-сервер, формы, шаблоны, static | `cmd/server`, `internal/handlers`, `templates`, `static` | `go test ./internal/handlers`; открыть `/` |
| Три официальных шрифта | корневые `.txt`, `internal/ascii` | `go test ./internal/ascii`; три POST в smoke |
| Структура сдачи | отдельные `cmd/server`, `cmd/health-monitor`, пакеты | дерево README; `go list ./...` |
| Только разрешённые пакеты | `go.mod`, импорты | `go list -m all` — один корневой модуль |
| Dockerfile и многоэтапный образ | `Dockerfile` | `docker image build -f Dockerfile -t symbol-web-docker .` |
| Реальный контейнер | Makefile `run`, smoke | `docker ps -a`; HTTP 200 |
| Файлы внутри контейнера, Bash | финальный этап Dockerfile | `docker exec ... /bin/bash`; `ls -l /app` |
| Labels образа и контейнера | Dockerfile, Makefile | `docker inspect`; smoke |
| Очистка неиспользуемых объектов | Makefile `clean`, trap smoke | `make stop clean`; фильтр проектной метки |
| Реальные uptime/restarts | `internal/monitor/docker.go` | inspect; CLI `--json` |
| Реальные память/CPU | `internal/monitor/docker.go` | stats; CLI `--json` |
| Правильные четыре оценки | `internal/monitor/score.go` | `TestScoreWorkedExamples`, `TestScoreBoundaries` |
| Звёзды только из суммы | `Rating` | тесты границ 59/60/69/70/79/80/89/90 |
| Неизвестный контейнер, daemon off | `Collect`, CLI exit 1, HTTP 503 | `TestCollectFailureCases`; реальный CLI против неверного имени |
| Стресс: испорченные данные | JSON/процент-парсер, лимиты | `FuzzDockerJSON`, `TestScoreRejectsInvalidMetrics` |
| Стресс: конкуренция | один HTTP-сбор, общий генератор | `TestHealthConcurrencyBound`, `TestConcurrentGeneration`, `-race` |
| Бонус: четыре правила линтера | `internal/monitor/lint.go`, CLI `--lint` | `make lint`, `TestLint`, `TestCLILint` |
| Бонус: польза правила и read-only | `Finding.Benefit`, только `os.Open` | вывод линтера; сравнение файла в тесте |
| Бонус: healthy + degraded tests | `TestScoreWorkedExamples` | 100 и 30 баллов |
| Дополнительно: nonroot и read-only | Dockerfile, Makefile | smoke запускает с ограничениями |
| Дополнительно: корректный SIGTERM | `cmd/server/main.go` | `docker stop --time 5` в smoke |
| Базовый помощник и вариации | `internal/textai`, `text-assist.js` | `go test ./internal/textai`; UI |
| Русские объяснения и ответы | README, `docs` | изучить и воспроизвести примеры |

Для полного контейнерного аудита запустите `make smoke` на Docker-хосте. Workflow делает это на GitHub Actions без установки Docker на пользовательский компьютер.
