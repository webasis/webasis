package webasis

import (
	"context"
	"fmt"
	"strings"
)

func LogOpen(ctx context.Context, name string) (id string, err error) {
	resp, err := Call(ctx, "log/open", name)
	err = resp.Error(err, 1)
	if err != nil {
		return "", err
	}

	id = resp.Rets[0]
	return id, nil
}

func LogClose(ctx context.Context, id string) error {
	resp, err := Call(ctx, "log/close", id)
	return resp.Error(err, 0)
}

func LogDelete(ctx context.Context, id string) error {
	resp, err := Call(ctx, "log/delete", id)
	return resp.Error(err, 0)
}

func LogAppend(ctx context.Context, id string, logs ...string) error {
	args := make([]string, 0, len(logs)+1)
	args = append(args, id)
	for _, l := range logs {
		args = append(args, l)
	}

	resp, err := Call(ctx, "log/append", args...)
	return resp.Error(err, 0)
}

func LogGet(ctx context.Context, id string) (logs []string, err error) {
	resp, err := Call(ctx, "log/get", id)
	err = resp.Error(err, -1)
	if err != nil {
		return nil, err
	}

	return resp.Rets, nil
}

func LogAll(ctx context.Context) (ids []string, names []string, err error) {
	resp, err := Call(ctx, "log/all")
	err = resp.Error(err, -1)
	if err != nil {
		return nil, nil, err
	}

	ids = make([]string, len(resp.Rets))
	names = make([]string, len(resp.Rets))
	for i, ret := range resp.Rets {
		data := strings.SplitN(ret, ",", 2)
		fmt.Println(ret, "|", data)
		ids[i] = data[0]
		names[i] = data[1]
	}

	return ids, names, nil
}

// chan in MUST be closed by user
func LogSync(ctx context.Context, bufsize int, name string) (in chan<- string, e <-chan error) {
	ch := make(chan string, 100)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			for range ch {
			}
		}()
		defer close(errCh)

		id, err := LogOpen(ctx, name)
		if err != nil {
			errCh <- err
			return
		}

		for line := range ch {
			err = LogAppend(ctx, id, line)
			if err != nil {
				errCh <- err
				return
			}
		}

		err = LogClose(ctx, id)
		if err != nil {
			errCh <- err
			return
		}
	}()

	return ch, errCh
}
