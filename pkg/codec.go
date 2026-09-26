package pkg

import (
	"errors"
)

// StringCodec реализует интерфейс securecookie.Serializer.
// Он предназначен для сверхбыстрой сериализации и десериализации чистых строк (UUID)
// без использования тяжелой рефлексии пакетов JSON или Gob.
type StringCodec struct{}

// Serialize преобразует входящую строку userID в сырой срез байт.
func (StringCodec) Serialize(src any) ([]byte, error) {
	str, ok := src.(string)
	if !ok {
		return nil, errors.New("securecookie: кодек поддерживает только плоские строки")
	}
	// Преобразуем строку в байты без использования внешних маршалеров
	return []byte(str), nil
}

// Deserialize считывает сырые байты из куки и записывает их напрямую в строку.
func (StringCodec) Deserialize(src []byte, dst any) error {
	ptr, ok := dst.(*string)
	if !ok {
		return errors.New("securecookie: целевой объект должен быть указателем на строку")
	}
	// Записываем чистую строку напрямую по переданному указателю
	*ptr = string(src)
	return nil
}
