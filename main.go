package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"imagetoolbox/internal/cmd"
	"imagetoolbox/internal/nativebin"
)

//go:embed bins/**
var binaries embed.FS

var version = "dev"

func main() {
	nativebin.Init(binaries)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd.Execute(ctx, version); err != nil {
		// 已按 itb.error.v1 输出到 stdout 的错误不再向 stderr 重复打印
		if !errors.Is(err, cmd.ErrReported) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
