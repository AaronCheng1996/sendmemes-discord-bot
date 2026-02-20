package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/goccy/go-json"
)

const (
	// Base settings
	host     = "app"
	attempts = 20

	// Attempts connection
	httpURL        = "http://" + host + ":8080"
	healthPath     = httpURL + "/healthz"
	requestTimeout = 5 * time.Second

	// HTTP REST
	basePathV1 = httpURL + "/v1"
)

var errHealthCheck = fmt.Errorf("url %s is not available", healthPath)

func doWebRequestWithTimeout(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

func getHealthCheck(url string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)

	defer cancel()

	resp, err := doWebRequestWithTimeout(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return -1, err
	}

	defer resp.Body.Close()

	return resp.StatusCode, nil
}

func healthCheck(attempts int) error {
	for attempts > 0 {
		statusCode, err := getHealthCheck(healthPath)
		if err != nil {
			return err
		}

		if statusCode == http.StatusOK {
			return nil
		}

		log.Printf("Integration tests: url %s is not available, attempts left: %d", healthPath, attempts)

		time.Sleep(time.Second)

		attempts--
	}

	return errHealthCheck
}

func TestMain(m *testing.M) {
	err := healthCheck(attempts)
	if err != nil {
		log.Fatalf("Integration tests: httpURL %s is not available: %s", httpURL, err)
	}

	log.Printf("Integration tests: httpURL %s is available", httpURL)

	code := m.Run()
	os.Exit(code)
}

// HTTP POST: /v1/translation/do-translate.
func TestHTTPDoTranslateV1(t *testing.T) {
	tests := []struct {
		description string
		body        string
		expected    int
	}{
		{
			description: "DoTranslate Success",
			body: `{
				"destination": "en",
				"original": "текст для перевода",
				"source": "auto"
			}`,
			expected: http.StatusOK,
		},
		{
			description: "DoTranslate Success",
			body: `{
				"destination": "en",
				"original": "Текст для перевода",
				"source": "ru"
			}`,
			expected: http.StatusOK,
		},
		{
			description: "DoTranslate Fail",
			body: `{
				"destination": "en",
				"original": "текст для перевода"
			}`,
			expected: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			url := basePathV1 + "/translation/do-translate"
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)

			defer cancel()

			resp, err := doWebRequestWithTimeout(ctx, http.MethodPost, url, bytes.NewBuffer([]byte(tt.body)))
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, resp.StatusCode)
			}
		})
	}
}

// HTTP GET: /v1/translation/history.
func TestHTTPHistoryV1(t *testing.T) {
	url := basePathV1 + "/translation/history"
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)

	defer cancel()

	resp, err := doWebRequestWithTimeout(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var body struct {
		History []struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
			Original    string `json:"original"`
			Translation string `json:"translation"`
		} `json:"history"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	// History may be empty before any translate calls; allow empty
	_ = body.History
}
