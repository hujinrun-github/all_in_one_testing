package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRunTimeoutMs = 3000
	maxRunRequests      = 10000
	maxRunConcurrency   = 256
)

type createRunRequest struct {
	ScenarioID    string         `json:"scenarioId,omitempty"`
	ScenarioName  string         `json:"-"`
	Name          string         `json:"name"`
	Method        string         `json:"method"`
	URL           string         `json:"url"`
	TotalRequests int            `json:"totalRequests"`
	Concurrency   int            `json:"concurrency"`
	TimeoutMs     int            `json:"timeoutMs"`
	Headers       []runHeader    `json:"headers,omitempty"`
	BodyVariants  []bodyVariant  `json:"bodyVariants,omitempty"`
	QueryVariants []queryVariant `json:"queryVariants,omitempty"`
}

type bodyVariant struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
	Body   string `json:"body"`
}

type queryVariant struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	QueryParams string `json:"queryParams"`
}

type runHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type createRunResponse struct {
	ID               string  `json:"id"`
	ScenarioID       string  `json:"scenarioId,omitempty"`
	ScenarioName     string  `json:"scenarioName,omitempty"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	Method           string  `json:"method"`
	URL              string  `json:"url"`
	TotalRequests    int     `json:"totalRequests"`
	SuccessRequests  int     `json:"successRequests"`
	FailedRequests   int     `json:"failedRequests"`
	DurationMs       float64 `json:"durationMs"`
	QPS              float64 `json:"qps"`
	AverageLatencyMs float64 `json:"averageLatencyMs"`
	P95LatencyMs     float64 `json:"p95LatencyMs"`
}

type runSample struct {
	latency time.Duration
	success bool
}

func handleCreateRun(store *scenarioStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input createRunRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must be valid JSON"})
			return
		}

		normalized, status, err := resolveCreateRunRequest(store, input)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		result := executeHTTPRun(r.Context(), normalized)
		writeJSON(w, http.StatusOK, result)
	}
}

func resolveCreateRunRequest(store *scenarioStore, input createRunRequest) (createRunRequest, int, error) {
	scenarioID := strings.TrimSpace(input.ScenarioID)
	if scenarioID == "" {
		normalized, err := validateCreateRunRequest(input)
		return normalized, http.StatusBadRequest, err
	}

	scenario, found, err := store.get(scenarioID)
	if err != nil {
		return input, http.StatusInternalServerError, err
	}
	if !found {
		return input, http.StatusNotFound, fmt.Errorf("scenario not found")
	}

	input = applyScenarioToRunRequest(input, scenario)
	normalized, err := validateCreateRunRequest(input)
	return normalized, http.StatusBadRequest, err
}

func applyScenarioToRunRequest(input createRunRequest, scenario scenarioRecord) createRunRequest {
	input.ScenarioID = scenario.ID
	input.ScenarioName = scenario.Name
	if strings.TrimSpace(input.Name) == "" {
		input.Name = scenario.Name
	}
	input.Method = scenario.Method
	input.URL = scenarioRunURL(scenario)
	input.Headers = scenarioHeadersToRunHeaders(scenario.Headers)
	input.QueryVariants = scenarioQueryVariantsToRunVariants(scenario.QueryVariants)
	input.BodyVariants = scenarioBodyVariantsToRunVariants(scenario.BodyVariants)
	if input.TimeoutMs <= 0 {
		input.TimeoutMs = parseScenarioInt(scenario.TimeoutMs)
	}
	return input
}

func scenarioRunURL(scenario scenarioRecord) string {
	baseURL := strings.TrimRight(strings.TrimSpace(scenario.BaseURL), "/")
	path := "/" + strings.TrimLeft(strings.TrimSpace(scenario.Path), "/")
	return baseURL + path
}

func scenarioHeadersToRunHeaders(headers []scenarioHeader) []runHeader {
	runHeaders := make([]runHeader, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" {
			continue
		}
		runHeaders = append(runHeaders, runHeader{Key: key, Value: strings.TrimSpace(header.Value)})
	}
	return runHeaders
}

func scenarioQueryVariantsToRunVariants(variants []scenarioQueryVariant) []queryVariant {
	runVariants := make([]queryVariant, 0, len(variants))
	for _, variant := range variants {
		queryParams := strings.TrimSpace(variant.QueryParams)
		if queryParams == "" {
			continue
		}
		runVariants = append(runVariants, queryVariant{
			Name:        strings.TrimSpace(variant.Name),
			Weight:      parseScenarioInt(variant.Weight),
			QueryParams: queryParams,
		})
	}
	return runVariants
}

func scenarioBodyVariantsToRunVariants(variants []scenarioBodyVariant) []bodyVariant {
	runVariants := make([]bodyVariant, 0, len(variants))
	for _, variant := range variants {
		body := strings.TrimSpace(variant.Body)
		if body == "" {
			continue
		}
		runVariants = append(runVariants, bodyVariant{
			Name:   strings.TrimSpace(variant.Name),
			Weight: parseScenarioInt(variant.Weight),
			Body:   body,
		})
	}
	return runVariants
}

func parseScenarioInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func validateCreateRunRequest(input createRunRequest) (createRunRequest, error) {
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		return input, fmt.Errorf("url is required")
	}
	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return input, fmt.Errorf("url must be an absolute http or https URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return input, fmt.Errorf("url must use http or https")
	}

	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	if input.Method != http.MethodGet && input.Method != http.MethodPost {
		return input, fmt.Errorf("method must be GET or POST")
	}
	if input.TotalRequests <= 0 || input.TotalRequests > maxRunRequests {
		return input, fmt.Errorf("totalRequests must be between 1 and %d", maxRunRequests)
	}
	if input.Concurrency <= 0 || input.Concurrency > maxRunConcurrency {
		return input, fmt.Errorf("concurrency must be between 1 and %d", maxRunConcurrency)
	}
	if input.Concurrency > input.TotalRequests {
		input.Concurrency = input.TotalRequests
	}
	if input.TimeoutMs <= 0 {
		input.TimeoutMs = defaultRunTimeoutMs
	}
	if input.Name == "" {
		input.Name = "ad-hoc-http-run"
	}
	input.Headers = normalizeRunHeaders(input.Headers)
	if err := validateBodyVariants(input.BodyVariants); err != nil {
		return input, err
	}
	if err := validateQueryVariants(input.QueryVariants); err != nil {
		return input, err
	}
	return input, nil
}

func normalizeRunHeaders(headers []runHeader) []runHeader {
	runHeaders := make([]runHeader, 0, len(headers))
	for _, header := range headers {
		key := strings.TrimSpace(header.Key)
		if key == "" {
			continue
		}
		runHeaders = append(runHeaders, runHeader{Key: key, Value: strings.TrimSpace(header.Value)})
	}
	return runHeaders
}

func executeHTTPRun(ctx context.Context, input createRunRequest) createRunResponse {
	client := &http.Client{Timeout: time.Duration(input.TimeoutMs) * time.Millisecond}
	jobs := make(chan struct{})
	samples := make(chan runSample, input.TotalRequests)
	startedAt := time.Now()

	var workers sync.WaitGroup
	for worker := 0; worker < input.Concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				samples <- executeHTTPSample(ctx, client, input)
			}
		}()
	}

	for requestIndex := 0; requestIndex < input.TotalRequests; requestIndex++ {
		jobs <- struct{}{}
	}
	close(jobs)
	workers.Wait()
	close(samples)

	result := summarizeRun(input, samples, time.Since(startedAt))
	return result
}

func executeHTTPSample(ctx context.Context, client *http.Client, input createRunRequest) runSample {
	startedAt := time.Now()
	targetURL := input.URL
	if len(input.QueryVariants) > 0 {
		targetURL = appendQueryParams(targetURL, selectWeightedQuery(input.QueryVariants))
	}
	var requestBody io.Reader
	if len(input.BodyVariants) > 0 {
		requestBody = strings.NewReader(selectWeightedBody(input.BodyVariants))
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, targetURL, requestBody)
	if err != nil {
		return runSample{latency: time.Since(startedAt), success: false}
	}
	for _, header := range input.Headers {
		request.Header.Set(header.Key, header.Value)
	}
	response, err := client.Do(request)
	if err != nil {
		return runSample{latency: time.Since(startedAt), success: false}
	}
	defer response.Body.Close()

	success := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusBadRequest
	return runSample{latency: time.Since(startedAt), success: success}
}

func summarizeRun(input createRunRequest, samples <-chan runSample, duration time.Duration) createRunResponse {
	latencies := make([]float64, 0, input.TotalRequests)
	successRequests := 0
	failedRequests := 0
	totalLatencyMs := 0.0

	for sample := range samples {
		latencyMs := float64(sample.latency.Microseconds()) / 1000
		latencies = append(latencies, latencyMs)
		totalLatencyMs += latencyMs
		if sample.success {
			successRequests++
		} else {
			failedRequests++
		}
	}

	durationMs := float64(duration.Microseconds()) / 1000
	durationSeconds := math.Max(duration.Seconds(), 0.001)
	sort.Float64s(latencies)

	return createRunResponse{
		ID:               fmt.Sprintf("run-%d", time.Now().UnixNano()),
		ScenarioID:       input.ScenarioID,
		ScenarioName:     input.ScenarioName,
		Name:             input.Name,
		Status:           "finished",
		Method:           input.Method,
		URL:              input.URL,
		TotalRequests:    input.TotalRequests,
		SuccessRequests:  successRequests,
		FailedRequests:   failedRequests,
		DurationMs:       durationMs,
		QPS:              float64(input.TotalRequests) / durationSeconds,
		AverageLatencyMs: averageLatency(totalLatencyMs, len(latencies)),
		P95LatencyMs:     percentile(latencies, 0.95),
	}
}

func averageLatency(total float64, count int) float64 {
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func validateBodyVariants(variants []bodyVariant) error {
	if len(variants) == 0 {
		return nil
	}

	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight < 0 {
			return fmt.Errorf("bodyVariants weight must be greater than or equal to 0")
		}
		totalWeight += variant.Weight
	}
	if totalWeight == 0 {
		return fmt.Errorf("bodyVariants must include at least one positive weight")
	}

	return nil
}

func validateQueryVariants(variants []queryVariant) error {
	if len(variants) == 0 {
		return nil
	}

	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight < 0 {
			return fmt.Errorf("queryVariants weight must be greater than or equal to 0")
		}
		totalWeight += variant.Weight
	}
	if totalWeight == 0 {
		return fmt.Errorf("queryVariants must include at least one positive weight")
	}

	return nil
}

func selectWeightedBody(variants []bodyVariant) string {
	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight > 0 {
			totalWeight += variant.Weight
		}
	}
	if totalWeight == 0 {
		return ""
	}

	pick := rand.Intn(totalWeight)
	cumulativeWeight := 0
	for _, variant := range variants {
		if variant.Weight <= 0 {
			continue
		}
		cumulativeWeight += variant.Weight
		if pick < cumulativeWeight {
			return variant.Body
		}
	}

	return ""
}

func selectWeightedQuery(variants []queryVariant) string {
	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight > 0 {
			totalWeight += variant.Weight
		}
	}
	if totalWeight == 0 {
		return ""
	}

	pick := rand.Intn(totalWeight)
	cumulativeWeight := 0
	for _, variant := range variants {
		if variant.Weight <= 0 {
			continue
		}
		cumulativeWeight += variant.Weight
		if pick < cumulativeWeight {
			return variant.QueryParams
		}
	}

	return ""
}

func appendQueryParams(rawURL string, queryParams string) string {
	queryParams = strings.TrimPrefix(strings.TrimSpace(queryParams), "?")
	if queryParams == "" {
		return rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = queryParams
	} else {
		parsed.RawQuery = parsed.RawQuery + "&" + queryParams
	}

	return parsed.String()
}

func percentile(sortedValues []float64, percentile float64) float64 {
	if len(sortedValues) == 0 {
		return 0
	}
	index := int(math.Ceil(percentile*float64(len(sortedValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sortedValues) {
		index = len(sortedValues) - 1
	}
	return sortedValues[index]
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
