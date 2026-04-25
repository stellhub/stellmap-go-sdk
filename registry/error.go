package registry

import (
	"errors"
	"fmt"
)

// APIError 表示 StellMap 服务端返回的错误。
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
	RequestID  string
	LeaderID   uint64
	LeaderAddr string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return fmt.Sprintf("stellmap api error: status=%d code=%s", e.HTTPStatus, e.Code)
	}
	return fmt.Sprintf("stellmap api error: status=%d code=%s message=%s", e.HTTPStatus, e.Code, e.Message)
}

// IsNotLeader 判断是否为 not_leader 错误。
func (e *APIError) IsNotLeader() bool {
	return e != nil && e.Code == "not_leader"
}

// IsRevisionExpired 判断是否为 revision_expired 错误。
func (e *APIError) IsRevisionExpired() bool {
	return e != nil && e.Code == "revision_expired"
}

// IsAPIError 判断 err 是否为 APIError。
func IsAPIError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr)
}
