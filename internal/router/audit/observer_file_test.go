package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"go.uber.org/zap"
)

// TestFileObserver_Success проверяет штатный сценарий создания логгера,
// асинхронной записи события и корректной структуры JSON на диске.
func TestFileObserver_Success(t *testing.T) {
	// 1. Создаем изолированную временную директорию
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	logger := zap.NewNop() // Гасим системные логи во время выполнения тестов

	// 2. Инициализируем наш FileObserver
	observer, err := NewFileObserver(logPath, logger)
	if err != nil {
		t.Fatalf("Не удалось инициализировать FileObserver: %v", err)
	}

	// 3. Формируем тестовое событие
	event := Event{
		TS:         time.Now().Unix(),
		UserID:     "user-456",
		Action:     ActionShorten,
		URL:        "https://example.com",
		Method:     "POST",
		Path:       "/",
		StatusCode: 201,
	}

	// Вызываем метод записи
	observer.OnRequest(context.Background(), event)

	// Закрываем файл, чтобы гарантированно сбросить буферы ОС на диск
	if err := observer.Close(); err != nil {
		t.Fatalf("Ошибка при закрытии FileObserver: %v", err)
	}

	// 4. Читаем записанный файл для проверки содержимого
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Не удалось прочитать созданный файл лога: %v", err)
	}

	// Десериализуем JSON обратно в структуру
	var readEvent Event
	if err := json.Unmarshal(data, &readEvent); err != nil {
		t.Fatalf("Файл содержит невалидный JSON: %v. Содержимое файла: %s", err, string(data))
	}

	// 5. Валидируем поля (с учетом тегов игнорирования json:"-")
	if readEvent.TS != event.TS {
		t.Errorf("Таймстемпы не совпадают. Ожидалось %d, получено %d", event.TS, readEvent.TS)
	}
	if readEvent.UserID != event.UserID {
		t.Errorf("UserID не совпадает. Ожидалось %s, получено %s", event.UserID, readEvent.UserID)
	}
	if readEvent.Action != event.Action {
		t.Errorf("Action не совпадает. Ожидалось %s, получено %s", event.Action, readEvent.Action)
	}
	if readEvent.URL != event.URL {
		t.Errorf("URL не совпадает. Ожидалось %s, получено %s", event.URL, readEvent.URL)
	}

	// Важная проверка тегов json:"-": технические HTTP-поля ДОЛЖНЫ быть пустыми
	if readEvent.Method != "" || readEvent.Path != "" || readEvent.StatusCode != 0 {
		t.Errorf("Поля с тегом json:\"-\" утекли в файл. Содержимое: %+v", readEvent)
	}
}

// TestFileObserver_Close_Twice проверяет идемпотентность деструктора
// и отсутствие паники при повторном или ошибочном вызове закрытия.
func TestFileObserver_Close_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit_close.log")

	observer, err := NewFileObserver(logPath, zap.NewNop())
	if err != nil {
		t.Fatalf("Ошибка инициализации: %v", err)
	}

	// Первое закрытие должно пройти успешно
	if err := observer.Close(); err != nil {
		t.Errorf("Первое закрытие завершилось ошибкой: %v", err)
	}

	// Повторный вызов Close() на уже закрытом файле должен вернуть ошибку операционной системы,
	// но рантайм Go не должен паниковать (вызывать panic)
	err = observer.Close()
	if err == nil {
		t.Error("Ожидалась ошибка при повторном закрытии дескриптора файла, но метод вернул nil")
	}
}

// BenchmarkFileObserver_SingleThread замеряет чистую скорость
// последовательного кодирования и записи событий в файл одним потоком.
func BenchmarkFileObserver_SingleThread(b *testing.B) {
	tmpDir := b.TempDir()
	logPath := filepath.Join(tmpDir, "bench_single.log")

	observer, err := NewFileObserver(logPath, zap.NewNop())
	if err != nil {
		b.Fatalf("Не удалось создать логгер: %v", err)
	}
	defer observer.Close()

	event := Event{
		TS:         time.Now().Unix(),
		UserID:     "benchmark-user-111",
		Action:     ActionShorten,
		URL:        "https://example.com",
		Method:     "POST",
		Path:       "/api/shorten",
		StatusCode: 201,
	}

	ctx := context.Background()

	for b.Loop() {
		observer.OnRequest(ctx, event)
	}
}

// BenchmarkFileObserver_Parallel замеряет падение производительности логгера
// при одновременной записи логов из множества конкурирующих горутин (воркеров).
func BenchmarkFileObserver_Parallel(b *testing.B) {
	tmpDir := b.TempDir()
	logPath := filepath.Join(tmpDir, "bench_parallel.log")

	observer, err := NewFileObserver(logPath, zap.NewNop())
	if err != nil {
		b.Fatalf("Не удалось создать логгер: %v", err)
	}
	defer observer.Close()

	event := Event{
		TS:         time.Now().Unix(),
		UserID:     "benchmark-user-222",
		Action:     ActionFollow,
		URL:        "https://example.com",
		Method:     "GET",
		Path:       "/alias",
		StatusCode: 307,
	}

	ctx := context.Background()

	b.ResetTimer()

	// Запуск параллельного тестирования воркеров
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			observer.OnRequest(ctx, event)
		}
	})
}
