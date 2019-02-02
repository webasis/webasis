package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"

	clitable "github.com/crackcomm/go-clitable"
	"github.com/gorilla/websocket"
	"github.com/webasis/webasis/webasis"
	"github.com/webasis/wrpc"
	"github.com/webasis/wrpc/wret"
	"github.com/webasis/wsync"
)

type weblog struct {
	name   string
	logs   []string
	closed bool
}

func (wl weblog) size() int {
	size := len(wl.logs) // size of '\n'
	for _, l := range wl.logs {
		size += len([]byte(l))
	}
	return size
}

func new_weblog(name string) *weblog {
	return &weblog{
		name:   name,
		logs:   make([]string, 0, 16),
		closed: false,
	}
}

// log/open|name -> OK|id	#After# log:new
// log/close|id -> OK	#After# log#id:close, log:close
// log/all -> OK{|id,name}
// log/get|id -> OK{|logs}
// log/append|id{|logs} -> OK #After# log#id:append, log:append
// log/delete|id ->OK #After# log#id:delete, log:delete
func EnableLog(rpc *wrpc.Server, sync *wsync.Server) {
	weblogs := make(map[string]*weblog) // map[id]Log
	nextId := 1

	getNextId := func() string {
		id := nextId
		nextId++
		return fmt.Sprint(id)
	}

	ch := make(chan func(), 1000)
	go func() {
		for fn := range ch {
			fn()
		}
	}()

	rpc.HandleFunc("log/open", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		name := r.Args[0]
		id := make(chan string, 1)
		defer close(id)
		ch <- func() {
			new_id := getNextId()
			weblogs[new_id] = new_weblog(name)
			id <- new_id

			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast("log:new")
			}
		}
		return wret.OK(<-id)
	})

	rpc.HandleFunc("log/close", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		id := r.Args[0]

		retOK := make(chan bool, 1)
		defer close(retOK)
		ch <- func() {
			if weblog, ok := weblogs[id]; ok {
				weblog.closed = true
				retOK <- true
			} else {
				retOK <- false
			}
		}
		if <-retOK {
			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(fmt.Sprintf("log#%s:close", id))
				sync.Boardcast("log:close")
			}

			return wret.OK()
		} else {
			return wret.Error("not_found")
		}
	})

	rpc.HandleFunc("log/all", func(r wrpc.Req) wrpc.Resp {
		retLogs := make(chan []string, 1)
		defer close(retLogs)
		ch <- func() {
			logs := make([]string, 0, len(weblogs))
			for id, weblog := range weblogs {
				stat := webasis.WebLogStat{
					Id:     id,
					Closed: weblog.closed,
					Size:   weblog.size(),
					Name:   weblog.name,
				}
				logs = append(logs, stat.Encode())
			}
			retLogs <- logs
		}
		logs := <-retLogs
		sort.Slice(logs, func(i, j int) bool {
			return logs[i] < logs[j]
		})
		return wret.OK(logs...)
	})

	rpc.HandleFunc("log/get", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		id := r.Args[0]
		retOK := make(chan bool, 1)
		retLog := make(chan []string, 1)
		defer close(retOK)
		defer close(retLog)
		ch <- func() {
			weblog, ok := weblogs[id]
			if !ok {
				retOK <- false
				return
			}

			retLog <- weblog.logs
			retOK <- true
			return
		}

		if <-retOK {
			return wret.OK((<-retLog)...)
		} else {
			return wret.Error("not_found")
		}
	})

	rpc.HandleFunc("log/delete", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		id := r.Args[0]
		ch <- func() {
			delete(weblogs, id)
			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(fmt.Sprintf("log#%s:delete", id))
				sync.Boardcast("log:delete")
			}

		}
		return wret.OK()
	})

	rpc.HandleFunc("log/append", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) < 1 {
			return wret.Error("args")
		}

		id := r.Args[0]

		var logs []string
		if len(r.Args) > 1 {
			logs = r.Args[1:]
		}

		reason := ""
		retOK := make(chan bool, 1)
		ch <- func() {
			weblog, ok := weblogs[id]
			if !ok {
				reason = "not_found"
				retOK <- false
				return
			}

			if weblog.closed {
				reason = "closed"
				retOK <- false
				return
			}

			for _, log := range logs {
				weblog.logs = append(weblog.logs, log)
			}

			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(fmt.Sprintf("log#%s:append", id))
				sync.Boardcast("log:append")
			}
			retOK <- true
		}

		if <-retOK {
			return wret.OK()
		} else {
			return wret.Error(reason)
		}
	})
}

func logs_ls() {
	stats, err := webasis.LogAll(context.TODO())
	ExitIfErr(err)

	table := clitable.New([]string{"id", "name", "size", "closed"})

	for _, stats := range stats {
		id := stats.Id
		size := stats.Size
		name := stats.Name
		closed := stats.Closed
		table.AddRow(map[string]interface{}{"id": id, "name": name, "size": size, "closed": closed})
	}
	table.Print()
}

func log() {
	cmd := "sync"

	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "get":
		id := ""
		if len(os.Args) > 2 {
			id = os.Args[2]
		}
		if id == "" {
			log_help()
		}

		logs, err := webasis.LogGet(context.TODO(), id)
		ExitIfErr(err)
		for _, l := range logs {
			fmt.Println(l)
		}
	case "delete", "remove", "rm":
		id := ""
		if len(os.Args) > 2 {
			id = os.Args[2]
		}
		if id == "" {
			log_help()
		}

		ExitIfErr(webasis.LogDelete(context.TODO(), id))
	case "list", "ls":
		logs_ls()
	case "sync":
		name := fmt.Sprintf("/dev/stdin#%d", os.Getpid())
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		ctx := context.TODO()
		in, e := webasis.LogSync(ctx, 50000, name)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			in <- scanner.Text()
		}
		close(in)
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading standard input:", err)
		}

		ExitIfErr(<-e)
	case "stats":
		sync := wsync.NewClient(WSyncServerURL, Token)
		sync.AfterOpen = func(_ *websocket.Conn) {
			go sync.Sub("log:new", "log:delete", "log:append", "log:close")
		}

		needUpdate := make(chan bool, 1)
		go func() {
			for range needUpdate {
				fmt.Print("\x1B[1;1H\x1B[0J")
				logs_ls()
			}
		}()

		sync.OnTopic = func(topic string, metas ...string) {
			select {
			case needUpdate <- true:
			default:
			}
		}

		for {
			sync.Serve()
		}
	default:
		log_help()
	}
}

func ExitIfErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}

func log_help() {
	fmt.Println("help:")
	fmt.Println("\t", "webasis sync [name]")
	fmt.Println("\t", "webasis list|ls")
	fmt.Println("\t", "webasis get id")
	fmt.Println("\t", "webasis delete|remove|rm id")
	os.Exit(-2)
}
