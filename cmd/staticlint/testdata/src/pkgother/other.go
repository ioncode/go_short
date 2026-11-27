package pkgother

import "os"

func main() {
	os.Exit(1) // игнорируется: пакет не называется main
}
