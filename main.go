package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	clitable "github.com/crackcomm/go-clitable"
	"github.com/gorilla/websocket"
	"github.com/immofon/mlog"
	"github.com/webasis/webasis/webasis"
	"github.com/webasis/wrpc"
	"github.com/webasis/wrpc/wret"
	"github.com/webasis/wsync"
)

var (
	// server
	ServeSSL = getenv("WEBASIS_SSL", "off")
	SSLCert  = getenv("WEBASIS_SSL_CERT", "")
	SSLKey   = getenv("WEBASIS_SSL_KEY", "")

	ServeAddr = getenv("WEBASIS_LISTEN", "localhost:8111")

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

func daemon() {
	sync := wsync.NewServer()
	rpc := wrpc.NewServer()

	sync.Auth = func(token string, m wsync.AuthMethod, topic string) bool {
		if token == Token {
			return true
		}
		return false
	}
	rpc.Auth = func(r wrpc.Req) bool {
		if r.Token == Token {
			return true
		}
		return false
	}

	rpc.HandleFunc("notify", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 2 {
			return wret.Error("args")
		}

		content := r.Args[0]
		url := r.Args[1]
		sync.C <- func(sync *wsync.Server) {
			sync.Boardcast("notify", content, url)
		}
		return wret.OK()
	})

	EnableStatus(rpc, sync)
	EnableLog(rpc, sync)

	http.Handle("/wrpc", rpc)
	http.Handle("/wsync", sync)

	mlog.L().WithField("addr", ServeAddr).WithField("token", Token).Info("listen")
	if ServeSSL == "on" {
		mlog.L().Info("open ssl")
		mlog.L().Error(http.ListenAndServeTLS(ServeAddr, SSLCert, SSLKey, nil))
	} else {
		mlog.L().Error(http.ListenAndServe(ServeAddr, nil))
	}
}

func main() {
	mlog.TextMode()
	cmd := getenv("cmd", "rpc")
	switch cmd {
	case "daemon":
		daemon()
	case "push":
		push()
	case "notify":
		notify()
	case "client":
		client()
	case "watch":
		watch()
	case "rpc":
		rpc()
	case "log":
		log()
	}
}

func rpc() {
	if len(os.Args) < 2 {
		fmt.Println("webasis method {args}")
		return
	}

	method := os.Args[1]
	var args []string
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}

	resp, err := webasis.Call(context.TODO(), method, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}

	if resp.Status == wrpc.StatusOK {
		for _, ret := range resp.Rets {
			fmt.Println(ret)
		}
		os.Exit(0)
	} else {
		fmt.Fprintln(os.Stderr, resp.Status)
		for _, ret := range resp.Rets {
			fmt.Fprintln(os.Stderr, ret)
		}
		os.Exit(1)
	}
}

func notify() {
	resp, err := webasis.Call(context.TODO(), "notify", getenv("content", "hello"), getenv("url", "https://ws.mofon.top/"))
	if err != nil {
		panic(err)
	}
	fmt.Println(resp)
}
func client() {
	sync := wsync.NewClient(WSyncServerURL, Token)
	sync.OnTopic = func(topic string, metas ...string) {
		fmt.Println("t:", topic, metas)
	}
	sync.OnError = func(err error) {
		fmt.Println("error:", err)
	}
	sync.AfterOpen = func(conn *websocket.Conn) {
		go func() {
			sync.Sub("notify")

			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				raw := scanner.Text()
				data := strings.Split(raw, " ")
				method, topic, metas := wsync.DecodeData(data...)
				switch method {
				case "S":
					sync.Sub(topic)
				case "U":
					sync.Unsub(topic)
				case "B":
					sync.Boardcast(topic, metas...)
				case "R":
					resp, err := webasis.Call(context.TODO(), topic, metas...)
					if err != nil {
						fmt.Println(err)
					} else {
						fmt.Println(resp.Status, resp.Rets)
					}
				default:
					help()
				}
			}
			if err := scanner.Err(); err != nil {
				fmt.Fprintln(os.Stderr, "reading standard input:", err)
			}
		}()
	}

	for {
		sync.Serve()
	}
}

func push() {
	name := "/dev/stdin"
	if len(os.Args) > 1 {
		name = os.Args[1]
	}
	err := webasis.ShareFile(context.TODO(), name, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}

func watch() {
	connected := ""
	messageSent := ""
	rpcCount := ""
	ch := make(chan func(), 1)

	show := func() {
		fmt.Print("\x1B[1;1H\x1B[0J")
		table := clitable.New([]string{"key", "value"})

		table.AddRow(map[string]interface{}{"key": "count/wsync/connect", "value": connected})
		table.AddRow(map[string]interface{}{"key": "count/wsync/message", "value": messageSent})
		table.AddRow(map[string]interface{}{"key": "count/rpc/called", "value": rpcCount})

		table.Print()
	}
	go func() {
		for {
			time.Sleep(time.Second)
			resp, err := webasis.Call(context.TODO(), "status/wsync/connected")
			if err != nil {
				continue
			}

			if resp.Status == wrpc.StatusOK {
				ch <- func() {
					connected = resp.Rets[0]
				}
			}
		}
	}()
	go func() {
		for {
			time.Sleep(time.Second)
			resp, err := webasis.Call(context.TODO(), "status/wsync/message")
			if err != nil {
				continue
			}

			if resp.Status == wrpc.StatusOK {
				ch <- func() {
					messageSent = resp.Rets[0]
				}
			}
		}
	}()
	go func() {
		for {
			time.Sleep(time.Second)
			resp, err := webasis.Call(context.TODO(), "status/wrpc/called")
			if err != nil {
				continue
			}

			if resp.Status == wrpc.StatusOK {
				ch <- func() {
					rpcCount = resp.Rets[0]
				}
			}
		}
	}()

	for fn := range ch {
		fn()
		show()
	}
}

func help() {
	fmt.Println("(R|S|U|B) topic {metas}")
}
