package function

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

type mathRequest struct {
	A  *float64 `json:"a,omitempty"`
	B  *float64 `json:"b,omitempty"`
	Op string   `json:"op"`
}

type mathResponse struct {
	Result    float64 `json:"result"`
	Operation string  `json:"operation"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Handle processes math operations requested via JSON body or query parameters.
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	mReq, err := parseMathRequest(req)
	if err != nil {
		writeError(res, http.StatusBadRequest, err)
		return
	}

	result, opLabel, err := compute(*mReq.A, *mReq.B, mReq.Op)
	if err != nil {
		writeError(res, http.StatusBadRequest, err)
		return
	}

	payload := mathResponse{
		Result:    result,
		Operation: opLabel,
	}

	res.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(res).Encode(payload); err != nil {
		writeError(res, http.StatusInternalServerError, err)
	}
}

func parseMathRequest(req *http.Request) (*mathRequest, error) {
	var payload mathRequest

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	defer req.Body.Close()

	if len(bytes.TrimSpace(body)) > 0 {
		var direct mathRequest
		if err := json.Unmarshal(body, &direct); err == nil && (direct.A != nil || direct.B != nil || direct.Op != "") {
			payload = direct
		} else {
			var wrapper struct {
				Input *mathRequest `json:"input"`
			}
			if err := json.Unmarshal(body, &wrapper); err != nil {
				return nil, fmt.Errorf("decode json payload: %w", err)
			}
			if wrapper.Input == nil {
				return nil, errors.New("missing input payload")
			}
			payload = *wrapper.Input
		}
	} else {
		payload.Op = req.URL.Query().Get("op")
		if payload.Op == "" {
			return nil, errors.New("missing operation. Provide 'op' field via JSON body or query parameter")
		}

		if aStr := req.URL.Query().Get("a"); aStr != "" {
			aVal, err := strconv.ParseFloat(aStr, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid 'a' value: %w", err)
			}
			payload.A = &aVal
		}
		if bStr := req.URL.Query().Get("b"); bStr != "" {
			bVal, err := strconv.ParseFloat(bStr, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid 'b' value: %w", err)
			}
			payload.B = &bVal
		}
	}

	if payload.A == nil || payload.B == nil {
		return nil, errors.New("both 'a' and 'b' must be provided")
	}
	if payload.Op == "" {
		return nil, errors.New("operation 'op' must be provided")
	}

	return &payload, nil
}

func compute(a, b float64, op string) (float64, string, error) {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "add", "+":
		return a + b, "add", nil
	case "subtract", "sub", "-":
		return a - b, "subtract", nil
	case "multiply", "mul", "*", "x":
		return a * b, "multiply", nil
	case "divide", "div", "/":
		if b == 0 {
			return 0, "", errors.New("cannot divide by zero")
		}
		return a / b, "divide", nil
	case "power", "pow", "^":
		return math.Pow(a, b), "power", nil
	default:
		return 0, "", fmt.Errorf("unsupported operation %q", op)
	}
}

func writeError(res http.ResponseWriter, status int, err error) {
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(status)
	_ = json.NewEncoder(res).Encode(errorResponse{Error: err.Error()})
}
