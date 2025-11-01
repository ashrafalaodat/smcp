package function

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultRegistryBaseURL = "http://mcp-registry.default.svc.cluster.local"
	discoverAction         = "discover"
	invokeAction           = "invoke"
	requestReadLimit       = 1 << 20 // 1 MiB

	proxyIdentifier = "s2mcp"
)

var (
	httpClient = &http.Client{Timeout: 5 * time.Second}

	keywordTagMap = map[string][]string{
		"weather": {"weather", "temperature", "forecast", "rain", "climate"},
		"news":    {"news", "headline", "headlines"},
		"stocks":  {"stock", "stocks", "market", "finance", "ticker"},
		"search":  {"search", "lookup", "find"},
	}
)

// Handle routes discovery and invocation proxy requests for MCP tools.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(res, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	defer req.Body.Close()

	payload, err := decodeLookupRequest(req)
	if err != nil {
		writeError(res, http.StatusBadRequest, err.Error())
		return
	}

	action := strings.ToLower(strings.TrimSpace(payload.Action))
	if action == "" {
		if payload.Tool != nil && payload.Tool.ID != "" {
			action = invokeAction
		} else {
			action = discoverAction
		}
	}

	switch action {
	case discoverAction:
		handleDiscover(ctx, res, payload)
	case invokeAction:
		handleInvoke(ctx, res, payload)
	default:
		writeError(res, http.StatusBadRequest, fmt.Sprintf("unsupported action %q", action))
	}
}

// decodeLookupRequest parses the incoming JSON payload and annotates request metadata.
func decodeLookupRequest(req *http.Request) (lookupRequest, error) {
	data, err := io.ReadAll(io.LimitReader(req.Body, requestReadLimit))
	if err != nil {
		return lookupRequest{}, fmt.Errorf("read request failed: %w", err)
	}
	if len(data) == 0 {
		return lookupRequest{}, fmt.Errorf("request body required")
	}

	var payload lookupRequest
	if err := json.Unmarshal(data, &payload); err != nil {
		return lookupRequest{}, fmt.Errorf("invalid json payload: %w", err)
	}
	payload.RequestHost = req.Host
	payload.RequestScheme = forwardedScheme(req)
	return payload, nil
}

func handleDiscover(ctx context.Context, res http.ResponseWriter, payload lookupRequest) {
	snapshots, err := fetchSnapshots(ctx)
	if err != nil {
		writeError(res, http.StatusBadGateway, fmt.Sprintf("registry lookup failed: %v", err))
		return
	}

	proxyEndpoint := proxyEndpointFromRequest(payload.RequestScheme, payload.RequestHost)
	matches := filterCapabilities(payload, snapshots, proxyEndpoint)

	response := discoveryResponse{
		Matched: len(matches) > 0,
		Tools:   matches,
	}
	if response.Matched {
		response.Message = fmt.Sprintf("found %d tool(s) matching the request", len(matches))
	} else {
		response.Message = "no matching tools found in registry"
	}

	if err := writeJSON(res, http.StatusOK, response); err != nil {
		// Best effort fallback.
		_, _ = res.Write([]byte(`{"error":"failed to write response"}`))
	}
}

