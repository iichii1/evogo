package webhook_producer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	producer_interfaces "github.com/evolution-foundation/evolution-go/pkg/events/interfaces"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/metrics"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

type webhookProducer struct {
	url           string
	loggerWrapper *logger_wrapper.LoggerManager
}

func NewWebhookProducer(
	url string,
	loggerWrapper *logger_wrapper.LoggerManager,
) producer_interfaces.Producer {
	return &webhookProducer{
		url:           url,
		loggerWrapper: loggerWrapper,
	}
}

func (p *webhookProducer) Produce(
	queueName string,
	payload []byte,
	webhookUrl string,
	userID string,
) error {
	splitQueue := strings.Split(queueName, ".")

	if len(splitQueue) < 2 {
		return nil
	}

	if p.url != "" {
		go p.sendWebhookWithRetry(p.url, payload, 5, 30*time.Second, userID)
	}
	if webhookUrl != "" {
		go p.sendWebhookWithRetry(webhookUrl, payload, 5, 30*time.Second, userID)
	}

	return nil
}

func (p *webhookProducer) sendWebhookWithRetry(url string, body []byte, maxRetries int, retryInterval time.Duration, userID string) {
	for i := 0; i < maxRetries; i++ {
		err, responseBody, statusCode := p.sendWebhook(url, body, userID)
		if err == nil {
			metrics.WebhookDelivered.Inc()
			p.loggerWrapper.GetLogger(userID).LogInfo("[%s] webhook sent successfully - url: %s, status: %d, response: %s", userID, utils.RedactURL(url), statusCode, string(responseBody))
			return
		}
		metrics.WebhookAttemptFailures.Inc()
		p.loggerWrapper.GetLogger(userID).LogWarn("[%s] webhook failed - url: %s, attempt: %d, error: %v", userID, utils.RedactURL(url), i+1, redactErrorURL(err))

		time.Sleep(retryInterval)
	}
	metrics.WebhookGivenUp.Inc()
	p.loggerWrapper.GetLogger(userID).LogError("[%s] webhook failed after maximum retries - url: %s", userID, utils.RedactURL(url))
}

func (p *webhookProducer) sendWebhook(url string, body []byte, userID string) (error, []byte, int) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err, nil, 0
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err, nil, 0
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("erro ao ler resposta: %v", err), nil, 0
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("received non-2xx response: " + resp.Status), responseBody, resp.StatusCode
	}

	return nil, responseBody, resp.StatusCode
}

// redactErrorURL tira a URL do erro. Os erros do cliente HTTP repetem a URL
// inteira, com a query, e ela pode trazer um token.
func redactErrorURL(err error) error {
	var urlErr *neturl.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// CreateGlobalQueues não faz nada para webhook producer
func (p *webhookProducer) CreateGlobalQueues() error {
	return nil
}
