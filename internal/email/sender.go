package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// Send sends an email via SMTP with optional TLS.
func Send(host string, port int, user, password, from string, to []string, subject, body string) error {
	if host == "" || len(to) == 0 {
		return fmt.Errorf("email: host and recipients required")
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	msg := []byte(
		"To: " + strings.Join(to, ", ") + "\r\n" +
			"From: " + from + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n" +
			"\r\n" +
			body + "\r\n",
	)

	var err error
	if port == 465 {
		// TLS from start
		tlsConfig := &tls.Config{ServerName: host}
		conn, errConn := tls.Dial("tcp", addr, tlsConfig)
		if errConn != nil {
			return fmt.Errorf("email tls dial: %w", errConn)
		}
		defer conn.Close()
		client, errConn := smtp.NewClient(conn, host)
		if errConn != nil {
			return fmt.Errorf("email smtp client: %w", errConn)
		}
		defer client.Close()
		if user != "" && password != "" {
			auth := smtp.PlainAuth("", user, password, host)
			if err = client.Auth(auth); err != nil {
				return fmt.Errorf("email auth: %w", err)
			}
		}
		if err = client.Mail(from); err != nil {
			return err
		}
		for _, r := range to {
			if err = client.Rcpt(r); err != nil {
				return err
			}
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write(msg)
		w.Close()
		return err
	}

	// STARTTLS or no auth
	if user != "" && password != "" {
		auth := smtp.PlainAuth("", user, password, host)
		err = smtp.SendMail(addr, auth, from, to, msg)
	} else {
		err = smtp.SendMail(addr, nil, from, to, msg)
	}
	if err != nil {
		return fmt.Errorf("email send: %w", err)
	}
	return nil
}
