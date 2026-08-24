package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() { os.Exit(run(os.Args[1:], os.Getenv)) }

func run(args []string, getenv func(string) string) int {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "配置错误:", err)
		return 2
	}
	if cfg.SelfTest {
		if err := runSelfTest(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", err)
			return 1
		}
		fmt.Println("自检通过：案件已发布，幂等重放与审计链完整")
		return 0
	}
	if err := serve(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "服务退出:", err)
		return 1
	}
	return 0
}

func serve(cfg config) error {
	server, err := buildServer(cfg.DataDir)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.Addr, err)
	}
	defer listener.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	fmt.Printf("公共档案去标识审核服务监听 %s\n", cfg.Addr)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("优雅关闭: %w", err)
		}
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
