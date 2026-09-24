package audit

import (
	"context"
	"sync"
	"time"

	"github.com/ioncode/go_short/internal/logger"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Auditor является центральным диспетчером (Subject), который управляет
// списком наблюдателей и распределяет события через буферизованную очередь.
type Auditor struct {
	observers []Observer
	mu        sync.RWMutex

	queue  chan Event
	g      *errgroup.Group
	ctx    context.Context
	logger *zap.Logger
}

// New инициализирует и запускает новый экземпляр Auditor.
// Принимает контекст приложения, размер буфера очереди и количество фоновых воркеров.
func New(ctx context.Context, logger *zap.Logger, bufferSize int, workerCount int) *Auditor {
	logger = logger.Named("audit_auditor")
	logger.Info("Инициализация аудитора",
		zap.Int("buffer_size", bufferSize),
		zap.Int("worker_count", workerCount),
	)
	g, groupCtx := errgroup.WithContext(ctx)

	a := &Auditor{
		observers: make([]Observer, 0),
		queue:     make(chan Event, bufferSize),
		g:         g,
		ctx:       groupCtx,
		logger:    logger,
	}

	for range workerCount {
		a.g.Go(func() error {
			a.worker()
			return nil
		})
	}

	return a
}

// Register добавляет нового наблюдателя в список рассылки.
// Метод полностью потокобезопасен и может вызываться во время работы сервера.
func (a *Auditor) Register(o Observer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.observers = append(a.observers, o)
	a.logger.Info("Добавлен новый приемник событий аудита", zap.String("name", o.Name()))
}

// Notify отправляет событие в очередь для последующей обработки воркерами.
// Если контекст аудитора отменен или очередь переполнена, событие будет пропущено
// во избежание блокировки основного HTTP-потока.
func (a *Auditor) Notify(event Event) {
	select {
	case <-a.ctx.Done():
		return
	case a.queue <- event:
	default:
		logger.Log.Warn("Очередь аудита переполнена, событие пропущено",
			zap.String("method", event.Method),
			zap.String("path", event.Path),
			zap.String("action", string(event.Action)),
		)
	}
}

// worker — фоновая горутина, извлекающая события из очереди.
func (a *Auditor) worker() {
	for {
		select {
		case event, ok := <-a.queue:
			if !ok {
				return
			}
			a.notifyObservers(event)

		case <-a.ctx.Done():
			// При отмене контекста вычищаем остатки из буфера очереди
			for {
				select {
				case event, ok := <-a.queue:
					if !ok {
						return
					}
					a.notifyObservers(event)
				default:
					return
				}
			}
		}
	}
}

// notifyObservers последовательно обходит всех наблюдателей и передает им событие.
func (a *Auditor) notifyObservers(event Event) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, observer := range a.observers {
		observer.OnRequest(a.ctx, event)
	}
}

// Shutdown производит безопасную остановку пула воркеров.
// Закрывает очередь на вход и ожидает, пока воркеры обработают оставшиеся события
// в пределах переданного таймаута duration.
func (a *Auditor) Shutdown(timeout time.Duration) error {
	a.logger.Info("Остановка аудитора", zap.Duration("timeout", timeout))
	close(a.queue)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- a.g.Wait()
	}()

	select {
	case err := <-done:
		a.logger.Info("Аудитор успешно остановлен, все оповещения корректно обработаны. Освобождаем ресурсы приемников...")

		a.mu.Lock()
		defer a.mu.Unlock()
		for _, observer := range a.observers {
			// Проверяем с помощью утверждения типов (Type Assertion),
			// умеет ли конкретный наблюдатель закрывать за собой ресурсы
			if closer, ok := observer.(interface{ Close() error }); ok {
				if cErr := closer.Close(); cErr != nil {
					a.logger.Error("Не удалось освободить ресурсы приемника", zap.String("name", observer.Name()), zap.Error(cErr))
				} else {
					a.logger.Info("Ресурсы приемника корректно освобождены", zap.String("name", observer.Name()))
				}
			}
		}

		return err
	case <-ctx.Done():
		a.logger.Error("Процесс остановки прерван по окончании времени ожидания, возможна потеря данных")
		return ctx.Err()
	}
}
