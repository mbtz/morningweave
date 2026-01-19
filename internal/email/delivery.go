package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"morningweave/internal/config"
	"morningweave/internal/secrets"
)

// Message is the payload sent to an email provider.
type Message struct {
	From    string
	To      []string
	Subject string
	HTML    string
}

// Sender sends email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
	Provider() string
}

// NewSenderFromConfig returns a provider sender based on config and secrets.
func NewSenderFromConfig(cfg config.EmailConfig, resolver secrets.Resolver) (Sender, []string, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	warnings := []string{}
	if provider == "" {
		return nil, warnings, errors.New("email provider is required")
	}

	switch provider {
	case "resend":
		ref := strings.TrimSpace(cfg.Resend.APIKeyRef)
		if ref == "" {
			return nil, warnings, errors.New("email.resend.api_key_ref is required")
		}
		if warn := warnPlaintext(ref); warn != "" {
			warnings = append(warnings, warn)
		}
		apiKey, err := resolver.Resolve(ref)
		if err != nil {
			return nil, warnings, fmt.Errorf("resolve resend api key: %w", err)
		}
		return NewResendSender(apiKey, nil), warnings, nil
	case "smtp":
		host := strings.TrimSpace(cfg.SMTP.Host)
		if host == "" {
			return nil, warnings, errors.New("email.smtp.host is required")
		}
		port := cfg.SMTP.Port
		if port == 0 {
			port = 587
		}
		username := strings.TrimSpace(cfg.SMTP.Username)
		passwordRef := strings.TrimSpace(cfg.SMTP.PasswordRef)
		if passwordRef == "" {
			return nil, warnings, errors.New("email.smtp.password_ref is required")
		}
		if warn := warnPlaintext(passwordRef); warn != "" {
			warnings = append(warnings, warn)
		}
		password, err := resolver.Resolve(passwordRef)
		if err != nil {
			return nil, warnings, fmt.Errorf("resolve smtp password: %w", err)
		}
		return NewSMTPSender(host, port, username, password), warnings, nil
	default:
		return nil, warnings, fmt.Errorf("unsupported email provider: %s", provider)
	}
}

func warnPlaintext(ref string) string {
	provider, _, ok := secrets.ParseRef(ref)
	if !ok {
		return ""
	}
	switch provider {
	case "secrets", "secret", "plain", "literal", "raw":
		return "plaintext secrets in use; prefer keychain/1Password for production"
	default:
		return ""
	}
}

// ResendSender delivers email via the Resend API.
type ResendSender struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewResendSender constructs a Resend sender.
func NewResendSender(apiKey string, client *http.Client) *ResendSender {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &ResendSender{
		apiKey:  apiKey,
		baseURL: "https://api.resend.com",
		client:  client,
	}
}

// Provider returns the provider name.
func (s *ResendSender) Provider() string {
	return "resend"
}

// Send delivers the message via Resend.
func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	if strings.TrimSpace(s.apiKey) == "" {
		return errors.New("resend api key is empty")
	}
	payload := map[string]any{
		"from":    msg.From,
		"to":      msg.To,
		"subject": msg.Subject,
		"html":    msg.HTML,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend api error: %s", resp.Status)
	}
	return nil
}

// SMTPSender delivers email via SMTP.
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
}

// NewSMTPSender constructs an SMTP sender.
func NewSMTPSender(host string, port int, username string, password string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
	}
}

// Provider returns the provider name.
func (s *SMTPSender) Provider() string {
	return "smtp"
}

// Send delivers the message via SMTP.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	_ = ctx
	if strings.TrimSpace(s.host) == "" {
		return errors.New("smtp host is empty")
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))

	headers := []string{
		"From: " + msg.From,
		"To: " + strings.Join(msg.To, ", "),
		"Subject: " + msg.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}
	body := strings.Join(headers, "\r\n") + "\r\n\r\n" + msg.HTML

	envelopeFrom := msg.From
	if parsed, err := mail.ParseAddress(msg.From); err == nil {
		envelopeFrom = parsed.Address
	}
	recipients := make([]string, 0, len(msg.To))
	for _, raw := range msg.To {
		candidate := strings.TrimSpace(raw)
		if candidate == "" {
			continue
		}
		if parsed, err := mail.ParseAddress(candidate); err == nil {
			recipients = append(recipients, parsed.Address)
		} else {
			recipients = append(recipients, candidate)
		}
	}

	var auth smtp.Auth
	if strings.TrimSpace(s.username) != "" && strings.TrimSpace(s.password) != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}
	return smtp.SendMail(addr, auth, envelopeFrom, recipients, []byte(body))
}
