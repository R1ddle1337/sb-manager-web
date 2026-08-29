package types

import "time"

const (
	ServerLocal = "local"
	TaskPending = "pending"
	TaskRunning = "running"
	TaskSuccess = "success"
	TaskFailed  = "failed"
)

type Server struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Address            string    `json:"address,omitempty"`
	Region             string    `json:"region,omitempty"`
	Tags               []string  `json:"tags,omitempty"`
	AgentVersion       string    `json:"agent_version,omitempty"`
	SBManagerVersion   string    `json:"sb_manager_version,omitempty"`
	CoreVersion        string    `json:"core_version,omitempty"`
	Arch               string    `json:"arch,omitempty"`
	Backend            string    `json:"backend,omitempty"`
	Online             bool      `json:"online"`
	LastSeen           time.Time `json:"last_seen,omitempty"`
	Status             any       `json:"status,omitempty"`
	Capabilities       any       `json:"capabilities,omitempty"`
	AgentPublicKey     string    `json:"-"`
	AgentIdentityState string    `json:"agent_identity_state,omitempty"`
}

type Task struct {
	ID             string         `json:"id"`
	ServerID       string         `json:"server_id"`
	Action         string         `json:"action"`
	Args           map[string]any `json:"args,omitempty"`
	Status         string         `json:"status"`
	Output         string         `json:"output,omitempty"`
	Error          string         `json:"error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

type EnrollmentToken struct {
	Hash      string    `json:"hash"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

type User struct {
	Username string    `json:"username"`
	Hash     string    `json:"hash"`
	Created  time.Time `json:"created"`
}

type Session struct {
	ID        string    `json:"id"`
	CSRF      string    `json:"csrf"`
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}
