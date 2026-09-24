package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
)

func remoteAPI(args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return fmt.Errorf("usage: eng api GET|POST /api/v1/path [body.json]")
	}
	method, p := args[0], args[1]
	if (method == http.MethodPost && len(args) != 3) || (method == http.MethodGet && len(args) != 2) {
		return fmt.Errorf("POST needs a body file; GET must not have one")
	}
	var data []byte
	if len(args) == 3 {
		f, err := os.Open(args[2])
		if err != nil {
			return err
		}
		defer f.Close()
		data, err = io.ReadAll(io.LimitReader(f, controlclient.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(data) > controlclient.MaxRequest {
			return fmt.Errorf("request body exceeds 1 MiB")
		}
	}
	c, err := controlclient.FromEnvironment(os.Getenv)
	if err != nil {
		return err
	}
	defer c.Close()
	response, err := c.Raw(context.Background(), method, p, data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(response))
	return err
}
