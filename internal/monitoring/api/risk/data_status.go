package risk

import "net/http"

type DataStatus string

const (
	DataStatusServiceUnavailable DataStatus = "service_unavailable"
	DataStatusAvailable          DataStatus = "available"
	DataStatusNoData             DataStatus = "no_data"
	DataStatusCalibrating        DataStatus = "calibrating"
	DataStatusInsufficientData   DataStatus = "insufficient_data"
)

func serviceUnavailable(reason, detail string) (int, any) {
	return http.StatusServiceUnavailable, map[string]any{
		"data_status": string(DataStatusServiceUnavailable),
		"reason":      reason,
		"detail":      detail,
	}
}
