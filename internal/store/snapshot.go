package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func prepareSnapshot(path string, value any) (string, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("编码案件快照: %w", err)
	}
	b = append(b, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建临时快照: %w", err)
	}
	name := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := f.Chmod(0o640); err != nil {
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		return "", fmt.Errorf("写入临时快照: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("同步临时快照: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("关闭临时快照: %w", err)
	}
	ok = true
	return name, nil
}

func replaceSnapshot(tempPath, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("原子替换案件快照: %w", err)
	}
	dir, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return fmt.Errorf("打开快照目录: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("同步快照目录: %w", err)
	}
	return nil
}

func jsonUnmarshalStrict(b []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("快照包含多余 JSON 值")
	}
	return nil
}
