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
