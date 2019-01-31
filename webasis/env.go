package webasis

import (
	"context"
	"os"

	"github.com/webasis/wrpc"
)

var (
	// client
	WSyncServerURL = getenv("WEBASIS_WSYNC_SERVER_URL", "ws://localhost:8111/wsync")
	WRPCServerURL  = getenv("WEBASIS_WRPC_SERVER_URL", "http://localhost:8111/wrpc")
	Token          = getenv("WEBASIS_TOKEN", "")
)

func getenv(key, defv string) string {
	v := os.Getenv(key)
	if v == "" {
		return defv
	}
	return v
}

var rpc = wrpc.NewClient(WRPCServerURL, Token)

func Call(ctx context.Context, method string, args ...string) (wrpc.Resp, error) {
	return rpc.Call(ctx, method, args...)
}
