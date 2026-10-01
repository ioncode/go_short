package pkg

import (
	"errors"

	"github.com/google/uuid"
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

// UUIDCodec реализует интерфейс securecookie.Serializer.
// Он предназначен для сверхбыстрой сериализации бинарных UUID
// без аллокаций памяти и использования рефлексии.
type UUIDCodec struct{}

// Serialize преобразует входящий uuid.UUID в сырой срез байт.
func (UUIDCodec) Serialize(src any) ([]byte, error) {
	id, ok := src.(uuid.UUID)
	if !ok {
		return nil, errors.New("securecookie: кодек поддерживает только тип uuid.UUID")
	}

	// uuid.UUID — это [16]byte. Слайсим массив, чтобы вернуть []byte.
	// Операция среза массива не создает аллокаций в куче!
	return id[:], nil
}

// Deserialize считывает сырые 16 байт из куки и записывает их напрямую в uuid.UUID.
func (UUIDCodec) Deserialize(src []byte, dst any) error {
	ptr, ok := dst.(*uuid.UUID)
	if !ok {
		return errors.New("securecookie: целевой объект должен быть указателем на uuid.UUID")
	}

	// Валидируем длину байтового среза UUID
	if len(src) != 16 {
		return errors.New("securecookie: неверная длина бинарного UUID")
	}

	// Копируем 16 байт напрямую в память по указателю
	copy(ptr[:], src)
	return nil
}
