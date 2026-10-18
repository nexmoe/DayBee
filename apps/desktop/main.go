package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/egoist/mygo"
	"github.com/nexmoe/DayBee/apps/desktop/internal/asr"
	"github.com/nexmoe/DayBee/apps/desktop/internal/codec"
	"github.com/nexmoe/DayBee/apps/desktop/internal/desktop"
)

func main() {
	if os.Getenv("DAYBEE_VERIFY_AUDIO") == "1" {
		if err := codec.Diagnostic(os.Args[1:], os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if os.Getenv("DAYBEE_VERIFY_NATIVE") == "1" {
		if err := asr.CheckVersion(); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("{\"sherpa\":%q,\"onnxruntime\":%q}\n", asr.Version, asr.OnnxRuntimeVersion)
		return
	}
	mygo.App.SetName(desktop.AppName)
	if mygo.App.Version() == "" {
		mygo.App.SetVersion("1.0.0")
	}
	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := desktop.RunMCP(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}
	if err := desktop.Run(desktop.New()); err != nil {
		log.Fatal(err)
	}
}
