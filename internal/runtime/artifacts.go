package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"

	"prediction-market-arb-bot/internal/types"
)

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalizeJSONValue(value), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func appendJSONLine(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(normalizeJSONValue(value))
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func normalizeJSONValue(value any) any {
	switch v := value.(type) {
	case types.ValidationResult:
		if v.Errors == nil {
			v.Errors = []types.ValidationError{}
		}
		if v.Warnings == nil {
			v.Warnings = []types.ValidationError{}
		}
		return v
	case types.ExecutionPreview:
		if v.Items == nil {
			v.Items = []types.ExecutionPreviewItem{}
		}
		return v
	case types.ExecutionResult:
		if v.Trades == nil {
			v.Trades = []types.PaperTrade{}
		}
		return v
	case types.PortfolioSnapshot:
		if v.Positions == nil {
			v.Positions = []types.PaperPosition{}
		}
		return v
	default:
		return value
	}
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
