package api

type ListRequest struct {
}

type ListResponse struct {
	Monitors []MonitorDTO `json:"monitors"`
}

type MonitorDTO struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type StatusRequest struct {
	Name string `params:"name"`
}

type StatusResponse struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type DashboardRequest struct {
}

type DashboardResponse struct {
	Monitors []MonitorHealthDTO `json:"monitors"`
}

type MonitorHealthDTO struct {
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	State     string     `json:"state"`
	LatencyMs int64      `json:"latencyMs"`
	UptimePct float64    `json:"uptimePct"`
	Recent    []CheckDTO `json:"recent"`
}

type CheckDTO struct {
	Up        bool  `json:"up"`
	LatencyMs int64 `json:"latencyMs"`
	At        int64 `json:"at"` // unix seconds
}
