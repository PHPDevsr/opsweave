//go:build linux || darwin

package dockeraction

import (
	"context"
	"net"
	"strings"
)

func dialLocal(ctx context.Context, endpoint string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", strings.TrimPrefix(endpoint, "unix://"))
}