func handleInvoke(ctx context.Context, res http.ResponseWriter, payload lookupRequest) {
	if payload.Tool == nil || payload.Tool.ID == "" {
		writeError(res, http.StatusBadRequest, "tool selection required for invoke")
		return
	}

	snapshots, err := fetchSnapshots(ctx)
	if err != nil {
		writeError(res, http.StatusBadGateway, fmt.Sprintf("registry lookup failed: %v", err))
		return
	}

	selected, err := findCapability(payload.Tool.ID, snapshots)
	if err != nil {
		writeError(res, http.StatusNotFound, err.Error())
		return
	}

	remoteEndpoint := payload.Tool.OriginalEndpoint
	if remoteEndpoint == "" {
		remoteEndpoint = selected.Server.Endpoint
	}
	if remoteEndpoint == "" {
		writeError(res, http.StatusBadGateway, "resolved tool endpoint is empty")
		return
	}

	remotePayload := map[string]any{
		"action": invokeAction,
		"tool": map[string]any{
			"id":    payload.Tool.ID,
			"name":  firstNonEmpty(payload.Tool.Name, selected.Capability.Name),
			"input": payload.Tool.Input,
		},
		"metadata": map[string]any{
			"proxy": proxyIdentifier,
		},
	}

	reqBody, err := json.Marshal(remotePayload)
	if err != nil {
		writeError(res, http.StatusInternalServerError, fmt.Sprintf("encode proxy payload failed: %v", err))
		return
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, remoteEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		writeError(res, http.StatusBadGateway, fmt.Sprintf("construct proxy request failed: %v", err))
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		writeError(res, http.StatusBadGateway, fmt.Sprintf("proxy request failed: %v", err))
		return
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, requestReadLimit))
	if err != nil {
		writeError(res, http.StatusBadGateway, fmt.Sprintf("read proxy response failed: %v", err))
		return
	}

	var parsed any
	if len(respBody) > 0 && json.Valid(respBody) {
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			parsed = nil
		}
	}

	invokeResp := invokeResponse{
		Proxied:     true,
		Status:      httpResp.StatusCode,
		ForwardedTo: remoteEndpoint,
		ToolID:      payload.Tool.ID,
		Server: serverSummary{
			ID:       selected.Server.ID,
			Name:     selected.Server.Name,
			Endpoint: selected.Server.Endpoint,
			Region:   selected.Server.Region,
			Version:  selected.Server.Version,
			Metadata: selected.Server.Metadata,
		},
	}
	if parsed != nil {
		invokeResp.Result = parsed
	} else {
		invokeResp.Raw = string(respBody)
	}

	if err := writeJSON(res, http.StatusOK, invokeResp); err != nil {
		_, _ = res.Write([]byte(`{"error":"failed to write response"}`))
	}
}

