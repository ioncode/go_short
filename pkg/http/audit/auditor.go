package audit

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// obsLoop объединяет наблюдателя и его изолированную очередь.
type obsLoop struct {
	observer Observer
	queue    chan Event
}

// Auditor является центральным диспетчером (Subject), который распределяет
// события по индивидуальным буферизованным очередям каждого наблюдателя.
type Auditor struct {
	mu         sync.RWMutex
	loops      []*obsLoop
	bufferSize int
	g          *errgroup.Group
	ctx        context.Context
	logger     *zap.Logger
}

// New инициализирует новый экземпляр Auditor.
// Принимает контекст приложения и размер буфера для очереди каждого наблюдателя.
// Аргумент workerCount больше не используется для общего пула, но сохранен для совместимости сигнатуры.
func New(ctx context.Context, logger *zap.Logger, bufferSize int, workerCount int) *Auditor {
	logger = logger.Named("audit_auditor")
	logger.Info("Инициализация аудитора с изолированными очередями",
		zap.Int("buffer_size", bufferSize),
	)

	g, groupCtx := errgroup.WithContext(ctx)

	return &Auditor{
		loops:      make([]*obsLoop, 0),
		bufferSize: bufferSize,
		g:          g,
		ctx:        groupCtx,
		logger:     logger,
	}
}

// Register добавляет нового наблюдателя, создает для него персональную
// буферизованную очередь и запускает выделенный фоновый воркер.
func (a *Auditor) Register(o Observer) {
	a.mu.Lock()
	defer a.mu.Unlock()

	loop := &obsLoop{
		observer: o,
		queue:    make(chan Event, a.bufferSize),
	}
	a.loops = append(a.loops, loop)

	// Запускаем персональный воркер для этого наблюдателя
	a.g.Go(func() error {
		a.worker(loop)
		return nil
	})

	a.logger.Info("Добавлен новый изолированный приемник событий аудита", zap.String("name", o.Name()))
}

// Notify асинхронно рассылает событие по очередям всех зарегистрированных наблюдателей.
// Если очередь конкретного приёмника переполнена, событие сбрасывается ТОЛЬКО для него,
// не блокируя отправку остальным участникам и основной HTTP-поток.
func (a *Auditor) Notify(event Event) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	select {
	case <-a.ctx.Done():
		return
	default:
	}

	for _, loop := range a.loops {
		select {
		case loop.queue <- event:
		default:
			a.logger.Warn("Очередь приемника переполнена, событие для него пропущено",
				zap.String("receiver", loop.observer.Name()),
				zap.String("method", event.Method),
				zap.String("path", event.Path),
			)
		}
	}
}

// worker обрабатывает события из персональной очереди конкретного наблюдателя.
func (a *Auditor) worker(loop *obsLoop) {
	for {
		select {
		case event, ok := <-loop.queue:
			if !ok {
				return
			}
			loop.observer.OnRequest(a.ctx, event)

		case <-a.ctx.Done():
			// При отмене контекста вычищаем остатки из буфера этой конкретной очереди
			for {
				select {
				case event, ok := <-loop.queue:
					if !ok {
						return
					}
					loop.observer.OnRequest(a.ctx, event)
				default:
					return
				}
			}
		}
	}
}

// Shutdown производит безопасную и аккуратную остановку всех очередей.
func (a *Auditor) Shutdown(timeout time.Duration) error {
	a.logger.Info("Остановка аудитора и закрытие изолированных очередей", zap.Duration("timeout", timeout))

	a.mu.Lock()
	// Закрываем каналы всех воркеров, чтобы они вышли из циклов после дочитывания буферов
	for _, loop := range a.loops {
		close(loop.queue)
	}
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- a.g.Wait()
	}()

	select {
	case err := <-done:
		a.logger.Info("Все воркеры аудита завершили работу. Освобождаем системные ресурсы приемников...")

		a.mu.Lock()
		defer a.mu.Unlock()
		for _, loop := range a.loops {
			if closer, ok := loop.observer.(interface{ Close() error }); ok {
				if cErr := closer.Close(); cErr != nil {
					a.logger.Error("Не удалось освободить ресурсы приемника", zap.String("name", loop.observer.Name()), zap.Error(cErr))
				} else {
					a.logger.Info("Ресурсы приемника корректно освобождены", zap.String("name", loop.observer.Name()))
				}
			}
		}
		return err

	case <-ctx.Done():
		a.logger.Error("Процесс остановки прерван по таймауту, возможна потеря хвоста логов в буферах")
		return ctx.Err()
	}
}
