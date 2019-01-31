package webasis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/webasis/wrpc"
)

func ShareFile(ctx context.Context, name string, r io.Reader) error {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	resp, err := Call(ctx, "notify", name, string(data))
	if err != nil {
		return err
	}

	if resp.Status != wrpc.StatusOK {
		return errors.New(fmt.Sprintf("error: wrpc response expect ok but got %s", string(resp.Status)))
	}
	return nil
}
