package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pjunak/addon-dnd-engine/internal/engine"
	"github.com/pjunak/addon-dnd-engine/internal/provider"
	"github.com/pjunak/ttrpg-codex/sdk/go/workerrpc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := workerrpc.RunNativeWorker(ctx, workerrpc.NativeWorkerConfig{
		Reader:  os.Stdin,
		Writer:  os.Stdout,
		Methods: advertisedMethods(),
		HandlerFactory: workerrpc.NativeWorkerHandlerFactoryFunc(func(
			worker workerrpc.NativeWorkerContext,
		) (workerrpc.RequestHandler, error) {
			data, err := provider.New(worker.Peer)
			if err != nil {
				return nil, err
			}
			return engine.New(data)
		}),
	})
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func advertisedMethods() map[string]string {
	methods := []string{
		"context",
		"get-record",
		"query-records",
		"derive",
		"hydrate",
		"builder-plan",
		"apply-builder-choice",
		"reconcile-builder-decisions",
	}
	result := make(map[string]string, len(methods))
	for _, method := range methods {
		result["service/dnd5e.rules-engine/"+method] = engine.ContractVersion
	}
	return result
}
