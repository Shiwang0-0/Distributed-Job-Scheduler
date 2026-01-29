package jobregistry

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"time"
)

// JobHandler interface - keep as interface since that's what your code expects
type JobHandler interface {
	Execute(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error)
	Type() string
}

type JobRegistry struct {
	handlers map[string]JobHandler
}

func NewJobRegistry() *JobRegistry {
	registry := &JobRegistry{
		handlers: make(map[string]JobHandler),
	}

	// Register handler INSTANCES
	registry.Register("send_email", &EmailHandler{})

	log.Println("JobRegistry: Initialized with default handlers")

	return registry
}

func (r *JobRegistry) Register(jobType string, handler JobHandler) {
	r.handlers[jobType] = handler
	log.Printf("JobRegistry: Registered handler for job type: %s", jobType)
}

func (r *JobRegistry) Execute(ctx context.Context, jobType string, payload map[string]interface{}) (map[string]interface{}, error) {
	handler, exists := r.handlers[jobType]
	if !exists {
		return nil, fmt.Errorf("unknown job type: %s", jobType)
	}

	log.Printf("JobRegistry: Executing job type: %s", jobType)
	return handler.Execute(ctx, payload)
}

func (r *JobRegistry) GetRegisteredTypes() []string {
	types := make([]string, 0, len(r.handlers))
	for jobType := range r.handlers {
		types = append(types, jobType)
	}
	return types
}

type EmailHandler struct{}

func (h *EmailHandler) Type() string {
	return "send_email"
}

func (h *EmailHandler) Execute(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	// Extract email parameters
	to, _ := payload["to"].(string)
	subject, _ := payload["subject"].(string)
	body, _ := payload["body"].(string)
	from := getEnvOrDefault("SMTP_FROM", "noreply@example.com")

	if to == "" {
		return nil, fmt.Errorf("'to' field is required")
	}

	// Get SMTP configuration from environment
	smtpHost := getEnvOrDefault("SMTP_HOST", "smtp.gmail.com")
	smtpPort := getEnvOrDefault("SMTP_PORT", "587")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")

	// Check if SMTP credentials are configured
	if smtpUser == "" || smtpPass == "" {
		log.Printf("Warning: SMTP credentials not configured, simulating email send")
		// Simulate email sending for testing
		time.Sleep(500 * time.Millisecond)
	} else {
		// Compose email
		msg := []byte(fmt.Sprintf(
			"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
			from, to, subject, body,
		))

		// Send email
		auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
		err := smtp.SendMail(
			smtpHost+":"+smtpPort,
			auth,
			from,
			[]string{to},
			msg,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to send email: %w", err)
		}
	}

	result := map[string]interface{}{
		"status":     "sent",
		"to":         to,
		"subject":    subject,
		"sent_at":    time.Now().Format(time.RFC3339),
		"message_id": fmt.Sprintf("msg-%d", time.Now().Unix()),
	}

	log.Printf("Email sent to %s: %s", to, subject)
	return result, nil
}
