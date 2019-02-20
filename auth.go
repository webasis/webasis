package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/webasis/wrbac"
	"github.com/webasis/wrpc"
	"github.com/webasis/wsync"
)

type User struct {
	Secret string   `json:"secret"`
	Roles  []string `json:"roles"`
}
type AuthModel map[string]map[string]User // map[name]map[comment]User

func EnableAuth(rpc *wrpc.Server, sync *wsync.Server) {
	rbac := wrbac.New()
	rbac.Register("root", &wrbac.Role{
		Sync: func(token string, m wsync.AuthMethod, topic string) bool {
			return true
		},
		RPC: func(r wrpc.Req) bool {
			return true
		},
	})

	rbac.Register("notification_sender", &wrbac.Role{
		Sync: func(token string, m wsync.AuthMethod, topic string) bool {
			return false
		},
		RPC: func(r wrpc.Req) bool {
			return r.Method == "notify"
		},
	})
	rbac.Register("notification_receiver", &wrbac.Role{
		Sync: func(token string, m wsync.AuthMethod, topic string) bool {
			return true
		},
		RPC: func(r wrpc.Req) bool {
			switch r.Method {
			case "log/notify/info", "log/get/after":
				return true
			}
			return false
		},
	})

	var authModel AuthModel
	authJsonData, err := ioutil.ReadFile(AuthFile)
	if err != nil {
		fmt.Println("require set $WEBASIS_AUTH_FILE")
		panic(err)
	}
	if err := json.Unmarshal(authJsonData, &authModel); err != nil {
		panic(err)
	}

	configFailure := false
	for name, client := range authModel {
		fmt.Println("name:", name)
		for desc, user := range client {
			for _, role := range user.Roles {
				if !rbac.Check(role) {
					fmt.Printf("config error: %s.%s unregistered_role: %s\n", name, desc, role)
					configFailure = true
				}
			}
			if configFailure {
				os.Exit(1)
			}
			rbac.Load(name, user.Secret, user.Roles...)
			fmt.Printf("\t%s: <token> %s\n", desc, wrbac.ToToken(name, user.Secret))
		}
	}

	sync.Auth = rbac.AuthSync
	rpc.Auth = rbac.AuthRPC

}
