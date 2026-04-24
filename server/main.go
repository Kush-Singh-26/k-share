package main

import (
	"os"

	"github.com/Kush-Singh-26/k-share/server/internal/bootstrap"
)

func main() {
	bootstrap.Run(os.Args)
}
