package pool_test

import (
	"sync"
	"testing"

	"github.com/ioncode/go_short/pkg/pool"
)

type testObj struct {
	Items []string
	ID    int
}

func (t *testObj) Reset() {
	t.ID = 0
	t.Items = t.Items[:0]
}

func TestPool_GetPutAndReset(t *testing.T) {
	p := pool.New(func() *testObj {
		return &testObj{
			Items: make([]string, 0, 10),
		}
	})

	// Получаем новый объект
	obj := p.Get()
	if obj == nil {
		t.Fatal("ожидался не-nil объект из пула")
	}

	// Заполняем данными
	obj.ID = 42
	obj.Items = append(obj.Items, "one", "two")

	// Возвращаем в пул (должен вызваться Reset)
	p.Put(obj)

	// Получаем объект повторно и проверяем, что состояние сброшено
	reused := p.Get()
	if reused.ID != 0 {
		t.Errorf("ожидался ID = 0 после Reset(), получено %d", reused.ID)
	}
	if len(reused.Items) != 0 {
		t.Errorf("ожидался пустой слайс после Reset(), получено %v", reused.Items)
	}
}

func TestPool_Concurrent(t *testing.T) {
	p := pool.New(func() *testObj {
		return &testObj{Items: make([]string, 0, 8)}
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			item := p.Get()
			if item.ID != 0 || len(item.Items) != 0 {
				t.Errorf("получен несброшенный объект: %+v", item)
			}
			item.ID = id
			item.Items = append(item.Items, "data")
			p.Put(item)
		}(i)
	}
	wg.Wait()
}
