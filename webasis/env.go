package webasis

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/webasis/wrpc"
)

var (
	// client
	WSyncServerURL = getenv("WEBASIS_WSYNC_SERVER_URL", "ws://localhost:8111/wsync")
	WRPCServerURL  = getenv("WEBASIS_WRPC_SERVER_URL", "http://localhost:8111/wrpc")
	Token          = getenv("WEBASIS_TOKEN", "")

	Debug = getenv("WEBASIS_DEBUG", "off") // on|off
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
	resp, err := rpc.Call(ctx, method, args...)
	if Debug == "on" {
		from := method
		if len(args) > 0 {
			from += "|" + strings.Join(args, "|")
		}
		to := string(resp.Status)
		if len(resp.Rets) > 0 {
			to += "|" + strings.Join(resp.Rets, "|")
		}
		fmt.Fprintf(os.Stderr, "\x1B[92m%s -> %s\n\x1B[0m", from, to)
	}

	return resp, err
}
