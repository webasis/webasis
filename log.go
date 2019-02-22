package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	clitable "github.com/crackcomm/go-clitable"
	"github.com/gorilla/websocket"
	"github.com/webasis/webasis/webasis"
	"github.com/webasis/wrbac"
	"github.com/webasis/wrpc"
	"github.com/webasis/wrpc/wret"
	"github.com/webasis/wsync"
)

type weblog struct {
	name       string
	logs       []string
	closed     bool
	alwaysOpen bool
}

func (wl weblog) size() int {
	size := len(wl.logs) // size of '\n'
	for _, l := range wl.logs {
		size += len([]byte(l))
	}
	return size
}

func (wl weblog) Stat(id string) webasis.WebLogStat {
	return webasis.WebLogStat{
		Id:     id,
		Closed: wl.closed,
		Size:   wl.size(),
		Line:   len(wl.logs),
		Name:   wl.name,
	}

}

func new_weblog(name string) *weblog {
	return &weblog{
		name:       name,
		logs:       make([]string, 0, 16),
		closed:     false,
		alwaysOpen: false,
	}
}

type notifyLog struct {
	id      string
	version string
}

const DefaultBufSize = 0

// log/open|name -> OK|id	#After# log:new
// log/close|id -> OK	#After# log#id:stat, log:stat
// log/all -> OK{|id,closed,size,name}
// log/get|id[|start[|max_num]] -> OK{|logs}
// log/append|id{|logs} -> OK #After# log#id:stat, log:stat
// log/delete|id ->OK #After# log#id:stat, log:stat
// log/stat|id ->OK|name|size:int|closed:bool
// alias: log/get/after -> log/get
func EnableLog(rpc *wrpc.Server, sync *wsync.Server) {

	weblogs := make(map[string]*weblog) // map[id]Log
	nextId := 1
	notify_logs := make(map[string]*notifyLog) // map[name]

	getNextId := func(token string) string {
		id := nextId
		nextId++

		name, _ := wrbac.FromToken(token)
		return name + "@" + strconv.Itoa(id)
	}

	getNotifyLog := func(token string) *notifyLog {
		name, _ := wrbac.FromToken(token)
		id := name + "@" + "notification"

		nl := notify_logs[name]
		if nl == nil {
			wl := new_weblog("notification_log")
			wl.alwaysOpen = true
			weblogs[id] = wl

			nl = &notifyLog{
				id:      id,
				version: fmt.Sprint(time.Now().Unix()),
			}
			notify_logs[name] = nl
		}
		return nl
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
			new_id := getNextId(r.Token)
			weblogs[new_id] = new_weblog(name)
			id <- new_id

			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast("log:new")

				sync.Boardcast("logs")
			}
		}
		return wret.OK(<-id)
	})

	rpc.HandleFunc("log/notify", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		content := r.Args[0]

		id := make(chan string, 1)
		ch <- func() {
			nl := getNotifyLog(r.Token)
			id <- nl.id
		}

		return rpc.CallWithoutAuth(wrpc.Req{
			Token:  r.Token,
			Method: "log/append",
			Args:   []string{<-id, content},
		})
	})

	rpc.HandleFunc("log/notify/info", func(r wrpc.Req) wrpc.Resp {
		nlch := make(chan notifyLog, 1)
		ch <- func() {
			nl := getNotifyLog(r.Token)
			nlch <- *nl
		}
		nl := <-nlch
		return wret.OK(nl.id, nl.version)
	})

	rpc.HandleFunc("log/close", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		id := r.Args[0]

		retOK := make(chan bool, 1)
		statCh := make(chan webasis.WebLogStat, 1)
		defer close(retOK)
		ch <- func() {
			defer close(statCh)
			weblog, ok := weblogs[id]
			statCh <- weblog.Stat(id)
			if ok {
				if weblog.alwaysOpen {
					retOK <- false
					return
				}
				weblog.closed = true
				retOK <- true
			} else {
				retOK <- false
			}
		}

		stat := <-statCh
		if <-retOK {
			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(fmt.Sprintf("log#%s:stat", id))
				sync.Boardcast("log:stat")

				// new
				sync.Boardcast("logs")
				sync.Boardcast("log:"+id, webasis.Int(stat.Line))
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
				logs = append(logs, weblog.Stat(id).Encode())
			}
			retLogs <- logs
		}
		logs := <-retLogs
		sort.Slice(logs, func(i, j int) bool {
			return logs[i] < logs[j]
		})
		return wret.OK(logs...)
	})

	getAfter := func(id string, start, max_num int) wrpc.Resp {
		if max_num < 1 {
			max_num = 1
		}

		retOK := make(chan bool, 1)
		retLog := make(chan []string, 1)
		defer close(retOK)
		ch <- func() {
			defer close(retLog)
			weblog, ok := weblogs[id]
			if !ok {
				retOK <- false
				return
			}

			if start < len(weblog.logs) {
				end := start + max_num
				if end > len(weblog.logs) {
					end = len(weblog.logs)
				}
				retLog <- weblog.logs[start:end]
			}
			retOK <- true
			return
		}

		if <-retOK {
			return wret.OK((<-retLog)...)
		} else {
			return wret.Error("not_found")
		}
	}

	// log/get|id[|start[|max_num]] -> OK{|logs}
	rpc.HandleFunc("log/get", func(r wrpc.Req) wrpc.Resp {
		fields := webasis.Fields(r.Args)
		id := fields.Get(0, "")
		start := fields.Int(1, 0)
		max_num := fields.Int(2, 1000000)
		if id == "" || start < 0 || max_num < 1 {
			return wret.Error("args")
		}

		return getAfter(id, start, max_num)
	})

	rpc.Alias("log/get", "log/get/after")

	rpc.HandleFunc("log/delete", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		id := r.Args[0]
		ch <- func() {
			if wl := weblogs[id]; wl.alwaysOpen {
				return
			}

			delete(weblogs, id)
			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(fmt.Sprintf("log#%s:stat", id))
				sync.Boardcast("log:stat")

				// new
				sync.Boardcast("logs")
				sync.Boardcast("log:"+id, webasis.Int(-1))
			}

		}
		return wret.OK()
	})

	// log/stat|id ->OK|name|size:int|closed:bool
	rpc.HandleFunc("log/stat", func(r wrpc.Req) wrpc.Resp {
		if len(r.Args) != 1 {
			return wret.Error("args")
		}

		id := r.Args[0]

		retCh := make(chan webasis.WebLogStat, 1)
		ch <- func() {
			defer close(retCh)
			weblog, ok := weblogs[id]
			if ok {
				retCh <- weblog.Stat(id)
			}
		}
		ret := <-retCh
		if ret.Id != id {
			return wret.Error("not_found")
		}
		return wret.OK(ret.Name, webasis.Int(ret.Size), webasis.Int(ret.Line), webasis.Bool(ret.Closed))
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

			stat := weblog.Stat(id)

			sync.C <- func(sync *wsync.Server) {
				sync.Boardcast(fmt.Sprintf("log#%s:stat", id))
				sync.Boardcast("log:stat")

				// new
				sync.Boardcast("logs")
				sync.Boardcast("log:"+id, webasis.Int(stat.Line))

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

func logs_ls(need_refresh bool) {
	stats, err := webasis.LogAll(context.TODO())
	ExitIfErr(err)

	table := clitable.New([]string{"id", "name", "size", "line", "closed"})

	for _, stat := range stats {
		table.AddRow(map[string]interface{}{
			"id":     stat.Id,
			"name":   stat.Name,
			"line":   stat.Line,
			"size":   stat.Size,
			"closed": stat.Closed,
		})
	}

	if need_refresh {
		fmt.Print("\x1B[1;1H\x1B[0J")
	}
	table.Print()
}

func log() {
	cmd := "help"

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
			return
		}

		log_get(id, false)
	case "delete", "remove", "rm":
		if len(os.Args) < 3 {
			log_help()
			return
		}
		ids := os.Args[2:]

		for _, id := range ids {
			ExitIfErr(webasis.LogDelete(context.TODO(), id))
		}
	case "list", "ls":
		logs_ls(false)
	case "create":
		name := fmt.Sprintf("/dev/stdin#%d", os.Getpid())
		if len(os.Args) > 2 {
			name = os.Args[2]
		}
		bufsize_raw := ""
		if len(os.Args) > 3 {
			bufsize_raw = os.Args[3]
		}

		bufsize, err := strconv.Atoi(bufsize_raw)
		if err != nil {
			bufsize = DefaultBufSize
		}

		ctx := context.TODO()
		id, err := webasis.LogOpen(ctx, name)
		ExitIfErr(err)

		log_append(id, bufsize)
	case "append":
		id := ""
		if len(os.Args) > 2 {
			id = os.Args[2]
		}
		if id == "" {
			log_help()
			return
		}

		bufsize_raw := ""
		if len(os.Args) > 3 {
			bufsize_raw = os.Args[3]
		}

		bufsize, err := strconv.Atoi(bufsize_raw)
		if err != nil {
			bufsize = DefaultBufSize
		}

		log_append(id, bufsize)
	case "stats":
		sync := wsync.NewClient(WSyncServerURL, Token)
		sync.AfterOpen = func(_ *websocket.Conn) {
			go sync.Sub("log:new", "log:stat")
		}

		needUpdate := make(chan bool, 1)
		go func() {
			for range needUpdate {
				logs_ls(true)
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
	case "stat":
		id := ""
		if len(os.Args) > 2 {
			id = os.Args[2]
		}
		if id == "" {
			log_help()
			return
		}

		stat, err := webasis.LogStat(context.TODO(), id)
		ExitIfErr(err)
		fmt.Println("Name:", stat.Name)
		fmt.Println("Size:", stat.Size)
		fmt.Println("Line:", stat.Line)
		fmt.Println("Closed:", stat.Closed)
	case "watch":
		id := ""
		if len(os.Args) > 2 {
			id = os.Args[2]
		}
		if id == "" {
			log_help()
			return
		}
		watch_log(id)
	case "help":
		log_help()
	default:
		log_help()
	}
}

func watch_log(id string) {
	sync := wsync.NewClient(WSyncServerURL, Token)
	sync.AfterOpen = func(_ *websocket.Conn) {
		go sync.Sub(fmt.Sprintf("log#%s:stat", id))
	}

	needUpdate := make(chan bool, 1)
	go func() {
		index := 0
		for range needUpdate {
			stat, err := webasis.LogStat(context.TODO(), id)
			ExitIfErr(err)
			logs, err := webasis.LogGetAfter(context.TODO(), id, index)
			ExitIfErr(err)
			index += len(logs)

			for _, line := range logs {
				fmt.Println(line)
			}

			if stat.Closed {
				os.Exit(0)
			}
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
}

func log_get(id string, refresh bool) {
	logs, err := webasis.LogGet(context.TODO(), id)
	ExitIfErr(err)

	if refresh {
		fmt.Print("\x1B[1;1H\x1B[0J")
	}
	for _, l := range logs {
		fmt.Println(l)
	}
}

func log_append(id string, bufsize int) {
	ctx := context.TODO()
	in, e := webasis.LogAppendWithBuf(ctx, bufsize, id)
	go func() {
		ExitIfErr(<-e) // for exit in real-time
	}()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		in <- scanner.Text()
	}
	close(in)
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
	ExitIfErr(<-e) // just for sync
}

func ExitIfErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}

func log_help() {
	fmt.Println("help:")
	fmt.Println("\t", "webasis create [name [bufsize=0]] ")
	fmt.Println("\t", "webasis append [id [bufsize=0]] ")
	fmt.Println("\t", "webasis list|ls")
	fmt.Println("\t", "webasis get id")
	fmt.Println("\t", "webasis stat id")
	fmt.Println("\t", "webasis delete|remove|rm id {id}")
	fmt.Println("\t", "webasis help")
	os.Exit(-2)
}
