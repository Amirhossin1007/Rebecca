package nodecontroller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func friendlyNodeError(action string, nodeID int64, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("node %d %s timed out", nodeID, action)
	}
	if st, ok := status.FromError(err); ok {
		detail := strings.TrimSpace(st.Message())
		if detail == "" {
			detail = st.Code().String()
		}
		switch st.Code() {
		case codes.Unavailable:
			return fmt.Errorf("node %d is unavailable during %s: %s", nodeID, action, detail)
		case codes.Unauthenticated, codes.PermissionDenied:
			return fmt.Errorf("node %d rejected %s authentication: %s", nodeID, action, detail)
		case codes.FailedPrecondition:
			return fmt.Errorf("node %d cannot run %s yet: %s", nodeID, action, detail)
		case codes.InvalidArgument:
			return fmt.Errorf("node %d received invalid %s request: %s", nodeID, action, detail)
		default:
			return fmt.Errorf("node %d %s failed: %s", nodeID, action, detail)
		}
	}
	message := strings.TrimSpace(err.Error())
	if strings.Contains(strings.ToLower(message), "certificate") || strings.Contains(strings.ToLower(message), "tls") {
		return fmt.Errorf("node %d TLS/certificate error during %s: %s", nodeID, action, message)
	}
	if strings.Contains(strings.ToLower(message), "connection refused") || strings.Contains(strings.ToLower(message), "no such host") {
		return fmt.Errorf("node %d connection error during %s: %s", nodeID, action, message)
	}
	return fmt.Errorf("node %d %s failed: %s", nodeID, action, message)
}
