//go:build e2e_docker

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/metadata-inventory/pkg/db"
	"github.com/stretchr/testify/require"
)

func TestE2E_DockerFullStack(t *testing.T) {
	baseURL := strings.TrimRight(envOr("E2E_BASE_URL", "http://localhost:8080"), "/")
	require.NoError(t, waitForHTTP200(baseURL+"/health", 90*time.Second))
	require.NoError(t, waitForHTTP200(baseURL+"/ready", 90*time.Second))

	testURL := fmt.Sprintf("http://api:8080/health?e2e=%d", time.Now().UnixNano())
	body := bytes.NewBufferString(fmt.Sprintf(`{"url":"%s"}`, testURL))
	postResp, err := http.Post(baseURL+"/v1/metadata", "application/json", body)
	require.NoError(t, err)
	defer postResp.Body.Close()
	require.Contains(t, []int{http.StatusCreated, http.StatusAccepted}, postResp.StatusCode)

	query := url.QueryEscape(testURL)
	var final db.MetadataRecord

	require.Eventually(t, func() bool {
		resp, err := http.Get(fmt.Sprintf("%s/v1/metadata?url=%s", baseURL, query))
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusAccepted {
			return false
		}

		if resp.StatusCode != http.StatusOK {
			return false
		}

		if err := json.NewDecoder(resp.Body).Decode(&final); err != nil {
			return false
		}

		return final.Status == db.StatusReady
	}, 2*time.Minute, 2*time.Second)

	require.Equal(t, testURL, final.URL)
	require.Equal(t, db.StatusReady, final.Status)
	require.NotEmpty(t, final.Headers)

	workerHealth, err := http.Get("http://localhost:9091/health")
	require.NoError(t, err)
	defer workerHealth.Body.Close()
	require.Equal(t, http.StatusOK, workerHealth.StatusCode)
}

func waitForHTTP200(target string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(target)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for HTTP 200: %s", target)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
