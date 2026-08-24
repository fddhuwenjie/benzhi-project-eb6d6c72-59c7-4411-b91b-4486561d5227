package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

const maxRequestBody = 1 << 20

type dataEnvelope struct {
	Data any `json:"data"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Details any    `json:"details,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if header := r.Header.Get("Content-Type"); header != "" {
		media, _, err := mime.ParseMediaType(header)
		if err != nil || media != "application/json" {
			return fmt.Errorf("Content-Type 必须为 application/json")
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fmt.Errorf("请求体不得超过 %d 字节", maxRequestBody)
		}
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("请求体必须包含 JSON 对象")
		}
		return fmt.Errorf("JSON 请求无效: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("请求体只能包含一个 JSON 值")
		}
		return fmt.Errorf("JSON 请求尾部无效: %w", err)
	}
	return nil
}

func writeData(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dataEnvelope{Data: value})
}

func writeError(w http.ResponseWriter, status int, code, message, field string) {
	writeDetailedError(w, status, code, message, field, nil)
}

func writeDetailedError(w http.ResponseWriter, status int, code, message, field string, details any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: errorBody{Code: code, Message: message, Field: field, Details: details}})
}
