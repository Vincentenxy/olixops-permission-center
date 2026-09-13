package health

import "time"

type Service struct{}

type Status struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

func (Service) Check() Status {
	return Status{Status: "ok", Time: time.Now().UTC()}
}
