package main

import (
	"log"
	"os"

	"github.com/nexmoe/eve/apps/desktop/internal/codec"
)

func main() {
	if err := codec.Diagnostic(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}
