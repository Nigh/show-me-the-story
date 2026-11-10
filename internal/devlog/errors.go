package devlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"syscall"
)

// ErrorAttrs preserves standard error diagnostics without persisting arbitrary
// Error() strings, which may contain complete model responses or user input.
func ErrorAttrs(err error) []any {
	if err == nil {
		return nil
	}
	attrs := []any{"error_type", fmt.Sprintf("%T", err)}
	var errno syscall.Errno
	var syntax *json.SyntaxError
	var fieldType *json.UnmarshalTypeError
	switch {
	case errors.Is(err, context.Canceled):
		attrs = append(attrs, "error_kind", "canceled")
	case errors.Is(err, context.DeadlineExceeded):
		attrs = append(attrs, "error_kind", "deadline_exceeded")
	case errors.As(err, &errno):
		attrs = append(attrs, "error_kind", "os_error", "errno", int(errno), "os_error", errno.Error())
	case errors.As(err, &syntax):
		attrs = append(attrs, "error_kind", "invalid_json", "json_offset", syntax.Offset)
	case errors.As(err, &fieldType):
		attrs = append(attrs, "error_kind", "invalid_json_type", "json_offset", fieldType.Offset)
	}
	return attrs
}
