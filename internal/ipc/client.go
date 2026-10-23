package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
)

type Client struct {
	Network string
	Address string
}

func (c Client) Call(
	ctx context.Context,
	method string,
	params interface{},
) (interface{}, error) {
	var d net.Dialer

	conn, err := d.DialContext(ctx, c.Network, c.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	request := Request{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  method,
		Params:  params,
	}

	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return nil, fmt.Errorf("daemon closed connection")
	}

	var response Response
	if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf(
			"daemon error %d: %s",
			response.Error.Code,
			response.Error.Message,
		)
	}

	return response.Result, nil
}
