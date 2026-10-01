package remote

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestCircuitBreaker_ClosedToOpen проверяет, что предохранитель корректно
// переходит из состояния CLOSED в состояние OPEN при достижении лимита ошибок,
// а также защищает логи от спама при избыточных ошибках.
func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	logger := zap.NewNop()
	cb := NewCircuitBreaker()

	// 1. Изначально цепь должна быть закрыта
	if !cb.AllowRequest() {
		t.Fatal("Ожидалось, что в исходном состоянии CLOSED запросы разрешены")
	}

	// 2. Симулируем 4 ошибки (на 1 меньше порога размыкания)
	for i := 0; i < failureThreshold-1; i++ {
		cb.RecordFailure(logger)
		if !cb.AllowRequest() {
			t.Errorf("Запрос заблокирован на ошибке %d, хотя лимит %d еще не достигнут", i+1, failureThreshold)
		}
	}

	// 3. Фиксируем 5-ю ошибку — цепь должна разомкнуться (перейти в OPEN)
	cb.RecordFailure(logger)
	if cb.AllowRequest() {
		t.Error("Ожидалось, что предохранитель заблокирует запросы после достижения лимита ошибок")
	}

	// 4. Тест защиты от спама: отправляем еще одну ошибку в состоянии OPEN.
	// Время открытияopenedAt не должно перезаписываться текущим моментом.
	oldOpenedAt := cb.openedAt
	time.Sleep(1 * time.Millisecond) // Минимальная пауза для разницы во времени

	cb.RecordFailure(logger)
	if cb.openedAt != oldOpenedAt {
		t.Error("Уязвимость: запоздалые параллельные ошибки перезаписывают время openedAt в состоянии OPEN")
	}
}

// TestCircuitBreaker_CooldownAndHalfOpen проверяет логику остывания цепи
// и автоматический переход в состояние HALF-OPEN по истечении таймаута.
func TestCircuitBreaker_CooldownAndHalfOpen(t *testing.T) {

	cb := NewCircuitBreaker()

	// 1. Искусственно переводим цепь в OPEN
	cb.state = StateOpen
	cb.openedAt = time.Now()

	// 2. Сразу после падения запросы должны быть жестко заблокированы
	if cb.AllowRequest() {
		t.Error("Ожидалось, что в состоянии OPEN запросы будут заблокированы до истечения таймаута")
	}

	// 3. Эмулируем прохождение времени остывания.
	// Сдвигаем время открытия назад в прошлое на (cooldownTimeout + 1 секунда)
	cb.openedAt = time.Now().Add(-cooldownTimeout).Add(-time.Second)

	// 4. Теперь AllowRequest должен перевести цепь в HALF-OPEN и вернуть true
	if !cb.AllowRequest() {
		t.Error("Ожидалось, что после остывания предохранитель пропустит тестовый запрос")
	}

	if cb.state != StateHalfOpen {
		t.Errorf("Ожидался переход в состояние HALF-OPEN, текущее состояние: %s", cb.state)
	}
}

// TestCircuitBreaker_HalfOpen_Concurrency Шлюз проверяет, что в режиме HALF-OPEN
// строго соблюдается атомарность: только ОДНА горутина получает доступ,
// а все параллельные потоки блокируются до получения результатов теста.
func TestCircuitBreaker_HalfOpen_Concurrency(t *testing.T) {
	cb := NewCircuitBreaker()

	// Переводим цепь в режим проверки связи
	cb.state = StateHalfOpen
	cb.hasActiveTestRequest = false

	// 1. Первая горутина занимает слот
	if !cb.AllowRequest() {
		t.Fatal("Ожидалось, что первый тестовый запрос в HALF-OPEN будет разрешен")
	}
	if !cb.hasActiveTestRequest {
		t.Error("Флаг hasActiveTestRequest должен быть равен true после захвата слота")
	}

	// 2. Вторая горутина пытается прорваться параллельно — ей должно отказать
	if cb.AllowRequest() {
		t.Error("Защита сломана: предохранитель пропустил второй параллельный запрос в режиме HALF-OPEN")
	}

	// 3. Высвобождаем слот (например, по деферу в OnRequest при отмене контекста)
	cb.ReleaseHalfOpenSlot()
	if cb.hasActiveTestRequest {
		t.Error("Флаг активности должен сброситься после вызова ReleaseHalfOpenSlot")
	}

	// 4. После освобождения слота следующая горутина снова может попробовать пройти тест
	if !cb.AllowRequest() {
		t.Error("Ожидалось, что после освобождения слота следующий тест будет разрешен")
	}
}

// TestCircuitBreaker_HalfOpenToClosed проверяет сценарий успешного выздоровления цепи.
// Предохранитель должен зафиксировать successThreshold (3) успехов подряд, чтобы закрыться.
func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	logger := zap.NewNop()
	cb := NewCircuitBreaker()

	cb.state = StateHalfOpen

	// Посылаем (successThreshold - 1) успешных ответов
	for i := 0; i < successThreshold-1; i++ {
		cb.RecordSuccess(logger)
		if cb.state != StateHalfOpen {
			t.Errorf("Цепь преждевременно вышла из HALF-OPEN на шаге %d", i+1)
		}
	}

	// Фиксируем финальный успешный шаг — цепь обязана закрыться
	cb.RecordSuccess(logger)
	if cb.state != StateClosed {
		t.Errorf("Ожидался возврат в состояние CLOSED, текущее состояние: %s", cb.state)
	}

	if cb.failureCount != 0 || cb.successCount != 0 {
		t.Error("Счетчики ошибок и успехов не обнулились при переходе в CLOSED")
	}
}

// TestCircuitBreaker_HalfOpenToOpen проверяет, что при первой же ошибке
// в режиме HALF-OPEN цепь моментально отбрасывается назад в состояние OPEN без ретраев.
func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	logger := zap.NewNop()
	cb := NewCircuitBreaker()

	cb.state = StateHalfOpen
	cb.successCount = 2 // Накопили определенный прогресс выздоровления

	// Фиксируем сетевую ошибку на тесте
	cb.RecordFailure(logger)

	if cb.state != StateOpen {
		t.Errorf("Ожидался мгновенный возврат в состояние OPEN, текущее состояние: %s", cb.state)
	}

	if cb.successCount != 0 {
		t.Error("Счетчик накопленных успехов должен был сброситься в 0")
	}
}
