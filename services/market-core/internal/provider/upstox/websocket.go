package upstox

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type GorillaDialer struct {
	HandshakeTimeout time.Duration
}

func (d GorillaDialer) Dial(ctx context.Context, rawURI string) (BinaryConnection, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" {
		return nil, errors.New("invalid websocket URI")
	}

	timeout := d.HandshakeTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: timeout,
	}

	connection, response, err := dialer.DialContext(ctx, rawURI, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return &gorillaConnection{connection: connection}, nil
}

type gorillaConnection struct {
	connection *websocket.Conn
}

func (c *gorillaConnection) WriteBinary(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.connection.SetWriteDeadline(deadline); err != nil {
			return err
		}
	} else if err := c.connection.SetWriteDeadline(time.Time{}); err != nil {
		return err
	}

	return c.connection.WriteMessage(websocket.BinaryMessage, payload)
}

func (c *gorillaConnection) ReadBinary(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cancelRead := make(chan struct{})
	if ctx.Done() != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = c.connection.SetReadDeadline(time.Now())
			case <-cancelRead:
			}
		}()
		defer close(cancelRead)
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.connection.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
	} else if err := c.connection.SetReadDeadline(time.Time{}); err != nil {
		return nil, err
	}

	for {
		messageType, payload, err := c.connection.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if messageType == websocket.BinaryMessage {
			return payload, nil
		}
	}
}

func (c *gorillaConnection) Close() error {
	return c.connection.Close()
}