func filterCapabilities(payload lookupRequest, snapshots []serverSnapshot, proxyEndpoint string) []toolDescriptor {
	explicitTags := normalizeTags(payload.Tags)
	heuristicTags := deriveTagsFromQuery(payload.Query)
	queryLower := strings.ToLower(payload.Query)
	queryTokens := tokenize(queryLower)

	var matches []toolDescriptor
	for _, snap := range snapshots {
		for _, cap := range snap.Capabilities {
			tagHitExplicit := anyTagMatch(cap.Tags, explicitTags)
			tagHitHeuristic := anyTagMatch(cap.Tags, heuristicTags)
			descriptorTokens := descriptorTokenSet(cap, snap)
			tokenScore := overlapScore(queryTokens, descriptorTokens)
			matchByQuery := tokenScore > 0 || capabilityMatchesQuery(cap, snap, queryLower, queryTokens)

			switch {
			case len(explicitTags) > 0 && !tagHitExplicit:
				continue
			case len(explicitTags) == 0 && len(heuristicTags) > 0 && !tagHitHeuristic && !matchByQuery:
				continue
			case payload.Query != "" && len(explicitTags) == 0 && len(heuristicTags) == 0 && !matchByQuery:
				continue
			}

			if snap.Server.Metadata == nil {
				snap.Server.Metadata = map[string]any{}
			}

			score := calculateScore(tagHitExplicit, tagHitHeuristic, tokenScore, len(queryTokens))
			matches = append(matches, toolDescriptor{
				ID:               fmt.Sprintf("%s:%s", snap.Server.ID, cap.ID),
				Name:             cap.Name,
				Type:             cap.Type,
				Tags:             cap.Tags,
				ProxyEndpoint:    proxyEndpoint,
				OriginalEndpoint: snap.Server.Endpoint,
				Server: serverSummary{
					ID:       snap.Server.ID,
					Name:     snap.Server.Name,
					Endpoint: snap.Server.Endpoint,
					Region:   snap.Server.Region,
					Version:  snap.Server.Version,
					Metadata: snap.Server.Metadata,
				},
				Capability: capabilitySummary{
					ID:   cap.ID,
					Name: cap.Name,
					Type: cap.Type,
					Tags: cap.Tags,
				},
				Score: score,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Name < matches[j].Name
		}
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > 5 {
		matches = matches[:5]
	}

	return matches
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			seen[tag] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	result := make([]string, 0, len(seen))
	for tag := range seen {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func deriveTagsFromQuery(query string) []string {
	seen := map[string]struct{}{}
	lowerQuery := strings.ToLower(query)
	if lowerQuery != "" {
		for tag, keywords := range keywordTagMap {
			for _, kw := range keywords {
				if strings.Contains(lowerQuery, kw) {
					seen[tag] = struct{}{}
					break
				}
			}
		}
		for _, token := range uniqueTokens(tokenize(lowerQuery)) {
			if len(token) > 2 {
				seen[token] = struct{}{}
			}
		}
	}

	if len(seen) == 0 {
		return nil
	}
	result := make([]string, 0, len(seen))
	for tag := range seen {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func capabilityMatchesQuery(cap capabilityPayload, snap serverSnapshot, queryLower string, tokens []string) bool {
	if queryLower == "" {
		return true
	}
	nameLower := strings.ToLower(cap.Name)
	typeLower := strings.ToLower(cap.Type)
	serverNameLower := strings.ToLower(snap.Server.Name)

	if strings.Contains(nameLower, queryLower) || strings.Contains(queryLower, nameLower) {
		return true
	}
	if strings.Contains(typeLower, queryLower) {
		return true
	}
	if strings.Contains(serverNameLower, queryLower) {
		return true
	}
	for _, tag := range cap.Tags {
		tagLower := strings.ToLower(tag)
		if strings.Contains(queryLower, tagLower) || strings.Contains(tagLower, queryLower) {
			return true
		}
		for _, tok := range tokens {
			if tagLower == tok {
				return true
			}
		}
	}
	return false
}

func anyTagMatch(tags, candidates []string) bool {
	if len(tags) == 0 || len(candidates) == 0 {
		return false
	}
	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[strings.ToLower(t)] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := tagSet[strings.ToLower(candidate)]; ok {
			return true
		}
	}
	return false
}

func tokenize(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	results := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			results = append(results, f)
		}
	}
	return results
}

func uniqueTokens(tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	seen := make(map[string]struct{}, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		result = append(result, tok)
	}
	return result
}

func descriptorTokenSet(cap capabilityPayload, snap serverSnapshot) []string {
	var parts []string
	parts = append(parts, cap.Name, cap.Type, strings.Join(cap.Tags, " "))
	parts = append(parts, snap.Server.Name, snap.Server.Region, snap.Server.Version)
	if desc := metadataDescription(snap.Server.Metadata); desc != "" {
		parts = append(parts, desc)
	}
	if schemaText := flattenToText(cap.Schema); schemaText != "" {
		parts = append(parts, schemaText)
	}
	descriptor := strings.ToLower(strings.Join(parts, " "))
	return uniqueTokens(tokenize(descriptor))
}

func metadataDescription(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if desc, ok := metadata["description"]; ok {
		return flattenToText(desc)
	}
	return ""
}

func flattenToText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case []string:
		return strings.Join(v, " ")
	case []any:
		var parts []string
		for _, item := range v {
			if s := flattenToText(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		var keys []string
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var parts []string
		for _, key := range keys {
			if s := flattenToText(v[key]); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func overlapScore(queryTokens []string, descriptorTokens []string) float64 {
	if len(queryTokens) == 0 || len(descriptorTokens) == 0 {
		return 0
	}
	descSet := make(map[string]struct{}, len(descriptorTokens))
	for _, tok := range descriptorTokens {
		if tok == "" {
			continue
		}
		descSet[tok] = struct{}{}
	}

	uniqueQuery := uniqueTokens(queryTokens)
	if len(uniqueQuery) == 0 {
		return 0
	}

	count := 0
	for _, tok := range uniqueQuery {
		if _, ok := descSet[tok]; ok {
			count++
		}
	}

	return float64(count) / float64(len(uniqueQuery))
}

func calculateScore(explicitMatch, heuristicMatch bool, tokenScore float64, queryTokenCount int) float64 {
	score := tokenScore * 2
	if explicitMatch {
		score += 3
	}
	if heuristicMatch {
		score += 1
	}
	if score == 0 && queryTokenCount == 0 && !explicitMatch && !heuristicMatch {
		score = 0.1
	}
	return score
}

func proxyEndpointFromRequest(scheme, host string) string {
	if host == "" {
		return ""
	}
	if scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func fetchSnapshots(ctx context.Context) ([]serverSnapshot, error) {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("REGISTRY_BASE_URL")), "/")
	if base == "" {
		base = defaultRegistryBaseURL
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/servers", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, requestReadLimit))
		return nil, fmt.Errorf("registry returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var snapshots []serverSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshots); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return snapshots, nil
}

func findCapability(toolID string, snapshots []serverSnapshot) (capabilitySelection, error) {
	parts := strings.Split(toolID, ":")
	if len(parts) < 2 {
		return capabilitySelection{}, fmt.Errorf("invalid tool id %q", toolID)
	}
	serverID := parts[0]
	capabilityID := parts[1]

	for _, snap := range snapshots {
		if snap.Server.ID != serverID {
			continue
		}
		for _, cap := range snap.Capabilities {
			if cap.ID == capabilityID {
				if snap.Server.Metadata == nil {
					snap.Server.Metadata = map[string]any{}
				}
				return capabilitySelection{
					Server:     snap.Server,
					Capability: cap,
				}, nil
			}
		}
	}
	return capabilitySelection{}, fmt.Errorf("tool %q not found in registry snapshots", toolID)
}

func writeError(res http.ResponseWriter, status int, message string) {
	_ = writeJSON(res, status, map[string]any{
		"error":   message,
		"status":  status,
		"success": false,
	})
}

func writeJSON(res http.ResponseWriter, status int, payload any) error {
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(status)
	if payload == nil {
		return nil
	}
	return json.NewEncoder(res).Encode(payload)
}

func forwardedScheme(req *http.Request) string {
	if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type lookupRequest struct {
	Action string       `json:"action"`
	Query  string       `json:"query"`
	Tags   []string     `json:"tags"`
	Tool   *toolRequest `json:"tool"`

	RequestHost   string `json:"-"`
	RequestScheme string `json:"-"`
}

type toolRequest struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	OriginalEndpoint string         `json:"original_endpoint"`
	Input            map[string]any `json:"input"`
}

type discoveryResponse struct {
	Matched bool             `json:"matched"`
	Message string           `json:"message"`
	Tools   []toolDescriptor `json:"tools"`
}

type invokeResponse struct {
	Proxied     bool          `json:"proxied"`
	Status      int           `json:"status"`
	ForwardedTo string        `json:"forwarded_to"`
	ToolID      string        `json:"tool_id"`
	Server      serverSummary `json:"server"`
	Result      any           `json:"result,omitempty"`
	Raw         string        `json:"raw,omitempty"`
}

type toolDescriptor struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Tags             []string          `json:"tags,omitempty"`
	ProxyEndpoint    string            `json:"proxy_endpoint"`
	OriginalEndpoint string            `json:"original_endpoint"`
	Server           serverSummary     `json:"server"`
	Capability       capabilitySummary `json:"capability"`
	Score            float64           `json:"score,omitempty"`
}

type serverSummary struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Endpoint string         `json:"endpoint"`
	Region   string         `json:"region,omitempty"`
	Version  string         `json:"version,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type capabilitySummary struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Type string   `json:"type"`
	Tags []string `json:"tags,omitempty"`
}

type serverSnapshot struct {
	Server       serverPayload       `json:"server"`
	Capabilities []capabilityPayload `json:"capabilities"`
}

type serverPayload struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Endpoint   string         `json:"endpoint"`
	Version    string         `json:"version"`
	Region     string         `json:"region"`
	AuthMethod string         `json:"auth_method"`
	Metadata   map[string]any `json:"metadata"`
}

type capabilityPayload struct {
	ID        string   `json:"id"`
	ServerID  string   `json:"server_id"`
	Type      string   `json:"type"`
	Name      string   `json:"name"`
	Schema    any      `json:"schema"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

type capabilitySelection struct {
	Server     serverPayload
	Capability capabilityPayload
}
