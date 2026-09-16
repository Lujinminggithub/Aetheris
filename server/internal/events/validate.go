package events

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

func ParseAndValidate(raw []byte) (Event, error) {
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return Event{}, fmt.Errorf("invalid event JSON: %w", err)
	}
	required := []string{"event_id", "schema_version", "event_type", "tenant_id", "subject_id", "device_id", "project_id", "session_id", "correlation_id", "source", "source_version", "occurred_at", "ingested_at", "payload", "content_hash", "redaction_report", "processing_grants"}
	for _, field := range required {
		if _, ok := data[field]; !ok {
			return Event{}, fmt.Errorf("missing event field: %s", field)
		}
	}
	if number, ok := data["schema_version"].(json.Number); !ok || number.String() != "1" {
		return Event{}, fmt.Errorf("schema_version must be integer 1")
	}
	for _, field := range []string{"event_id", "event_type", "tenant_id", "subject_id", "device_id", "project_id", "session_id", "correlation_id", "source", "source_version", "occurred_at", "ingested_at", "content_hash"} {
		value, ok := data[field].(string)
		if !ok || value == "" {
			return Event{}, fmt.Errorf("%s must be a non-empty string", field)
		}
	}
	for _, field := range []string{"occurred_at", "ingested_at"} {
		if _, err := time.Parse(time.RFC3339Nano, data[field].(string)); err != nil {
			return Event{}, fmt.Errorf("%s must be RFC3339: %w", field, err)
		}
	}
	if _, ok := data["payload"].(map[string]any); !ok {
		return Event{}, fmt.Errorf("payload must be an object")
	}
	if _, ok := data["redaction_report"].(map[string]any); !ok {
		return Event{}, fmt.Errorf("redaction_report must be an object")
	}
	grants, ok := data["processing_grants"].([]any)
	if !ok {
		return Event{}, fmt.Errorf("processing_grants must be an array")
	}
	for _, grant := range grants {
		if _, ok := grant.(string); !ok {
			return Event{}, fmt.Errorf("processing_grants must contain strings")
		}
	}

	provided := data["content_hash"].(string)
	delete(data, "content_hash")
	canonical, err := canonicalASCII(data)
	if err != nil {
		return Event{}, err
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != provided {
		return Event{}, fmt.Errorf("content_hash does not match canonical event")
	}
	data["content_hash"] = provided
	encoded, _ := json.Marshal(data)
	var event Event
	if err := json.Unmarshal(encoded, &event); err != nil {
		return Event{}, fmt.Errorf("decode event: %w", err)
	}
	event.Raw = data
	return event, nil
}

// canonicalASCII 与现有 Python 端的 json.dumps(ensure_ascii=True, sort_keys=True, separators=(',', ':')) 保持一致。
func canonicalASCII(data map[string]any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	raw := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	result := make([]byte, 0, len(raw))
	for index := 0; index < len(raw); {
		if raw[index] < utf8.RuneSelf {
			result = append(result, raw[index])
			index++
			continue
		}
		runeValue, size := utf8.DecodeRune(raw[index:])
		if runeValue == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("invalid UTF-8 in event")
		}
		if runeValue <= 0xFFFF {
			result = append(result, '\\', 'u', hexDigit(byte((runeValue>>12)&0xF)), hexDigit(byte((runeValue>>8)&0xF)), hexDigit(byte((runeValue>>4)&0xF)), hexDigit(byte(runeValue&0xF)))
		} else {
			value := runeValue - 0x10000
			high := 0xD800 + (value >> 10)
			low := 0xDC00 + (value & 0x3FF)
			result = append(result, '\\', 'u', hexDigit(byte((high>>12)&0xF)), hexDigit(byte((high>>8)&0xF)), hexDigit(byte((high>>4)&0xF)), hexDigit(byte(high&0xF)))
			result = append(result, '\\', 'u', hexDigit(byte((low>>12)&0xF)), hexDigit(byte((low>>8)&0xF)), hexDigit(byte((low>>4)&0xF)), hexDigit(byte(low&0xF)))
		}
		index += size
	}
	return result, nil
}

func hexDigit(value byte) byte {
	if value < 10 {
		return '0' + value
	}
	return 'a' + value - 10
}
