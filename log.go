package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	clitable "github.com/crackcomm/go-clitable"
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

func new_weblog(name string) *weblog {
	return &weblog{
		name:   name,
		logs:   make([]string, 0, 16),
		closed: false,
	}
}

// log/open|name -> OK|id	#After# log:new
// log/close|id -> OK	#After# log#id:close
// log/all -> OK{|id,name}
// log/get|id -> OK{|logs}
// log/append|id{|logs} -> OK #After# log#id:append
// log/delete|id ->OK #After# log#id:delete
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
				logs = append(logs, fmt.Sprintf("%s,%s", id, weblog.name))
			}
			retLogs <- logs
		}
		return wret.OK((<-retLogs)...)
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
	case "list":
		ids, names, err := webasis.LogAll(context.TODO())
		ExitIfErr(err)

		table := clitable.New([]string{"id", "name"})

		for i, id := range ids {
			name := names[i]
			table.AddRow(map[string]interface{}{"id": id, "name": name})
		}
		table.Print()

	case "sync":
		name := fmt.Sprintf("/dev/stdin#%d", os.Getpid())
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		ctx := context.TODO()
		id, err := webasis.LogOpen(ctx, name)
		ExitIfErr(err)

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			l := scanner.Text()

			ExitIfErr(webasis.LogAppend(ctx, id, l))
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading standard input:", err)
		}

		ExitIfErr(webasis.LogClose(ctx, id))
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
	fmt.Println("webasis list")
	fmt.Println("webasis sync [name]")
	fmt.Println("webasis get [id]")
	os.Exit(-2)
}
