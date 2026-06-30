package komari

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

type NullTime struct {
	Time  time.Time
	Valid bool
}

func (t *NullTime) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) || bytes.Equal(data, []byte(`""`)) {
		t.Time = time.Time{}
		t.Valid = false
		return nil
	}

	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == "" {
		t.Time = time.Time{}
		t.Valid = false
		return nil
	}

	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
	} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			t.Time = parsed
			t.Valid = true
			return nil
		}
	}

	if strings.Contains(raw, " ") {
		normalized := strings.Replace(raw, " ", "T", 1)
		if parsed, err := time.Parse(time.RFC3339Nano, normalized); err == nil {
			t.Time = parsed
			t.Valid = true
			return nil
		}
	}

	t.Time = time.Time{}
	t.Valid = false
	return nil
}
