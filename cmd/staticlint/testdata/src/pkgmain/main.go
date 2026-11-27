package main

import (
	"os"
	myos "os"
)

type app struct{}

func (a *app) main() {
	os.Exit(1) // игнорируется: это метод структуры, а не функция main
}

func helper() {
	os.Exit(1) // игнорируется: вызов не внутри функции main
}

type fakeOS struct{}

func (fakeOS) Exit(int) {}

func main() {
	var os fakeOS
	os.Exit(0) // игнорируется: вызывается метод локальной структуры, а не пакет os

	myos.Exit(1) // want "direct call to os.Exit in main function is prohibited"
}
