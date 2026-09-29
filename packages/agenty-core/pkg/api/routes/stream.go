package routes

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"

	json "github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/masteryyh/agenty-core/pkg/infra/httpapi"
)

const (
	maxStreamCommandBytes = 1 << 20
	streamOutputBuffer    = 256
)

func registerStreamRoutes(v1 *gin.RouterGroup, api *API) {
	v1.POST("/stream", api.serveStream)
}

func (api *API) serveStream(c *gin.Context) {
	if api.stream == nil {
		writeAPIError(c.Writer, &APIError{Status: http.StatusServiceUnavailable, Code: "stream_unavailable", Message: "event stream is unavailable"})
		return
	}
	serveStream(c, api.stream)
}

func serveStream(c *gin.Context, broker *httpapi.StreamBroker) {
	writer := c.Writer
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeAPIError(writer, &APIError{Status: http.StatusInternalServerError, Code: "stream_unsupported", Message: "streaming responses are unavailable"})
		return
	}

	writer.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Agenty-Stream-ID", broker.StreamID())
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	commands := make(chan httpapi.StreamCommand, 8)
	inputErr := make(chan error, 1)
	go readStreamCommands(ctx, c.Request.Body, commands, inputErr)
	output := make(chan httpapi.StreamFrame, streamOutputBuffer)
	writeFrame := func(frame httpapi.StreamFrame) error {
		data, err := json.Marshal(frame)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	subscriptions := make(map[string]*httpapi.StreamSubscription)
	defer func() {
		for _, subscription := range subscriptions {
			subscription.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-inputErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				_ = writeFrame(httpapi.StreamFrame{Type: "error", Code: "invalid_stream", Message: err.Error()})
			}
			return
		case command, open := <-commands:
			if !open {
				return
			}
			if command.Type == "unsubscribe" {
				if subscription := subscriptions[command.Topic]; subscription != nil {
					subscription.Close()
					delete(subscriptions, command.Topic)
				}
				continue
			}
			if command.Type != "subscribe" {
				if err := writeFrame(httpapi.StreamFrame{Type: "error", Topic: command.Topic, Code: "invalid_command", Message: "type must be subscribe or unsubscribe"}); err != nil {
					return
				}
				continue
			}
			if old := subscriptions[command.Topic]; old != nil {
				old.Close()
				delete(subscriptions, command.Topic)
			}
			initial, subscription, err := broker.Subscribe(ctx, command.Topic, command.After)
			if err != nil {
				code := "resync_required"
				var brokerErr *httpapi.BrokerError
				if errors.As(err, &brokerErr) {
					code = brokerErr.Code
				}
				_ = writeFrame(httpapi.StreamFrame{Type: "error", Topic: command.Topic, Code: code, Message: err.Error()})
				continue
			}
			subscriptions[command.Topic] = subscription
			for _, frame := range initial {
				if err := writeFrame(frame); err != nil {
					return
				}
			}
			go forwardSubscription(ctx, command.Topic, subscription, output)
		case frame := <-output:
			if err := writeFrame(frame); err != nil || frame.Type == "disconnect" {
				return
			}
		}
	}
}

func readStreamCommands(ctx context.Context, body io.ReadCloser, commands chan<- httpapi.StreamCommand, failures chan<- error) {
	defer close(commands)
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), maxStreamCommandBytes)
	for scanner.Scan() {
		var command httpapi.StreamCommand
		if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
			failures <- err
			return
		}
		select {
		case commands <- command:
		case <-ctx.Done():
			return
		}
	}
	failures <- scanner.Err()
}

func forwardSubscription(ctx context.Context, topic string, subscription *httpapi.StreamSubscription, output chan<- httpapi.StreamFrame) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-subscription.Dropped:
			select {
			case output <- httpapi.StreamFrame{Type: "disconnect", Topic: topic, Code: "slow_consumer", Message: "event queue exceeded; reconnect with the last applied cursor"}:
			case <-ctx.Done():
			}
			return
		case frame := <-subscription.Frames:
			select {
			case output <- frame:
			case <-ctx.Done():
				return
			}
		}
	}
}
