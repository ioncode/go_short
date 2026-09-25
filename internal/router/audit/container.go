package audit

import (
	"maps"
	"sync"
)

// Глобальный пул для повторного использования контейнеров аудита.
// Позволяет избежать аллокаций памяти на каждый HTTP-запрос.
var containerPool = sync.Pool{
	New: func() any {
		return &auditContainer{
			data: make(map[string]any, 2), // Инициализируем с емкостью под используемые постоянно поля url и action
		}
	},
}

type auditContainer struct {
	mu   sync.RWMutex
	data map[string]any
}

// getAuditContainer извлекает чистый контейнер из пула
func getAuditContainer() *auditContainer {
	return containerPool.Get().(*auditContainer)
}

// release очищает карту и возвращает контейнер в пул.
// Очистка через range-delete в Go сохраняет выделенную емкость (capacity) карты,
// что предотвращает повторные аллокации памяти при следующем использовании.
func (c *auditContainer) release() {
	c.mu.Lock()
	for k := range c.data {
		delete(c.data, k)
	}
	c.mu.Unlock()

	containerPool.Put(c)
}

// snapshot создает копию данных под защитой RLock
func (c *auditContainer) snapshot() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.data) == 0 {
		return nil
	}

	copyData := make(map[string]any, len(c.data))
	maps.Copy(copyData, c.data)
	return copyData
}
