package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"archive-review/internal/domain"
	"archive-review/internal/workflow"
)

func writeMeta(r *http.Request, requireRevision bool) (workflow.WriteMeta, error) {
	meta := workflow.WriteMeta{ActorID: strings.TrimSpace(r.Header.Get("X-Actor-ID")),
		RequestID: strings.TrimSpace(r.Header.Get("X-Request-ID"))}
	if requireRevision {
		revision, err := parseRevision(r.Header.Get("If-Match"))
		if err != nil {
			return meta, err
		}
		meta.ExpectedRevision = revision
	}
	if err := meta.Validate(requireRevision); err != nil {
		return meta, err
	}
	return meta, nil
}

func parseRevision(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "W/") {
		value = strings.TrimPrefix(value, "W/")
	}
	value = strings.Trim(value, `"`)
	if value == "" {
		return 0, domain.Invalid("revision", "写操作必须提供 If-Match revision")
	}
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 1 {
		return 0, domain.Invalid("revision", "If-Match 必须是正整数 revision")
	}
	return revision, nil
}

func requirePathValue(r *http.Request, name string) (string, error) {
	value := strings.TrimSpace(r.PathValue(name))
	if value == "" {
		return "", domain.Invalid(name, fmt.Sprintf("路径参数 %s 不能为空", name))
	}
	return value, nil
}
