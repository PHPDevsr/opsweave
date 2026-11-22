package notificationaction

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

func emailMessage(p Profile, subject, message string) string {
	var out strings.Builder
	out.WriteString("From: <" + p.From + ">\r\nTo: <" + strings.Join(p.Recipients, ">, <") + ">\r\nSubject: " + mime.BEncoding.Encode("UTF-8", subject) + "\r\nDate: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(message))
	for len(encoded) > 76 {
		out.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	out.WriteString(encoded + "\r\n")
	return out.String()
}

func sendEmail(ctx context.Context, wire net.Conn, p resolved, config *tls.Config, subject, message string, deadline time.Time) error {
	data := emailMessage(p.profile, subject, message)
	if len(data) > 32<<10 {
		return errAction
	}
	if p.profile.TLSMode == "implicit_tls" {
		conn := tls.Client(wire, config)
		if err := conn.HandshakeContext(ctx); err != nil {
			return err
		}
		wire = conn
	}
	client, err := smtp.NewClient(wire, p.url.Hostname())
	if err != nil {
		return err
	}
	if p.profile.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errAction
		}
		if err := client.StartTLS(config); err != nil {
			return err
		}
	}
	if state, ok := client.TLSConnectionState(); !ok || !state.HandshakeComplete || state.Version < tls.VersionTLS12 {
		return errAction
	}
	if err := wire.SetDeadline(deadline); err != nil {
		return err
	}
	ok, methods := client.Extension("AUTH")
	plain := false
	for _, method := range strings.Fields(methods) {
		plain = plain || method == "PLAIN"
	}
	if !ok || !plain {
		return errAction
	}
	if err := client.Auth(smtp.PlainAuth("", p.username, p.password, p.url.Hostname())); err != nil {
		return err
	}
	if err := client.Mail(p.profile.From); err != nil {
		return err
	}
	for _, to := range p.profile.Recipients {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(writer, data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
