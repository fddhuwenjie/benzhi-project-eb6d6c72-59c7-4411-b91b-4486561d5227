package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	Addr     string
	DataDir  string
	SelfTest bool
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	fs := flag.NewFlagSet("archive-review-server", flag.ContinueOnError)
	fs.SetOutput(discardWriter{})
	addr := fs.String("addr", defaultAddress, "监听地址")
	dataDir := fs.String("data-dir", "./data", "持久化数据目录")
	selftest := fs.Bool("selftest", false, "运行端到端自检后退出")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("不支持位置参数")
	}
	addrExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrExplicit = true
		}
	})
	resolved := strings.TrimSpace(*addr)
	if !addrExplicit {
		portValue := strings.TrimSpace(getenv("PORT"))
		if portValue != "" {
			port, err := strconv.Atoi(portValue)
			if err != nil || port < 1 || port > 65535 {
				return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			resolved = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}
	if err := validateAddress(resolved); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*dataDir) == "" {
		return config{}, fmt.Errorf("data-dir 不能为空")
	}
	return config{Addr: resolved, DataDir: *dataDir, SelfTest: *selftest}, nil
}

func validateAddress(addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("addr 必须采用 host:port 格式: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("addr 必须使用明确的回环 IP 地址")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("addr 端口必须在 1 到 65535 之间")
	}
	return nil
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
