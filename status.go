package main

import (
	"fmt"

	"github.com/webasis/wrpc"
	"github.com/webasis/wrpc/wret"
	"github.com/webasis/wsync"
)

// status/wsync/connected -> OK|count
// status/wsync/message -> OK|count
// status/wrpc/called -> OK|count
func EnableStatus(rpc *wrpc.Server, sync *wsync.Server) {
	rpc.HandleFunc("status/wsync/connected", func(r wrpc.Req) wrpc.Resp {
		ch := make(chan int, 1)
		sync.C <- func(sync *wsync.Server) {
			ch <- len(sync.Agents)
		}
		return wret.OK(fmt.Sprint(<-ch))
	})

	rpc.HandleFunc("status/wsync/message", func(r wrpc.Req) wrpc.Resp {
		ch := make(chan int, 1)
		sync.C <- func(sync *wsync.Server) {
			ch <- sync.MessageSent
		}
		return wret.OK(fmt.Sprint(<-ch))
	})

	rpc.HandleFunc("status/wrpc/called", func(r wrpc.Req) wrpc.Resp {
		ss := rpc.Status()
		return wret.OK(fmt.Sprint(ss.Count))
	})
}
