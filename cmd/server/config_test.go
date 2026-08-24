package main

import "testing"

func TestParseConfigAddressSources(t *testing.T) {
	cfg, err := parseConfig(nil, func(key string) string {
		if key == "PORT" {
			return "19123"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19123" {
		t.Fatalf("addr=%s", cfg.Addr)
	}
	cfg, err = parseConfig([]string{"-addr=127.0.0.1:19234"}, func(string) string { return "invalid" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19234" {
		t.Fatalf("explicit addr=%s", cfg.Addr)
	}
}

func TestParseConfigRejectsUnsafeAddresses(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:19081", "localhost:19081", "127.0.0.1:8080", "127.0.0.1:0"} {
		_, err := parseConfig([]string{"-addr=" + addr}, func(string) string { return "" })
		if addr == "127.0.0.1:8080" {
			if err != nil {
				t.Fatalf("显式高风险常见端口仍是有效显式配置: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("应拒绝 %s", addr)
		}
	}
}
