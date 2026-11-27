// Package remote предоставляет сетевые компоненты для асинхронной
// отправки событий аудита на удаленные серверы с поддержкой отказоустойчивости.
package remote

import (
	"math/rand/v2"
	"sync"
	"time"

	"go.uber.org/zap"
)

// State определяет тип для управления состояниями предохранителя.
type State string

const (
	// StateClosed указывает, что цепь замкнута, запросы проходят в штатном режиме.
	StateClosed State = "CLOSED"
	// StateOpen указывает, что цепь разомкнута из-за ошибок, запросы блокируются.
	StateOpen State = "OPEN"
	// StateHalfOpen указывает, что цепь находится в тестовом режиме проверки связи.
	StateHalfOpen State = "HALF-OPEN"
)

const (
	failureThreshold = 5
	successThreshold = 3 // Нужно 3 успешных запроса подряд для закрытия цепи
	cooldownTimeout  = 30 * time.Second
	maxRetries       = 3
	initialBackoff   = 100 * time.Millisecond
	maxBackoff       = 2 * time.Second
)

// CircuitBreaker реализует паттерн «предохранитель» для предотвращения
// перегрузки удаленной системы и экономии ресурсов локальных воркеров при сбоях.
type CircuitBreaker struct {
	mu                   sync.RWMutex
	state                State
	failureCount         int
	successCount         int
	hasActiveTestRequest bool
	openedAt             time.Time
}

// NewCircuitBreaker создает и инициализирует новый экземпляр CircuitBreaker.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		state: StateClosed,
	}
}

// MaxRetries возвращает максимально допустимое количество попыток отправки одного запроса.
func (cb *CircuitBreaker) MaxRetries() int {
	return maxRetries
}

// AllowRequest проверяет, разрешено ли в текущий момент выполнять сетевой запрос.
// Если цепь в состоянии HALF-OPEN, метод строго контролирует конкурентность,
// пропуская только один тестовый поток с помощью атомарного флага.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen {
		if time.Since(cb.openedAt) > cooldownTimeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			cb.hasActiveTestRequest = true
			return true
		}
		return false
	}

	if cb.state == StateHalfOpen {
		if cb.hasActiveTestRequest {
			return false
		}
		cb.hasActiveTestRequest = true
		return true
	}

	return true
}

// ReleaseHalfOpenSlot безопасно сбрасывает флаг активности тестового запроса.
// Метод защищает стейт-машину от зависания при аварийной отмене контекста горутины.
func (cb *CircuitBreaker) ReleaseHalfOpenSlot() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == StateHalfOpen {
		cb.hasActiveTestRequest = false
	}
}

// IsHalfOpen возвращает true, если предохранитель находится в режиме тестирования связи.
func (cb *CircuitBreaker) IsHalfOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateHalfOpen
}

// RecordSuccess фиксирует успешное выполнение сетевого запроса.
func (cb *CircuitBreaker) RecordSuccess(logger *zap.Logger) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= successThreshold {
			logger.Info("Предохранитель успешно возвращен в состояние CLOSED. Связь восстановлена.")
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
		}
		return
	}

	cb.failureCount = 0
}

// RecordFailure фиксирует окончательный сбой цепочки попыток отправки лога.
func (cb *CircuitBreaker) RecordFailure(logger *zap.Logger) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Защита от запоздалых параллельных потоков: если цепь уже разомкнута, ничего не делаем
	if cb.state == StateOpen {
		return
	}

	if cb.state == StateHalfOpen {
		logger.Warn("Тестовый запрос в состоянии HALF-OPEN завершился ошибкой. Возврат в состояние OPEN.")
		cb.state = StateOpen
		cb.openedAt = time.Now()
		cb.successCount = 0
		return
	}

	cb.failureCount++
	if cb.failureCount >= failureThreshold {
		logger.Error("Достигнут лимит сетевых ошибок. Предохранитель переходит в состояние OPEN.",
			zap.Int("failure_count", cb.failureCount),
		)
		cb.state = StateOpen
		cb.openedAt = time.Now()
	}
}

// CalculateBackoff вычисляет время задержки с добавлением случайного джиттера.
func (cb *CircuitBreaker) CalculateBackoff(attempt int) time.Duration {
	backoff := min(initialBackoff*time.Duration(1<<attempt), maxBackoff)
	jitter := time.Duration(rand.N(int64(backoff / 2)))
	return backoff + jitter
}
