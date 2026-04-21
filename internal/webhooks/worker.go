package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"tockr/internal/db/sqlite"
)

type Worker struct {
	store      *sqlite.Store
	client     *http.Client
	log        *slog.Logger
	maxRetries int
}

func NewWorker(store *sqlite.Store, log *slog.Logger, maxRetries int) *Worker {
	return &Worker{
		store:      store,
		client:     &http.Client{Timeout: 5 * time.Second},
		log:        log,
		maxRetries: maxRetries,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	rows, err := w.store.PendingWebhookDeliveries(ctx, 10)
	if err != nil {
		w.log.Warn("load webhook deliveries failed", "err", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		w.deliver(ctx, rows)
	}
}

func (w *Worker) deliver(ctx context.Context, rows *sql.Rows) {
	var id int64
	var url, secret, event, payload string
	var attempts int
	if err := rows.Scan(&id, &url, &secret, &event, &payload, &attempts); err != nil {
		w.log.Warn("scan webhook delivery failed", "err", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		_ = w.store.MarkWebhookDelivery(ctx, id, "failed", attempts+1, err.Error(), time.Now().UTC().Add(time.Hour))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tockr-Event", event)
	req.Header.Set("X-Tockr-Signature", sign(secret, payload))
	resp, err := w.client.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_ = w.store.MarkWebhookDelivery(ctx, id, "sent", attempts+1, "", time.Now().UTC())
		return
	}
	message := "non-2xx response"
	if err != nil {
		message = err.Error()
	}
	attempts++
	if attempts >= w.maxRetries {
		_ = w.store.MarkWebhookDelivery(ctx, id, "failed", attempts, message, time.Now().UTC())
		return
	}
	next := time.Now().UTC().Add(time.Duration(attempts*attempts) * time.Minute)
	_ = w.store.MarkWebhookDelivery(ctx, id, "pending", attempts, message, next)
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
