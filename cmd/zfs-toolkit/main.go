package main

import (
	"os"

	"github.com/MajorPain007/unraid-zfs-toolkit/internal/zt"
)

func main() {
	os.Exit(zt.Main(os.Args[1:]))
}
