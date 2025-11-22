package domain

// ToolRecord contains the minimal registry payload persisted inside Qdrant.
type ToolRecord struct {
	Owner      string    `json:"owner"`
	Name       string    `json:"name"`
	Embedding  []float32 `json:"embedding"`
	Visibility string    `json:"visibility"`
}

// ScoredTool couples a tool with its similarity score.
type ScoredTool struct {
	Tool  ToolRecord `json:"tool"`
	Score float32    `json:"score"`
}
