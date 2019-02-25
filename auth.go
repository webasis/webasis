package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/webasis/wrbac"
	"github.com/webasis/wrpc"
	"github.com/webasis/wsync"
)

type User struct {
	Secret string   `json:"secret"`
	Mask   string   `json:"mask"`
	Roles  []string `json:"roles"`
}
type AuthModel map[string]map[string]User // map[name]map[comment]User

func EnableAuth(rpc *wrpc.Server, sync *wsync.Server) {
	rbac := wrbac.New()
	wrbac_register_role(rbac)
	wrbac_load(rbac)
	sync.Auth = rbac.AuthSync
	rpc.Auth = rbac.AuthRPC
}

func wrbac_check() {
	rbac := wrbac.New()
	wrbac_register_role(rbac)
	wrbac_load(rbac)
}

func wrbac_register_role(rbac *wrbac.Table) {
	rbac.Register("root", &wrbac.Role{
		Sync: func(token string, m wsync.AuthMethod, topic string) bool {
			return true
		},
		RPC: func(r wrpc.Req) bool {
			return true
		},
	})

	rbac.Register("mask_user", &wrbac.Role{
		Sync: func(token string, m wsync.AuthMethod, topic string) bool {
			if m == wsync.AuthMethod_Boardcast {
				return false
			}
			return true
		},
		RPC: func(r wrpc.Req) bool {
			name, _ := wrbac.FromToken(r.Token)

			switch r.Method {
			case "log/append", "log/get", "log/close", "log/stat", "log/delete":
				if len(r.Args) > 0 && strings.HasPrefix(r.Args[0], name+"@") {
					return true
				} else {
					return false
				}
			}

			if strings.HasPrefix(r.Method, "admin") {
				return false
			}
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
			case "log/get":
				return true
			}
			return false
		},
	})
}

func wrbac_load(rbac *wrbac.Table) {
	authModel := get_auth_model()
	configFailure := false
	for name, client := range authModel {
		if !configFailure {
			fmt.Println("name:", name)
		}

		for desc, user := range client {
			if !rbac.Check(user.Mask) {
				fmt.Printf("\x1b[31mconfig error: %s.%s unregistered_role: %s\n\x1b[0m", name, "<mask>", user.Mask)
				configFailure = true
			}
			for _, role := range user.Roles {
				if !rbac.Check(role) {
					fmt.Printf("\x1b[31mconfig error: %s.%s unregistered_role: %s\n\x1b[0m", name, desc, role)
					configFailure = true
				}
			}

			rbac.Load(name, user.Secret, user.Mask, user.Roles...)
			if !configFailure {
				fmt.Printf("    %s\t%s\n", desc, wrbac.ToToken(name, user.Secret))
			}
		}
	}
	if configFailure {
		os.Exit(1)
	}
}

func get_auth_model() AuthModel {
	var authModel AuthModel
	authJsonData, err := ioutil.ReadFile(AuthFile)
	if err != nil {
		fmt.Println("require set $WEBASIS_AUTH_FILE")
		panic(err)
	}
	if err := json.Unmarshal(authJsonData, &authModel); err != nil {
		panic(err)
	}
	return authModel
}
