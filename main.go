package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	clitable "github.com/crackcomm/go-clitable"
	"github.com/gorilla/websocket"
	"github.com/immofon/mlog"
	"github.com/webasis/webasis/webasis"
	"github.com/webasis/wlock"
	"github.com/webasis/wrbac"
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

	AuthFile = getenv("WEBASIS_AUTH_FILE", "")

	NotificationURL = getenv("WEBASIS_NOTIFICATION_URL", "http://"+ServeAddr+"/notification")

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

type notifyReq struct {
	Content string `json:"content"`
	Token   string `json:"token"`
}

type Notify struct {
	Time int64    `json:"time"`
	Type string   `json:"type"`
	Data []string `json:"data"`
}

func daemon() {
	sync := wsync.NewServer()
	rpc := wrpc.NewServer()
	rpc.MaxContentLength = 1024 * 1024 * 10 // 10MiB

	// open gc
	go func() {
		for {
			time.Sleep(time.Minute * 5)
			sync.C <- func(sync *wsync.Server) {
				sync.GC()
				// TODO fix: ref to outer resourece
				// e.g. log:{name}@{id}
			}
		}
	}()

	rpc.HandleFunc("notify", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		name, _ := wrbac.FromToken(r.Token)

		content := r.Args[0]

		raw, err := json.Marshal(Notify{
			Time: time.Now().Unix(),
			Type: "text",
			Data: []string{content},
		})
		if err != nil {
			return wret.IError(err.Error())
		}

		resp := rpc.CallWithoutAuth(wrpc.Req{
			Token:  r.Token,
			Method: "log/append",
			Args:   []string{name + "@notification", string(raw)},
		})
		if resp.Status == wrpc.StatusOK {
			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(name+"@notification", content, NotificationURL)
			}
		}
		return resp
	})

	// adminboardcast|topic{|metas}
	rpc.HandleFunc("admin/boardcast", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) < 1 {
			return wret.Error("args")
		}
		topic := r.Args[0]
		var metas []string
		if len(r.Args) > 1 {
			metas = r.Args[1:]
		}
		sync.C <- func(sync *wsync.Server) {
			sync.Boardcast(topic, metas...)
		}
		return wret.OK()
	})

	EnableAuth(rpc, sync)
	EnableStatus(rpc, sync)
	EnableLog(rpc, sync)

	lm := wlock.New()
	wlock.Enable(rpc, lm)

	http.Handle("/wrpc", rpc)
	http.Handle("/wsync", sync)
	http.HandleFunc("/api/notify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req notifyReq
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, err)
			return
		}

		ret := rpc.Call(wrpc.Req{
			Token:  req.Token,
			Method: "notify",
			Args:   []string{req.Content},
		})
		switch ret.Status {
		case wrpc.StatusOK:
			w.WriteHeader(http.StatusOK)
		case wrpc.StatusAuth:
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	})

	student_info_ch := make(chan StudentInfo, 200)
	go func() {
		f, err := os.OpenFile("students_info.json.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			mlog.L().Error(err)
			return
		}
		defer f.Close()

		out := json.NewEncoder(f)
		for info := range student_info_ch {
			out.Encode(info)
			f.Sync()
		}
	}()

	http.HandleFunc("/api/freetimecollector", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var info StudentInfo
		err := json.NewDecoder(r.Body).Decode(&info)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if info.Time == 0 {
			info.Time = int(time.Now().Unix())
		}

		// write info
		student_info_ch <- info

		w.WriteHeader(http.StatusOK)
	})

	mlog.L().WithField("addr", ServeAddr).Info("listen")
	if ServeSSL == "on" {
		mlog.L().Info("open ssl")
		mlog.L().Error(http.ListenAndServeTLS(ServeAddr, SSLCert, SSLKey, nil))
	} else {
		mlog.L().Error(http.ListenAndServe(ServeAddr, nil))
	}
}

type FreeTime struct {
	Week    string `json:"week"`
	Lession string `json:"lession"`
}

type StudentInfo struct {
	Name      string     `json:"student_name"`
	ID        string     `json:"student_id"`
	Class     string     `json:"student_class"`
	Token     string     `json:"token"`
	FreeTimes []FreeTime `json:"free_time"`
	Time      int        `json:"time"`
}

func main() {
	mlog.TextMode()
	cmd := getenv("cmd", "rpc")
	switch cmd {
	case "daemon":
		daemon()
	case "check":
		wrbac_check()
	case "push":
		push()
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
		fmt.Fprintf(os.Stderr, "\x1b[31m%s \x1b[33m%s\x1b[0m\n", resp.Status, strings.Join(resp.Rets, "\x1b[90m|\x1b[33m"))
		os.Exit(1)
	}
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
