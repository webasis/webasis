package webasis

import (
	"context"
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

func LogStat(ctx context.Context, id string) (stat WebLogStat, err error) {
	resp, err := Call(ctx, "log/stat", id)
	err = resp.Error(err, 3)
	if err != nil {
		return WebLogStat{}, err
	}

	fields := Fields(resp.Rets)

	return WebLogStat{
		Id:     id,
		Name:   fields.Get(0, ""),
		Size:   fields.Int(1, 0),
		Closed: fields.Bool(2, true),
	}, nil
}

type WebLogStat struct {
	Id     string
	Name   string
	Closed bool
	Size   int
}

func (stat WebLogStat) Encode() string {
	return strings.Join([]string{stat.Id, Bool(stat.Closed), Int(stat.Size), stat.Name}, ",")
}
func DecodeWebLogStat(raw string) WebLogStat {
	data := strings.SplitN(raw, ",", 4)
	fields := Fields(data)
	return WebLogStat{
		Id:     fields.Get(0, ""),
		Closed: fields.Bool(1, true),
		Size:   fields.Int(2, 0),
		Name:   fields.Get(3, ""),
	}
}

func LogAll(ctx context.Context) (stats []WebLogStat, err error) {
	resp, err := Call(ctx, "log/all")
	err = resp.Error(err, -1)
	if err != nil {
		return nil, err
	}

	stats = make([]WebLogStat, len(resp.Rets))
	for i, ret := range resp.Rets {
		stats[i] = DecodeWebLogStat(ret)
	}

	return stats, nil
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

		buf := make([]string, 0, 1024)
		size := 0
		for line := range ch {
			if (size + len(buf)) <= bufsize {
				buf = append(buf, line)
				size += len(buf)
				continue
			}

			custumedLine := false
			if len(buf) == 0 {
				buf = append(buf, line)
				size += len(buf)
				custumedLine = true
			}

			err = LogAppend(ctx, id, buf...)
			if err != nil {
				errCh <- err
				return
			}

			buf = buf[0:0]
			size = 0
			if !custumedLine {
				buf = append(buf, line)
				size = len(buf)
			}
		}
		if len(buf) > 0 {
			err = LogAppend(ctx, id, buf...)
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
