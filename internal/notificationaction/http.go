package notificationaction

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

func sendHTTP(ctx context.Context, conn net.Conn, owner *ownedConn, p resolved, message string, deadline time.Time) (resultErr error) {
	payload := map[string]any{"text": message}
	switch p.profile.Action {
	case "notification.telegram":
		payload["chat_id"] = p.profile.ChatID
	case "notification.slack":
		payload["unfurl_links"] = false
		payload["unfurl_media"] = false
	case "notification.webhook":
		payload = map[string]any{"message": message}
	}
	data, err := json.Marshal(payload)
	if err != nil || len(data) > 32<<10 {
		return errAction
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url.String(), strings.NewReader(string(data)))
	if err != nil {
		return errAction
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/json")
	if p.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+p.bearer)
	}
	if conn.SetDeadline(deadline) != nil {
		return errAction
	}
	if err := req.Write(conn); err != nil {
		return err
	}
	if conn.SetReadDeadline(earlier(deadline, time.Now().Add(5*time.Second))) != nil {
		return errAction
	}
	limited := &io.LimitedReader{R: conn, N: 16 << 10}
	response, err := http.ReadResponse(bufio.NewReader(limited), req)
	if err != nil {
		return err
	}
	defer func() {
		// Close the socket before body cleanup can try to drain a rejected reply.
		closeErr := owner.Close()
		resultErr = errors.Join(resultErr, closeErr, response.Body.Close())
	}()
	limited.N = 1<<63 - 1
	if conn.SetReadDeadline(deadline) != nil {
		return errAction
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<10)+1))
	if err != nil || len(body) > 8<<10 || !utf8.Valid(body) || response.Header.Get("Content-Encoding") != "" {
		return errAction
	}
	switch p.profile.Action {
	case "notification.telegram":
		var reply struct {
			OK *bool `json:"ok"`
		}
		if response.StatusCode != 200 || json.Unmarshal(body, &reply) != nil || reply.OK == nil || !*reply.OK {
			return errAction
		}
	case "notification.slack":
		if response.StatusCode != 200 || strings.TrimSpace(string(body)) != "ok" {
			return errAction
		}
	case "notification.webhook":
		if response.StatusCode < 200 || response.StatusCode > 299 {
			return errAction
		}
	default:
		return errAction
	}
	return nil
}
