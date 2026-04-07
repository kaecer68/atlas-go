package domain

import (
	"fmt"
	"time"
)

type FlexTime struct {
	time.Time
}

func (ft *FlexTime) UnmarshalJSON(data []byte) error {
	str := string(data)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	if str == "" || str == "null" {
		ft.Time = time.Time{}
		return nil
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05.999",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, str); err == nil {
			ft.Time = t
			return nil
		}
	}

	return fmt.Errorf("cannot parse time %q", str)
}

func (ft FlexTime) MarshalJSON() ([]byte, error) {
	if ft.IsZero() {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf("%q", ft.Format(time.RFC3339Nano))), nil
}
