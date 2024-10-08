// Copyright 2024 Canonical.

package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
)

const (
	PermissionCheckRequiredErrorCode = "permission check required"
)

// An Error represents an error sent in an RPC response.
type Error struct {
	Message string
	Code    string
	Info    map[string]interface{}
}

// Error implements the error interface.
func (e *Error) Error() string {
	// format the error in the same way as juju.
	if e.Code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Code)
	}
	return e.Message
}

// ErrorCode returns the error's code.
func (e *Error) ErrorCode() string {
	return e.Code
}

// NewClient takes a websocket connection and returns an RPC client.
// Note that a go routine is started that reads on the websocket.
func NewClient(conn *websocket.Conn) *Client {
	cl := &Client{
		conn:   conn,
		closed: make(chan struct{}),
		msgs:   make(map[uint64]inflight),
	}
	go cl.recv()
	return cl
}

type inflight struct {
	ch  chan struct{}
	msg **message
}

// A Client is a juju RPC client.
type Client struct {
	conn   *websocket.Conn
	closed chan struct{}

	mu    sync.Mutex
	reqID uint64
	msgs  map[uint64]inflight

	closing bool
	broken  bool
	err     error
}

func (c *Client) recv() {
	for {
		msg := new(message)
		if err := c.conn.ReadJSON(msg); err != nil {
			c.handleError(err)
			break
		}
		if msg.RequestID == 0 {
			// Use a 0 request ID to indicate that the message
			// received was not a valid RPC message.
			c.handleError(errors.E("received invalid RPC message"))
			break
		}
		if msg.isRequest() {
			c.handleRequest(msg)
			continue
		}
		c.handleResponse(msg)
	}
}

func (c *Client) handleError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closing {
		// We haven't sent a close message yet, so try to send one.
		cm := websocket.FormatCloseMessage(websocket.CloseProtocolError, err.Error())
		err := c.conn.WriteControl(websocket.CloseMessage, cm, time.Time{})
		if err != nil {
			zapctx.Error(context.Background(), "failed to write socket closure message", zap.Error(err))
		}
	}
	c.err = err
	c.conn.Close()
	close(c.closed)
}

// handleRequest handles any incoming request messages. Although the RPC
// protocol is defined such that it is bidirectional and requests may be
// sent from the server the juju API never does so. The request is
// therefore handled by sending a canned error response.
func (c *Client) handleRequest(msg *message) {
	var sb strings.Builder
	sb.WriteString(msg.Type)
	if msg.Version > 0 {
		fmt.Fprintf(&sb, "(%d)", msg.Version)
	}
	fmt.Fprintf(&sb, ".%s not implemented", msg.Request)

	resp := message{
		RequestID: msg.RequestID,
		Error:     sb.String(),
		ErrorCode: jujuparams.CodeNotImplemented,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Note we're ignoring any write error here as any subsequent write
	// will also error and that will be able to process the error more
	// appropriately.
	err := c.conn.WriteJSON(resp)
	if err != nil {
		zapctx.Error(context.Background(), "failed to write JSON resp", zap.Error(err))
	}
}

func (c *Client) handleResponse(msg *message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	waiter, ok := c.msgs[msg.RequestID]
	if ok {
		*waiter.msg = msg
		close(waiter.ch)
	}
}

// Call makes an RPC call to the server. Call sends the request message to
// the server and waits for the response to be returned or the context to
// be canceled.
func (c *Client) Call(ctx context.Context, facade string, version int, id, method string, args, resp interface{}) error {
	const op = errors.Op("rpc.Client.Call")

	var argsb []byte
	if args != nil {
		var err error
		argsb, err = json.Marshal(args)
		if err != nil {
			return errors.E(op, err)
		}
	}
	req := &message{
		Type:    facade,
		Version: version,
		ID:      id,
		Request: method,
		Params:  json.RawMessage(argsb),
	}
	c.mu.Lock()
	// Please note that an unlock is deferred here, but this function
	// does not always hold the lock for its entire duration. care must
	// be taken that when reaching this point in the defer stack the
	// function holds the lock.
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.reqID++
	// For anyone else as curious as me, one would need to send over
	// half a million messages per millisecond for a millennium before
	// this will wrap. So probably don't worry about checking for it.
	req.RequestID = c.reqID
	if err := c.conn.WriteJSON(req); err != nil {
		c.broken = true
		return errors.E(op, err)
	}
	ch := make(chan struct{})
	//nolint:staticcheck // Not sure why Martin made this a **. Ignore for now.
	respMsg := new(*message)
	c.msgs[req.RequestID] = inflight{
		ch:  ch,
		msg: respMsg,
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.msgs, req.RequestID)
	}()

	select {
	case <-ch:
		permissionsRequired, err := checkPermissionsRequired(ctx, *respMsg)
		if err != nil {
			return err
		}
		if permissionsRequired != nil {
			return &Error{
				Code: PermissionCheckRequiredErrorCode,
				Info: permissionsRequired,
			}
		}
		if (*respMsg).Error != "" {
			return &Error{
				Message: (*respMsg).Error,
				Code:    (*respMsg).ErrorCode,
				Info:    (*respMsg).ErrorInfo,
			}
		}
		if resp != nil {
			if err := json.Unmarshal([]byte((*respMsg).Response), &resp); err != nil {
				return errors.E(op, err)
			}
		}
		return nil
	case <-c.closed:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.err
	case <-ctx.Done():
		return errors.E(op, ctx.Err())
	}
}

// Close initiates closing the client connection by sending a close message
// to the server. This will normally allow any outstanding requests to
// complete before gracefully shutting down. If for any reason sending the
// close message fails Close will abruptly close the undelying connection.
func (c *Client) Close() error {
	const op = errors.Op("rpc.Client.Close")

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.err != nil {
		return c.err
	}
	// Start the process of stopping the RPC connection. This will
	// ultimately cause the background receiver go routine to finish
	// when it processes the stop message sent by the server in reply.
	// This process gives any outstanding calls a chance to finish.
	c.closing = true
	cm := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	if err := c.conn.WriteControl(websocket.CloseMessage, cm, time.Time{}); err != nil {
		c.err = errors.E(op, "error closing connection", err)
		// If sending the close message failed then tear down the
		// connection. Note that we don't need to clear up any
		// outstanding messages here as the receiver will error and
		// do that.
		c.conn.Close()
	}
	return c.err
}

// IsBroken returns true if client has determined that it is no longer able
// to send messages to the server.
func (c *Client) IsBroken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.broken
}
