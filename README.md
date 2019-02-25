# env
## SSL
```
WEBASIS_SSL=on|off
WEBASIS_SSL_CERT=cret_file
WEBASIS_SSL_KEY=key_file
```

## daemon
```
WEBASIS_LISTEN=host:port
WEBASIS_TOKEN=token_for_auth
```

## all of client
```
WEBASIS_WSYNC_SERVER_URL=ws[s]://host:port/wsync
WEBASIS_WRPC_SERVER_URL=http[s]://host:port/wrpc
WEBASIS_TOKEN=token_for_auth
```

# cmd

## daemon
- notify|content|(url|data) -> ok
- status/wsync/connected -> ok|count
- status/wsync/message -> ok|count
- status/wrpc/called -> ok|count
- log/open|name -> OK|id	WSYNC: logs,log:{id}|{line}|{created}
- log/close|id -> OK	WSYNC: logs,log:{id}|{line}|{created}
- log/all -> OK{|id,closed,size,name}
- log/get|id[|start[|max-num[|max-size]]] -> OK{|logs}
- log/append|id{|logs} -> OK WSYNC: logs,log:{id}|{line}|{created}
- log/delete|id -> OK WSYNC: logs,log:{id}
- log/stat|id -> OK|name|size:int|closed:bool|created:int
- alias: log/get/after -> log/get

## push
webasis [name=/dev/stdin]
push content to wsync's notify topic
read content from STDIN

## watch
watch the server's status

## rpc
webasis method {args}

## client
```
-> (R|S|U|B) topic {metas}
R: rpc
S: sub
U: unsup
B: boardcast
```

## log
webasis cmd {args}
- get: args=id
- delete|remove|rm: args={id}
- list|ls
- create: args=[name [bufsize=0]]
- append: args=[id   [bufsize=0]]
- stats
- stat args=id
- watch args=id



# http api
## notify
POST https://ws.mofon.top:8111/api/notify
```
{
	"title":"","content":"","token":""
}
```
```
curl https://ws.mofon.top:8111/api/notify -v -d "{\"title\":\"test\",\"content\":\"https://baidu.com/\",\"token\":\"${WEBASIS_TOKEN}\"}"
```
